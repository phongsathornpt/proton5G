package repository

import (
	"strings"
	"testing"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

func TestGenerateHostapdConf(t *testing.T) {
	cfg := domain.HotspotConfig{
		SSID:      "TestSSID",
		Password:  "password1",
		WlanIface: "wlan0",
		Channel:   6,
		Band:      domain.HotspotBand24,
		Country:   "TH",
	}
	out := GenerateHostapdConf(cfg)
	for _, want := range []string{
		"interface=wlan0",
		"ssid=TestSSID",
		"hw_mode=g",
		"channel=6",
		"wpa=2",
		"wpa_passphrase=password1",
		"country_code=TH",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGenerateHostapdConf5G(t *testing.T) {
	out := GenerateHostapdConf(domain.HotspotConfig{
		SSID: "x", Password: "password1", WlanIface: "wlan1",
		Band: domain.HotspotBand5, Channel: 36,
	})
	if !strings.Contains(out, "hw_mode=a") {
		t.Fatal(out)
	}
}

func TestGenerateDnsmasqConf(t *testing.T) {
	out, err := GenerateDnsmasqConf(domain.HotspotConfig{
		WlanIface: "wlan0",
		LANCIDR:   "192.168.50.1/24",
	}, "192.168.50.10", "192.168.50.200", "/run/fm350-manager/dnsmasq.leases")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"interface=wlan0",
		"bind-interfaces",
		"dhcp-range=192.168.50.10,192.168.50.200,12h",
		"dhcp-option=3,192.168.50.1",
		"dhcp-option=6,192.168.50.1",
		"dhcp-leasefile=/run/fm350-manager/dnsmasq.leases",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGenerateDnsmasqConfDerivesRangeForCustomLAN(t *testing.T) {
	out, err := GenerateDnsmasqConf(domain.HotspotConfig{
		WlanIface: "wlan0", LANCIDR: "10.20.30.1/24",
	}, appdefaults.HotspotDHCPStart, appdefaults.HotspotDHCPEnd, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dhcp-range=10.20.30.10,10.20.30.200,12h") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "dhcp-option=3,10.20.30.1") {
		t.Fatal(out)
	}
}

func TestGenerateDnsmasqConfSmallSubnetAvoidsGateway(t *testing.T) {
	out, err := GenerateDnsmasqConf(domain.HotspotConfig{
		WlanIface: "wlan0", LANCIDR: "10.20.30.1/29",
	}, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dhcp-range=10.20.30.2,10.20.30.6,12h") {
		t.Fatal(out)
	}
}

func TestGenerateHostapdConf5GDefaultChannel(t *testing.T) {
	out := GenerateHostapdConf(domain.HotspotConfig{
		SSID: "x", Password: "password1", WlanIface: "wlan1",
		Band: domain.HotspotBand5,
	})
	if !strings.Contains(out, "channel=36") {
		t.Fatal(out)
	}
}

func TestHotspotLANNetwork(t *testing.T) {
	got, err := hotspotLANNetwork("192.168.50.1/24")
	if err != nil || got != "192.168.50.0/24" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
