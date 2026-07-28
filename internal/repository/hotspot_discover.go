package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

// commonSbinPaths are tried when PATH omits sbin (non-root or minimal env).
var commonSbinPaths = []string{
	"/usr/sbin",
	"/sbin",
	"/usr/local/sbin",
}

// HotspotToolsPresent reports which hotspot-related binaries exist.
func HotspotToolsPresent() domain.HotspotTools {
	return domain.HotspotTools{
		Hostapd:  toolExists("hostapd"),
		Dnsmasq:  toolExists("dnsmasq"),
		Iw:       toolExists("iw"),
		IP:       toolExists("ip"),
		Nftables: toolExists("nft"),
		Iptables: toolExists("iptables"),
	}
}

// HotspotInstallHint returns an apt-oriented install line for missing tools.
func HotspotInstallHint(t domain.HotspotTools) string {
	var miss []string
	if !t.Hostapd {
		miss = append(miss, "hostapd")
	}
	if !t.Dnsmasq {
		miss = append(miss, "dnsmasq")
	}
	if !t.Iw {
		miss = append(miss, "iw")
	}
	if !t.IP {
		miss = append(miss, "iproute2")
	}
	if !t.Nftables && !t.Iptables {
		miss = append(miss, "nftables")
	}
	if len(miss) == 0 {
		return ""
	}
	return "sudo apt-get install -y " + strings.Join(miss, " ")
}

func toolExists(name string) bool {
	return resolveTool(name) != ""
}

func lookPath(name string) bool {
	return toolExists(name)
}

// resolveTool returns an absolute path or bare name found on PATH / sbin.
func resolveTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range commonSbinPaths {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// ListWiFiDevices finds wireless netdevs and AP capability (best-effort).
func ListWiFiDevices() []domain.WiFiDevice {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	iwOK := toolExists("iw")
	var out []domain.WiFiDevice
	for _, e := range entries {
		name := e.Name()
		if !isWirelessIface(name) {
			continue
		}
		dev := domain.WiFiDevice{
			Iface:     name,
			Phy:       wirelessPhy(name),
			Driver:    wirelessDriver(name),
			OperState: readSysfsTrim(filepath.Join("/sys/class/net", name, "operstate")),
		}
		if iwOK {
			modes, ap := wifiAPModes(dev.Phy, name)
			dev.Modes = modes
			dev.SupportsAP = ap
			dev.APKnown = true
			if ap {
				dev.Label = name + " (AP capable"
			} else {
				dev.Label = name + " (no AP"
			}
		} else {
			// Without iw we cannot verify AP; allow selection and verify at start.
			dev.SupportsAP = true
			dev.APKnown = false
			dev.Label = name + " (AP unknown — install iw"
		}
		if dev.Driver != "" {
			dev.Label += ", " + dev.Driver
		}
		if dev.OperState != "" {
			dev.Label += ", " + dev.OperState
		}
		dev.Label += ")"
		out = append(out, dev)
	}
	return out
}

// CollectHotspotDiagnostics builds a debug snapshot for the WebUI/API.
func CollectHotspotDiagnostics() domain.HotspotDiagnostics {
	tools := HotspotToolsPresent()
	devs := ListWiFiDevices()
	d := domain.HotspotDiagnostics{
		Tools:       tools,
		InstallHint: HotspotInstallHint(tools),
		Interfaces:  devs,
	}
	if !tools.Iw {
		d.Notes = append(d.Notes, "iw missing: AP capability not verified; install iw for accurate mode detection")
	}
	if !tools.Hostapd {
		d.Notes = append(d.Notes, "hostapd missing: required to start software AP")
	}
	if !tools.Dnsmasq {
		d.Notes = append(d.Notes, "dnsmasq missing: required for client DHCP")
	}
	if len(devs) == 0 {
		d.Notes = append(d.Notes, "no wireless interfaces found under /sys/class/net")
	}
	for _, dev := range devs {
		if dev.Driver == "iwlwifi" {
			d.Notes = append(d.Notes, "Intel iwlwifi detected: prefer 2.4 GHz AP (channel 1–11); 5 GHz AP often unsupported")
			break
		}
	}
	return d
}

func isWirelessIface(name string) bool {
	if _, err := os.Stat(filepath.Join("/sys/class/net", name, "wireless")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join("/sys/class/net", name, "phy80211")); err == nil {
		return true
	}
	return false
}

func wirelessPhy(iface string) string {
	link := filepath.Join("/sys/class/net", iface, "phy80211")
	target, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func wirelessDriver(iface string) string {
	// Prefer device/driver; fall back to phy device driver.
	for _, p := range []string{
		filepath.Join("/sys/class/net", iface, "device", "driver"),
		filepath.Join("/sys/class/net", iface, "phy80211", "device", "driver"),
	} {
		target, err := os.Readlink(p)
		if err == nil {
			return filepath.Base(target)
		}
	}
	return ""
}

func readSysfsTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func wifiAPModes(phy, iface string) (modes []string, supportsAP bool) {
	iwBin := resolveTool("iw")
	if iwBin == "" {
		return nil, false
	}
	var text string
	if phy != "" {
		if out, err := runCmd(3*time.Second, iwBin, "phy", phy, "info"); err == nil {
			text = out
		}
	}
	if text == "" {
		if out, err := runCmd(3*time.Second, iwBin, "list"); err == nil {
			text = out
		}
	}
	if text == "" {
		if out, err := runCmd(3*time.Second, iwBin, "dev", iface, "info"); err == nil {
			text = out
		}
	}
	return parseIWListAP(text)
}

// parseIWListAP extracts interface modes from iw phy/list output.
func parseIWListAP(text string) (modes []string, supportsAP bool) {
	inModes := false
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(line, "Supported interface modes:") {
			inModes = true
			continue
		}
		if inModes {
			if strings.HasPrefix(trim, "*") {
				mode := strings.TrimSpace(strings.TrimPrefix(trim, "*"))
				if mode != "" {
					modes = append(modes, mode)
					if mode == "AP" || strings.EqualFold(mode, "AP") {
						supportsAP = true
					}
				}
			} else if trim != "" && !strings.HasPrefix(trim, "*") {
				inModes = false
			}
		}
	}
	if !supportsAP && strings.Contains(text, "* AP") {
		supportsAP = true
		if len(modes) == 0 {
			modes = append(modes, "AP")
		}
	}
	return modes, supportsAP
}
