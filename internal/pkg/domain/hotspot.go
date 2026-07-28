package domain

import (
	"fmt"
	"net"
	"strings"
	"unicode/utf8"
)

// Hotspot runtime states (JSON-stable for WebUI).
const (
	HotspotStateStopped  = "stopped"
	HotspotStateStarting = "starting"
	HotspotStateRunning  = "running"
	HotspotStateStopping = "stopping"
	HotspotStateError    = "error"
)

// Hotspot bands.
const (
	HotspotBand24 = "2.4"
	HotspotBand5  = "5"
)

// HotspotConfig is the desired software AP configuration.
type HotspotConfig struct {
	Enabled     bool   `json:"enabled"`
	SSID        string `json:"ssid"`
	Password    string `json:"password"`
	WlanIface   string `json:"wlan_iface"`
	UplinkIface string `json:"uplink_iface,omitempty"` // empty = selected RNDIS net
	Channel     int    `json:"channel"`               // 0 = default
	Band        string `json:"band"`                  // "2.4" | "5"
	LANCIDR     string `json:"lan_cidr"`              // e.g. 192.168.50.1/24
	Country     string `json:"country,omitempty"`
}

// WiFiDevice is a host wireless interface candidate for AP mode.
type WiFiDevice struct {
	Iface      string   `json:"iface"`
	Phy        string   `json:"phy,omitempty"`
	Driver     string   `json:"driver,omitempty"`
	OperState  string   `json:"oper_state,omitempty"`
	SupportsAP bool     `json:"supports_ap"`
	APKnown    bool     `json:"ap_known"` // false when iw missing / AP capability unverified
	Modes      []string `json:"modes,omitempty"`
	Label      string   `json:"label"`
}

// HotspotTools reports which host binaries are available.
type HotspotTools struct {
	Hostapd  bool `json:"hostapd"`
	Dnsmasq  bool `json:"dnsmasq"`
	Iw       bool `json:"iw"`
	IP       bool `json:"ip"`
	Nftables bool `json:"nftables"`
	Iptables bool `json:"iptables"`
}

// HotspotDiagnostics is a host WiFi / tools snapshot for debugging AP start.
type HotspotDiagnostics struct {
	Tools       HotspotTools `json:"tools"`
	InstallHint string       `json:"install_hint,omitempty"`
	Interfaces  []WiFiDevice `json:"interfaces"`
	Notes       []string     `json:"notes,omitempty"`
}

// HotspotClient is an associated station (optional / best-effort).
type HotspotClient struct {
	MAC  string `json:"mac,omitempty"`
	IP   string `json:"ip,omitempty"`
	Name string `json:"name,omitempty"`
}

// HotspotStatus is the WebUI/API snapshot for the software AP.
type HotspotStatus struct {
	State        string              `json:"state"`
	Config       HotspotConfig       `json:"config"`
	Uplink       string              `json:"uplink_iface,omitempty"`
	UplinkAddrs  []string            `json:"uplink_addrs,omitempty"`
	LANAddrs     []string            `json:"lan_addrs,omitempty"`
	Tools        HotspotTools        `json:"tools"`
	InstallHint  string              `json:"install_hint,omitempty"`
	Devices      []WiFiDevice        `json:"devices,omitempty"`
	Diagnostics  HotspotDiagnostics  `json:"diagnostics,omitempty"`
	Clients      []HotspotClient     `json:"clients,omitempty"`
	Error        string              `json:"error,omitempty"`
	Note         string              `json:"note,omitempty"`
}

// HotspotStartRequest optionally overrides config fields for start.
type HotspotStartRequest struct {
	SSID        string `json:"ssid,omitempty"`
	Password    string `json:"password,omitempty"`
	WlanIface   string `json:"wlan_iface,omitempty"`
	UplinkIface string `json:"uplink_iface,omitempty"`
	Channel     int    `json:"channel,omitempty"`
	Band        string `json:"band,omitempty"`
	LANCIDR     string `json:"lan_cidr,omitempty"`
	Country     string `json:"country,omitempty"`
}

// RedactedConfig returns a copy with password masked for API responses.
func (c HotspotConfig) RedactedConfig() HotspotConfig {
	out := c
	if out.Password != "" {
		out.Password = "********"
	}
	return out
}

// ValidateHotspotConfig checks SSID/password/iface/LAN for WPA2 AP start.
func ValidateHotspotConfig(cfg HotspotConfig) error {
	ssid := strings.TrimSpace(cfg.SSID)
	if ssid == "" {
		return fmt.Errorf("ssid is required")
	}
	if n := utf8.RuneCountInString(ssid); n > 32 {
		return fmt.Errorf("ssid too long (%d > 32)", n)
	}
	pass := cfg.Password
	if len(pass) < 8 || len(pass) > 63 {
		return fmt.Errorf("password must be 8–63 characters (WPA2-PSK)")
	}
	if err := ValidateIfaceName(cfg.WlanIface); err != nil {
		return fmt.Errorf("wlan_iface: %w", err)
	}
	if cfg.UplinkIface != "" {
		if err := ValidateIfaceName(cfg.UplinkIface); err != nil {
			return fmt.Errorf("uplink_iface: %w", err)
		}
	}
	band := cfg.Band
	if band == "" {
		band = HotspotBand24
	}
	if band != HotspotBand24 && band != HotspotBand5 {
		return fmt.Errorf("band must be %q or %q", HotspotBand24, HotspotBand5)
	}
	ch := cfg.Channel
	if ch < 0 {
		return fmt.Errorf("invalid channel %d", ch)
	}
	if band == HotspotBand24 && ch > 14 {
		return fmt.Errorf("channel %d invalid for 2.4 GHz", ch)
	}
	if band == HotspotBand5 && ch > 0 && ch < 36 {
		return fmt.Errorf("channel %d looks invalid for 5 GHz", ch)
	}
	cidr := strings.TrimSpace(cfg.LANCIDR)
	if cidr == "" {
		return fmt.Errorf("lan_cidr is required")
	}
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("lan_cidr: %w", err)
	}
	if ip.To4() == nil {
		return fmt.Errorf("lan_cidr must be IPv4")
	}
	if !ipNet.Contains(ip) {
		return fmt.Errorf("lan_cidr host not in network")
	}
	if cfg.Country != "" {
		c := strings.ToUpper(strings.TrimSpace(cfg.Country))
		if len(c) != 2 {
			return fmt.Errorf("country must be ISO 2-letter code")
		}
	}
	return nil
}

// ValidateIfaceName rejects empty or unsafe Linux interface names.
func ValidateIfaceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("empty")
	}
	if len(name) > 15 {
		return fmt.Errorf("too long")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return fmt.Errorf("invalid character in %q", name)
	}
	return nil
}
