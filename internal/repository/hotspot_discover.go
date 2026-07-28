package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

// HotspotToolsPresent reports which hotspot-related binaries exist on PATH.
func HotspotToolsPresent() domain.HotspotTools {
	return domain.HotspotTools{
		Hostapd:  lookPath("hostapd"),
		Dnsmasq:  lookPath("dnsmasq"),
		Iw:       lookPath("iw"),
		IP:       lookPath("ip"),
		Nftables: lookPath("nft"),
		Iptables: lookPath("iptables"),
	}
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ListWiFiDevices finds wireless netdevs and whether they support AP mode.
func ListWiFiDevices() []domain.WiFiDevice {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	var out []domain.WiFiDevice
	for _, e := range entries {
		name := e.Name()
		if !isWirelessIface(name) {
			continue
		}
		dev := domain.WiFiDevice{
			Iface: name,
			Phy:   wirelessPhy(name),
		}
		modes, ap := wifiAPModes(dev.Phy, name)
		dev.Modes = modes
		dev.SupportsAP = ap
		if ap {
			dev.Label = name + " (AP capable)"
		} else {
			dev.Label = name + " (no AP / unknown)"
		}
		out = append(out, dev)
	}
	return out
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

func wifiAPModes(phy, iface string) (modes []string, supportsAP bool) {
	if !lookPath("iw") {
		return nil, false
	}
	var text string
	if phy != "" {
		if out, err := runCmd(3*time.Second, "iw", "phy", phy, "info"); err == nil {
			text = out
		}
	}
	if text == "" {
		if out, err := runCmd(3*time.Second, "iw", "list"); err == nil {
			text = out
		}
	}
	if text == "" {
		if out, err := runCmd(3*time.Second, "iw", "dev", iface, "info"); err == nil {
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
			if trim == "" || (!strings.HasPrefix(trim, "*") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ")) {
				// left the modes block
				if !strings.HasPrefix(trim, "*") {
					inModes = false
				}
			}
			if strings.HasPrefix(trim, "*") {
				mode := strings.TrimSpace(strings.TrimPrefix(trim, "*"))
				if mode != "" {
					modes = append(modes, mode)
					if mode == "AP" || strings.EqualFold(mode, "AP") {
						supportsAP = true
					}
				}
			} else if inModes && trim != "" && !strings.HasPrefix(trim, "*") {
				// next section
				inModes = false
			}
		}
	}
	// Fallback: any mention of " * AP" in file
	if !supportsAP && strings.Contains(text, "* AP") {
		supportsAP = true
		if len(modes) == 0 {
			modes = append(modes, "AP")
		}
	}
	return modes, supportsAP
}
