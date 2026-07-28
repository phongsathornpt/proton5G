package repository

import (
	"strings"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestHotspotInstallHint(t *testing.T) {
	h := HotspotInstallHint(domain.HotspotTools{})
	if h == "" || !strings.Contains(h, "hostapd") || !strings.Contains(h, "dnsmasq") || !strings.Contains(h, "iw") {
		t.Fatalf("hint=%q", h)
	}
	if HotspotInstallHint(domain.HotspotTools{
		Hostapd: true, Dnsmasq: true, Iw: true, IP: true, Nftables: true,
	}) != "" {
		t.Fatal("expected empty hint when all tools present")
	}
}

func TestParseIWListAP(t *testing.T) {
	sample := `
Wiphy phy0
	Supported interface modes:
		 * IBSS
		 * managed
		 * AP
		 * AP/VLAN
		 * monitor
	Band 1:
`
	modes, ap := parseIWListAP(sample)
	if !ap {
		t.Fatal("expected AP support")
	}
	found := false
	for _, m := range modes {
		if m == "AP" {
			found = true
		}
	}
	if !found {
		t.Fatalf("modes=%v", modes)
	}
}

func TestParseIWListNoAP(t *testing.T) {
	sample := `
	Supported interface modes:
		 * managed
		 * monitor
`
	_, ap := parseIWListAP(sample)
	if ap {
		t.Fatal("expected no AP")
	}
}
