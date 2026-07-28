package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

// probeCache avoids thrashing exclusive serial ports during UI polls.
var (
	probeMu    sync.Mutex
	probeCache = map[string]probeEntry{}
	probeTTL   = 15 * time.Second
)

type probeEntry struct {
	ok        bool
	expiresAt time.Time
}

// ListMBIMDevices returns all existing /dev/cdc-wdm* paths.
func ListMBIMDevices() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, d := range DefaultCDCWdm {
		if st, err := os.Stat(d); err == nil && !st.IsDir() {
			if _, ok := seen[d]; !ok {
				seen[d] = struct{}{}
				out = append(out, d)
			}
		}
	}
	matches, _ := filepath.Glob(CDCWdmGlob)
	sort.Strings(matches)
	for _, m := range matches {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

// ListUSBDevices returns every sysfs USB device matching vendor/product.
func ListUSBDevices(vendor, product string) []string {
	if vendor == "" {
		vendor = domain.DefaultFM350.Vendor
	}
	if product == "" {
		product = domain.DefaultFM350.Product
	}
	matches, err := filepath.Glob(SysfsUSBGlob)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range matches {
		v, errV := os.ReadFile(filepath.Join(m, "idVendor"))
		p, errP := os.ReadFile(filepath.Join(m, "idProduct"))
		if errV != nil || errP != nil {
			continue
		}
		if strings.TrimSpace(string(v)) == vendor && strings.TrimSpace(string(p)) == product {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// ttyNodesUnder walks a USB sysfs device tree for ttyUSB* names → /dev paths.
func ttyNodesUnder(sysPath string) []string {
	var names []string
	_ = filepath.Walk(sysPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		base := info.Name()
		if strings.HasPrefix(base, "ttyUSB") {
			names = append(names, "/dev/"+base)
		}
		// Some kernels expose .../tty/ttyUSB0
		if info.IsDir() && base == "tty" {
			entries, _ := os.ReadDir(path)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "ttyUSB") {
					names = append(names, "/dev/"+e.Name())
				}
			}
		}
		return nil
	})
	return uniqueSorted(names)
}

// mbimNodesUnder finds cdc-wdm nodes linked under a USB device sysfs tree.
func mbimNodesUnder(sysPath string) []string {
	var names []string
	_ = filepath.Walk(sysPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		base := info.Name()
		if strings.HasPrefix(base, "cdc-wdm") {
			dev := "/dev/" + base
			if st, err := os.Stat(dev); err == nil && !st.IsDir() {
				names = append(names, dev)
			}
		}
		return nil
	})
	return uniqueSorted(names)
}

func uniqueSorted(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ProbeATPortCached returns whether path answers AT, with a short TTL cache.
// skipPath is the currently open AT port (assumed usable; not re-probed).
func ProbeATPortCached(path, skipPath string) bool {
	if path == "" {
		return false
	}
	if skipPath != "" && path == skipPath {
		return true
	}
	probeMu.Lock()
	if e, ok := probeCache[path]; ok && time.Now().Before(e.expiresAt) {
		okv := e.ok
		probeMu.Unlock()
		return okv
	}
	probeMu.Unlock()

	ok := ProbeATPort(path)

	probeMu.Lock()
	probeCache[path] = probeEntry{ok: ok, expiresAt: time.Now().Add(probeTTL)}
	probeMu.Unlock()
	return ok
}

// ListModems builds logical modem entries for UI selection.
// openATPort is the manager's current serial path (skipped for exclusive probe).
func ListModems(vendor, product, openATPort string) []domain.ModemDevice {
	if vendor == "" {
		vendor = domain.DefaultFM350.Vendor
	}
	if product == "" {
		product = domain.DefaultFM350.Product
	}

	usbPaths := ListUSBDevices(vendor, product)
	allMBIM := ListMBIMDevices()
	assignedMBIM := map[string]struct{}{}
	assignedTTY := map[string]struct{}{}

	var modems []domain.ModemDevice

	for i, sys := range usbPaths {
		pwr := domain.PowerUnknown
		if b, err := os.ReadFile(filepath.Join(sys, "power", "control")); err == nil {
			pwr = domain.NormalizePowerControl(string(b))
		}
		ttys := ttyNodesUnder(sys)
		mbims := mbimNodesUnder(sys)
		// If walk found nothing, still allow global assignment later.

		var atIfaces []domain.ModemInterface
		for _, t := range ttys {
			assignedTTY[t] = struct{}{}
			ready := ProbeATPortCached(t, openATPort)
			label := t
			if ready {
				label = t + " (AT OK)"
			}
			atIfaces = append(atIfaces, domain.ModemInterface{
				Path:    t,
				Kind:    domain.IfaceKindAT,
				ATReady: ready,
				Label:   label,
			})
		}

		var mbimIfaces []domain.ModemInterface
		for _, m := range mbims {
			assignedMBIM[m] = struct{}{}
			mbimIfaces = append(mbimIfaces, domain.ModemInterface{
				Path:  m,
				Kind:  domain.IfaceKindMBIM,
				Label: m,
			})
		}

		base := filepath.Base(sys)
		name := fmt.Sprintf("Fibocom FM350-GL @ %s", base)
		if len(usbPaths) == 1 {
			name = "Fibocom FM350-GL"
		}
		modems = append(modems, domain.ModemDevice{
			ID:           "usb:" + sys,
			Name:         name,
			VendorID:     vendor,
			ProductID:    product,
			SysPath:      sys,
			Connected:    true,
			PowerControl: pwr,
			ATPorts:      atIfaces,
			MBIMNodes:    mbimIfaces,
		})
		_ = i
	}

	// Attach unassigned global MBIM nodes to the first USB modem if only one; else unassociated.
	var orphanMBIM []string
	for _, m := range allMBIM {
		if _, ok := assignedMBIM[m]; !ok {
			orphanMBIM = append(orphanMBIM, m)
		}
	}
	if len(orphanMBIM) > 0 {
		if len(modems) == 1 {
			for _, m := range orphanMBIM {
				modems[0].MBIMNodes = append(modems[0].MBIMNodes, domain.ModemInterface{
					Path: m, Kind: domain.IfaceKindMBIM, Label: m,
				})
				assignedMBIM[m] = struct{}{}
			}
		} else {
			var ifaces []domain.ModemInterface
			for _, m := range orphanMBIM {
				ifaces = append(ifaces, domain.ModemInterface{
					Path: m, Kind: domain.IfaceKindMBIM, Label: m,
				})
			}
			modems = append(modems, domain.ModemDevice{
				ID:        "mbim:unassociated",
				Name:      "MBIM devices (unassociated)",
				Connected: true,
				MBIMNodes: ifaces,
			})
		}
	}

	// Serial-only fallbacks for ttyUSB not under a matched USB device.
	candidates, _ := ListCandidatePorts(vendor, product)
	for _, c := range candidates {
		if _, ok := assignedTTY[c]; ok {
			continue
		}
		// Only add if device node exists
		if st, err := os.Stat(c); err != nil || st.IsDir() {
			continue
		}
		ready := ProbeATPortCached(c, openATPort)
		label := c
		if ready {
			label = c + " (AT OK)"
		}
		modems = append(modems, domain.ModemDevice{
			ID:        "serial:" + c,
			Name:      "Serial " + c,
			Connected: true,
			ATPorts: []domain.ModemInterface{{
				Path: c, Kind: domain.IfaceKindAT, ATReady: ready, Label: label,
			}},
		})
		assignedTTY[c] = struct{}{}
	}

	// Ensure currently open AT port appears even if discovery missed it.
	if openATPort != "" {
		if _, ok := assignedTTY[openATPort]; !ok {
			if st, err := os.Stat(openATPort); err == nil && !st.IsDir() {
				modems = append(modems, domain.ModemDevice{
					ID:        "serial:" + openATPort,
					Name:      "Serial " + openATPort + " (active)",
					Connected: true,
					ATPorts: []domain.ModemInterface{{
						Path: openATPort, Kind: domain.IfaceKindAT, ATReady: true, Label: openATPort + " (active)",
					}},
				})
			}
		}
	}

	return modems
}

// FindModem returns modem by id from a list.
func FindModem(modems []domain.ModemDevice, id string) (domain.ModemDevice, bool) {
	for _, m := range modems {
		if m.ID == id {
			return m, true
		}
	}
	return domain.ModemDevice{}, false
}

// ModemHasATPort reports whether path is listed under modem.
func ModemHasATPort(m domain.ModemDevice, path string) bool {
	for _, p := range m.ATPorts {
		if p.Path == path {
			return true
		}
	}
	return false
}

// ModemHasMBIM reports whether path is listed under modem.
func ModemHasMBIM(m domain.ModemDevice, path string) bool {
	for _, p := range m.MBIMNodes {
		if p.Path == path {
			return true
		}
	}
	return false
}

// PreferredATPort picks best AT path for a modem.
func PreferredATPort(m domain.ModemDevice, preferred string) string {
	if preferred != "" && ModemHasATPort(m, preferred) {
		return preferred
	}
	for _, p := range m.ATPorts {
		if p.ATReady {
			return p.Path
		}
	}
	if len(m.ATPorts) > 0 {
		return m.ATPorts[0].Path
	}
	return ""
}
