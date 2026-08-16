package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

// Exported aliases for callers (same as domain.DefaultFM350).
var (
	VendorID  = domain.DefaultFM350.Vendor
	ProductID = domain.DefaultFM350.Product
)

type Watchdog struct {
	vendorID  string
	productID string
	sysfsGlob string
}

func NewWatchdog(vendor, product string) *Watchdog {
	if vendor == "" {
		vendor = domain.DefaultFM350.Vendor
	}
	if product == "" {
		product = domain.DefaultFM350.Product
	}
	return &Watchdog{
		vendorID:  vendor,
		productID: product,
		sysfsGlob: SysfsUSBGlob,
	}
}

func (w *Watchdog) Check(globPattern string) domain.ModemStatus {
	if globPattern == "" {
		globPattern = w.sysfsGlob
	}

	devPath, found := w.findDevicePath(globPattern)
	if !found {
		return domain.ModemStatus{Connected: false}
	}

	pwrCtrl := domain.NormalizePowerControl(w.readSysfsValue(filepath.Join(devPath, "power", "control")))
	status := domain.ModemStatus{
		Connected:    true,
		SysPath:      devPath,
		PowerControl: pwrCtrl,
	}

	_ = w.DisableAutosuspend()
	_ = w.EnsurePowerOn(devPath)

	return status
}

func (w *Watchdog) findDevicePath(globPattern string) (string, bool) {
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return "", false
	}
	for _, m := range matches {
		v, errV := os.ReadFile(filepath.Join(m, "idVendor"))
		p, errP := os.ReadFile(filepath.Join(m, "idProduct"))
		if errV == nil && errP == nil {
			if domain.MatchFM350Filter(w.vendorID, w.productID, strings.TrimSpace(string(v)), strings.TrimSpace(string(p))) {
				return m, true
			}
		}
	}
	return "", false
}

func (w *Watchdog) DisableAutosuspend() error {
	data, err := os.ReadFile(AutosuspendPath)
	if err != nil {
		return fmt.Errorf("read autosuspend: %w", err)
	}
	if strings.TrimSpace(string(data)) != AutosuspendOff {
		return os.WriteFile(AutosuspendPath, []byte(AutosuspendOff+"\n"), 0644)
	}
	return nil
}

func (w *Watchdog) EnsurePowerOn(sysPath string) error {
	p := filepath.Join(sysPath, "power", "control")
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read power/control: %w", err)
	}
	if domain.NormalizePowerControl(string(data)) != domain.PowerOn {
		return os.WriteFile(p, []byte(string(domain.PowerOn)+"\n"), 0644)
	}
	return nil
}

func (w *Watchdog) readSysfsValue(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return string(domain.PowerUnknown)
	}
	return strings.TrimSpace(string(data))
}

// USBDeviceNode resolves /dev/bus/usb/BBB/DDD from a sysfs device path.
func USBDeviceNode(sysPath string) (string, error) {
	busRaw, err := os.ReadFile(filepath.Join(sysPath, "busnum"))
	if err != nil {
		return "", fmt.Errorf("read busnum: %w", err)
	}
	devRaw, err := os.ReadFile(filepath.Join(sysPath, "devnum"))
	if err != nil {
		return "", fmt.Errorf("read devnum: %w", err)
	}
	bus, err := strconv.Atoi(strings.TrimSpace(string(busRaw)))
	if err != nil {
		return "", fmt.Errorf("parse busnum: %w", err)
	}
	dev, err := strconv.Atoi(strings.TrimSpace(string(devRaw)))
	if err != nil {
		return "", fmt.Errorf("parse devnum: %w", err)
	}
	return fmt.Sprintf(USBFSNodeFmt, bus, dev), nil
}

// HardReset issues USBDEVFS_RESET against the device's usbfs node.
func (w *Watchdog) HardReset(sysPath string) error {
	if sysPath == "" {
		path, found := w.findDevicePath(w.sysfsGlob)
		if !found {
			return fmt.Errorf("modem not present in sysfs")
		}
		sysPath = path
	}

	node, err := USBDeviceNode(sysPath)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(node, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", node, err)
	}
	defer f.Close()

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(usbdevfsReset), 0)
	if errno != 0 {
		return fmt.Errorf("USBDEVFS_RESET on %s: %w", node, errno)
	}
	return nil
}

func (w *Watchdog) HardResetIfPresent() error {
	return w.HardReset("")
}

func (w *Watchdog) RunLoop(interval time.Duration, statusCh chan<- domain.ModemStatus, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	select {
	case statusCh <- w.Check(""):
	case <-stopCh:
		return
	}

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			select {
			case statusCh <- w.Check(""):
			case <-stopCh:
				return
			}
		}
	}
}
