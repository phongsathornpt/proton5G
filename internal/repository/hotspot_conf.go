package repository

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

// GenerateHostapdConf builds a minimal WPA2-PSK hostapd configuration.
func GenerateHostapdConf(cfg domain.HotspotConfig) string {
	ch := cfg.Channel
	if ch <= 0 {
		if cfg.Band == domain.HotspotBand5 {
			ch = 36
		} else {
			ch = appdefaults.HotspotChannel
		}
	}
	hwMode := "g"
	if cfg.Band == domain.HotspotBand5 {
		hwMode = "a"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "interface=%s\n", cfg.WlanIface)
	b.WriteString("driver=nl80211\n")
	fmt.Fprintf(&b, "ssid=%s\n", cfg.SSID)
	fmt.Fprintf(&b, "hw_mode=%s\n", hwMode)
	fmt.Fprintf(&b, "channel=%d\n", ch)
	b.WriteString("auth_algs=1\n")
	b.WriteString("wpa=2\n")
	b.WriteString("wpa_key_mgmt=WPA-PSK\n")
	b.WriteString("rsn_pairwise=CCMP\n")
	fmt.Fprintf(&b, "wpa_passphrase=%s\n", cfg.Password)
	b.WriteString("ignore_broadcast_ssid=0\n")
	if c := strings.TrimSpace(cfg.Country); c != "" {
		fmt.Fprintf(&b, "country_code=%s\n", strings.ToUpper(c))
	}
	return b.String()
}

// GenerateDnsmasqConf builds a bind-interfaces DHCP config for the AP LAN.
// Requested start/end are preserved only when both are usable inside cfg.LANCIDR;
// otherwise a safe pool is derived from that LAN network.
func GenerateDnsmasqConf(cfg domain.HotspotConfig, dhcpStart, dhcpEnd, leaseFile string) (string, error) {
	ip, ipNet, err := net.ParseCIDR(cfg.LANCIDR)
	if err != nil {
		return "", err
	}
	start, end, err := hotspotDHCPRange(ip, ipNet, dhcpStart, dhcpEnd)
	if err != nil {
		return "", err
	}
	gw := ip.String()
	var b strings.Builder
	fmt.Fprintf(&b, "interface=%s\n", cfg.WlanIface)
	b.WriteString("bind-interfaces\n")
	b.WriteString("except-interface=lo\n")
	fmt.Fprintf(&b, "dhcp-range=%s,%s,12h\n", start, end)
	fmt.Fprintf(&b, "dhcp-option=3,%s\n", gw)
	fmt.Fprintf(&b, "dhcp-option=6,%s\n", gw)
	if leaseFile != "" {
		fmt.Fprintf(&b, "dhcp-leasefile=%s\n", leaseFile)
	}
	b.WriteString("log-dhcp\n")
	return b.String(), nil
}

func hotspotDHCPRange(gateway net.IP, network *net.IPNet, requestedStart, requestedEnd string) (string, string, error) {
	gw4 := gateway.To4()
	net4 := network.IP.To4()
	if gw4 == nil || net4 == nil || len(network.Mask) != net.IPv4len {
		return "", "", fmt.Errorf("hotspot LAN must be IPv4")
	}
	base := binary.BigEndian.Uint32(net4)
	mask := binary.BigEndian.Uint32(network.Mask)
	last := base | ^mask
	if last-base < 3 {
		return "", "", fmt.Errorf("hotspot LAN has no usable DHCP pool")
	}
	firstHost, lastHost := base+1, last-1
	gw := binary.BigEndian.Uint32(gw4)

	parseRequested := func(raw string) (uint32, bool) {
		ip := net.ParseIP(strings.TrimSpace(raw)).To4()
		if ip == nil || !network.Contains(ip) {
			return 0, false
		}
		v := binary.BigEndian.Uint32(ip)
		return v, v >= firstHost && v <= lastHost && v != gw
	}
	if start, okStart := parseRequested(requestedStart); okStart {
		if end, okEnd := parseRequested(requestedEnd); okEnd && start <= end && !(gw >= start && gw <= end) {
			return uint32IPv4(start), uint32IPv4(end), nil
		}
	}

	start := base + 10
	if start > lastHost {
		start = firstHost
	}
	end := base + 200
	if end > lastHost {
		end = lastHost
	}
	if gw >= start && gw <= end {
		left, right := gw-start, end-gw
		if right >= left && gw < end {
			start = gw + 1
		} else if gw > start {
			end = gw - 1
		} else {
			return "", "", fmt.Errorf("hotspot LAN has no DHCP range excluding gateway")
		}
	}
	if start > end {
		return "", "", fmt.Errorf("hotspot LAN has no usable DHCP range")
	}
	return uint32IPv4(start), uint32IPv4(end), nil
}

func uint32IPv4(v uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return net.IP(b[:]).String()
}
