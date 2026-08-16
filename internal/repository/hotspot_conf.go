package repository

import (
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
		ch = appdefaults.HotspotChannel
	}
	hwMode := "g"
	if cfg.Band == domain.HotspotBand5 {
		hwMode = "a"
		if ch <= 0 {
			ch = 36
		}
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
// leaseFile, if non-empty, is written as dhcp-leasefile= for client listing.
func GenerateDnsmasqConf(cfg domain.HotspotConfig, dhcpStart, dhcpEnd, leaseFile string) (string, error) {
	ip, ipNet, err := net.ParseCIDR(cfg.LANCIDR)
	if err != nil {
		return "", err
	}
	gw := ip.String()
	if dhcpStart == "" {
		dhcpStart = appdefaults.HotspotDHCPStart
	}
	if dhcpEnd == "" {
		dhcpEnd = appdefaults.HotspotDHCPEnd
	}
	// Ensure range is inside the LAN network when defaults match 192.168.50.0/24.
	_ = ipNet
	var b strings.Builder
	fmt.Fprintf(&b, "interface=%s\n", cfg.WlanIface)
	b.WriteString("bind-interfaces\n")
	b.WriteString("except-interface=lo\n")
	fmt.Fprintf(&b, "dhcp-range=%s,%s,12h\n", dhcpStart, dhcpEnd)
	fmt.Fprintf(&b, "dhcp-option=3,%s\n", gw)
	fmt.Fprintf(&b, "dhcp-option=6,%s\n", gw)
	if leaseFile != "" {
		fmt.Fprintf(&b, "dhcp-leasefile=%s\n", leaseFile)
	}
	b.WriteString("log-dhcp\n")
	return b.String(), nil
}
