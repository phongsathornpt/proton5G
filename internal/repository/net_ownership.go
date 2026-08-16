package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

const sysClassNetRoot = "/sys/class/net"

// ValidateFM350NetIface protects destructive host networking operations by
// requiring the target interface to belong to a known FM350 USB device.
func ValidateFM350NetIface(iface string) error {
	return validateFM350NetIfaceAt(sysClassNetRoot, iface)
}

func validateFM350NetIfaceAt(classNetRoot, iface string) error {
	if !validIfaceName(iface) {
		return fmt.Errorf("invalid network interface name %q", iface)
	}
	devicePath := filepath.Join(classNetRoot, iface, "device")
	real, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return fmt.Errorf("network interface %q is not backed by a discoverable device: %w", iface, err)
	}

	for path := filepath.Clean(real); ; path = filepath.Dir(path) {
		vendor, product := readUSBID(path)
		if vendor != "" || product != "" {
			if domain.IsFM350(vendor, product) {
				return nil
			}
			return fmt.Errorf("network interface %q belongs to USB %s:%s, not an FM350", iface, vendor, product)
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
	}
	return fmt.Errorf("network interface %q is not owned by a recognized FM350 USB device", iface)
}

// ValidateFM350RNDISIface protects RNDIS-specific host operations. The target
// must belong to an FM350 and be bound to Linux's rndis_host driver. A cdc_mbim
// or other USB-network interface is a different data protocol and must not be
// configured with the FM350 RNDIS address convention.
func ValidateFM350RNDISIface(iface string) error {
	return validateFM350RNDISIfaceAt(sysClassNetRoot, iface)
}

func validateFM350RNDISIfaceAt(classNetRoot, iface string) error {
	if err := validateFM350NetIfaceAt(classNetRoot, iface); err != nil {
		return err
	}
	driver := strings.ToLower(netIfaceDeviceDriverAt(classNetRoot, iface))
	switch driver {
	case "rndis_host":
		return nil
	case "":
		return fmt.Errorf("cannot determine kernel driver for FM350 network interface %q", iface)
	case "cdc_mbim":
		return fmt.Errorf("network interface %q is backed by cdc_mbim; use its MBIM control endpoint instead of RNDIS", iface)
	default:
		return fmt.Errorf("network interface %q is backed by %s, not rndis_host", iface, driver)
	}
}

// netIfaceDeviceDriver reports the bound kernel driver for a host net interface.
// Empty means unavailable. It is intentionally best-effort telemetry only.
func netIfaceDeviceDriver(iface string) string {
	return netIfaceDeviceDriverAt(sysClassNetRoot, iface)
}

func netIfaceDeviceDriverAt(classNetRoot, iface string) string {
	if !validIfaceName(iface) {
		return ""
	}
	link := filepath.Join(classNetRoot, iface, "device", "driver")
	real, err := filepath.EvalSymlinks(link)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(real); err != nil {
		return ""
	}
	return filepath.Base(real)
}
