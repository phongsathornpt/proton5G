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
	// Bound concurrent AT probes (each open is independent tty; avoid hammering USB).
	probeParallelism = 6
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

func readUSBID(sysPath string) (vendor, product string) {
	if b, err := os.ReadFile(filepath.Join(sysPath, "idVendor")); err == nil {
		vendor = domain.NormalizeUSBHex(string(b))
	}
	if b, err := os.ReadFile(filepath.Join(sysPath, "idProduct")); err == nil {
		product = domain.NormalizeUSBHex(string(b))
	}
	return vendor, product
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
		if domain.MatchFM350Filter(vendor, product, strings.TrimSpace(string(v)), strings.TrimSpace(string(p))) {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// resolveSysfs follows /sys/bus/usb/devices/* symlinks so Walk can descend.
func resolveSysfs(sysPath string) string {
	if sysPath == "" {
		return sysPath
	}
	if real, err := filepath.EvalSymlinks(sysPath); err == nil && real != "" {
		return real
	}
	return sysPath
}

// ttyNodesUnder walks a USB sysfs device tree for ttyUSB* names → /dev paths.
func ttyNodesUnder(sysPath string) []string {
	var names []string
	_ = filepath.Walk(resolveSysfs(sysPath), func(path string, info os.FileInfo, err error) error {
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
	_ = filepath.Walk(resolveSysfs(sysPath), func(path string, info os.FileInfo, err error) error {
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

// netIfacesUnder finds network interface names under a USB device (RNDIS/ECM/NCM).
func netIfacesUnder(sysPath string) []string {
	var names []string
	_ = filepath.Walk(resolveSysfs(sysPath), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		// .../net/<ifname>
		if info.IsDir() && info.Name() == "net" {
			entries, _ := os.ReadDir(path)
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
		}
		return nil
	})
	return uniqueSorted(names)
}

func netIfaceState(name string) string {
	b, err := os.ReadFile(filepath.Join("/sys/class/net", name, "operstate"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(b))
}

func detectDataMode(mbimCount, netCount int) domain.DataMode {
	switch {
	case mbimCount > 0 && netCount > 0:
		return domain.DataModeMixed
	case mbimCount > 0:
		return domain.DataModeMBIM
	case netCount > 0:
		return domain.DataModeRNDIS
	default:
		return domain.DataModeATOnly
	}
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
	m := ProbeATPortsCached([]string{path}, skipPath)
	return m[path]
}

// ProbeATPortsCached probes many serial paths concurrently (bounded), using the
// same TTL cache as ProbeATPortCached. Distinct tty nodes open independently;
// skipPath (manager's open AT port) is never opened (assumed ready).
func ProbeATPortsCached(paths []string, skipPath string) map[string]bool {
	out := make(map[string]bool, len(paths))
	if len(paths) == 0 {
		return out
	}

	now := time.Now()
	var need []string
	seen := make(map[string]struct{}, len(paths))

	probeMu.Lock()
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		if skipPath != "" && path == skipPath {
			out[path] = true
			continue
		}
		if e, ok := probeCache[path]; ok && now.Before(e.expiresAt) {
			out[path] = e.ok
			continue
		}
		need = append(need, path)
	}
	probeMu.Unlock()

	if len(need) == 0 {
		return out
	}

	type result struct {
		path string
		ok   bool
	}
	resCh := make(chan result, len(need))
	sem := make(chan struct{}, probeParallelism)
	var wg sync.WaitGroup
	for _, path := range need {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ok := ProbeATPort(p)
			probeMu.Lock()
			probeCache[p] = probeEntry{ok: ok, expiresAt: time.Now().Add(probeTTL)}
			probeMu.Unlock()
			resCh <- result{path: p, ok: ok}
		}(path)
	}
	go func() {
		wg.Wait()
		close(resCh)
	}()
	for r := range resCh {
		out[r.path] = r.ok
	}
	return out
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

	// Pass 1: cheap sysfs discovery; collect all tty paths for one parallel AT probe.
	type usbDraft struct {
		sys        string
		pwr        domain.PowerControl
		ttys       []string
		mbimIfaces []domain.ModemInterface
		netIfaces  []domain.ModemInterface
	}
	drafts := make([]usbDraft, 0, len(usbPaths))
	var allTTY []string

	for _, sys := range usbPaths {
		pwr := domain.PowerUnknown
		if b, err := os.ReadFile(filepath.Join(sys, "power", "control")); err == nil {
			pwr = domain.NormalizePowerControl(string(b))
		}
		ttys := ttyNodesUnder(sys)
		mbims := mbimNodesUnder(sys)
		nets := netIfacesUnder(sys)

		for _, t := range ttys {
			assignedTTY[t] = struct{}{}
			allTTY = append(allTTY, t)
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

		// Net state/addrs are process-bound but cheap; parallelize if many ifaces.
		netIfaces := buildNetIfacesParallel(nets)

		drafts = append(drafts, usbDraft{
			sys:        sys,
			pwr:        pwr,
			ttys:       ttys,
			mbimIfaces: mbimIfaces,
			netIfaces:  netIfaces,
		})
	}

	// Serial-only fallbacks not under matched USB device.
	type serialDraft struct {
		path string
	}
	var serials []serialDraft
	candidates, _ := ListCandidatePorts(vendor, product)
	for _, c := range candidates {
		if _, ok := assignedTTY[c]; ok {
			continue
		}
		if st, err := os.Stat(c); err != nil || st.IsDir() {
			continue
		}
		serials = append(serials, serialDraft{path: c})
		allTTY = append(allTTY, c)
		assignedTTY[c] = struct{}{}
	}

	// One bounded-parallel probe for every candidate port.
	readyMap := ProbeATPortsCached(allTTY, openATPort)

	// Pass 2: assemble modem list with ATReady labels.
	var modems []domain.ModemDevice
	for _, d := range drafts {
		var atIfaces []domain.ModemInterface
		for _, t := range d.ttys {
			ready := readyMap[t]
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

		base := filepath.Base(d.sys)
		gotV, gotP := readUSBID(d.sys)
		if gotV == "" {
			gotV = vendor
		}
		if gotP == "" {
			gotP = product
		}
		name := fmt.Sprintf("Fibocom FM350-GL @ %s", base)
		if len(usbPaths) == 1 {
			name = "Fibocom FM350-GL"
		}
		mode := detectDataMode(len(d.mbimIfaces), len(d.netIfaces))
		switch mode {
		case domain.DataModeRNDIS:
			name += " [RNDIS]"
		case domain.DataModeMBIM:
			name += " [MBIM]"
		case domain.DataModeATOnly:
			name += " [AT-only]"
		}
		if gotP != "" && gotP != product {
			name += " (" + gotV + ":" + gotP + ")"
		}
		modems = append(modems, domain.ModemDevice{
			ID:           "usb:" + d.sys,
			Name:         name,
			VendorID:     gotV,
			ProductID:    gotP,
			SysPath:      d.sys,
			Connected:    true,
			PowerControl: d.pwr,
			DataMode:     mode,
			ATPorts:      atIfaces,
			MBIMNodes:    d.mbimIfaces,
			NetIfaces:    d.netIfaces,
		})
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

	for _, s := range serials {
		ready := readyMap[s.path]
		label := s.path
		if ready {
			label = s.path + " (AT OK)"
		}
		modems = append(modems, domain.ModemDevice{
			ID:        "serial:" + s.path,
			Name:      "Serial " + s.path,
			Connected: true,
			ATPorts: []domain.ModemInterface{{
				Path: s.path, Kind: domain.IfaceKindAT, ATReady: ready, Label: label,
			}},
		})
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

// buildNetIfacesParallel fills RNDIS/net iface metadata; parallel when several ifaces.
func buildNetIfacesParallel(nets []string) []domain.ModemInterface {
	if len(nets) == 0 {
		return nil
	}
	out := make([]domain.ModemInterface, len(nets))
	if len(nets) == 1 {
		n := nets[0]
		st := netIfaceState(n)
		label := fmt.Sprintf("%s (RNDIS/net, %s)", n, st)
		if addrs := NetIfaceAddrs(n); len(addrs) > 0 {
			label = fmt.Sprintf("%s (RNDIS/net, %s, %s)", n, st, strings.Join(addrs, ", "))
		}
		out[0] = domain.ModemInterface{Path: n, Kind: domain.IfaceKindNet, Label: label, State: st}
		return out
	}
	var wg sync.WaitGroup
	for i, n := range nets {
		wg.Add(1)
		go func(i int, n string) {
			defer wg.Done()
			st := netIfaceState(n)
			label := fmt.Sprintf("%s (RNDIS/net, %s)", n, st)
			if addrs := NetIfaceAddrs(n); len(addrs) > 0 {
				label = fmt.Sprintf("%s (RNDIS/net, %s, %s)", n, st, strings.Join(addrs, ", "))
			}
			out[i] = domain.ModemInterface{Path: n, Kind: domain.IfaceKindNet, Label: label, State: st}
		}(i, n)
	}
	wg.Wait()
	return out
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
