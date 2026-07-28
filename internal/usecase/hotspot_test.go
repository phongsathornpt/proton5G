package usecase

import (
	"errors"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

type fakeHotspot struct {
	running bool
	tools   domain.HotspotTools
	devs    []domain.WiFiDevice
	startFn func(domain.HotspotConfig, string) (string, error)
	lastUp  string
}

func (f *fakeHotspot) ListWiFiDevices() []domain.WiFiDevice { return f.devs }
func (f *fakeHotspot) Tools() domain.HotspotTools {
	if f.tools.Hostapd || f.tools.Dnsmasq {
		return f.tools
	}
	return domain.HotspotTools{Hostapd: true, Dnsmasq: true, Iw: true, IP: true, Nftables: true}
}
func (f *fakeHotspot) Start(cfg domain.HotspotConfig, uplink string) (string, error) {
	f.lastUp = uplink
	if f.startFn != nil {
		out, err := f.startFn(cfg, uplink)
		if err == nil {
			f.running = true
		}
		return out, err
	}
	f.running = true
	return "ok", nil
}
func (f *fakeHotspot) Stop() (string, error) {
	f.running = false
	return "stopped", nil
}
func (f *fakeHotspot) IsRunning() bool { return f.running }
func (f *fakeHotspot) StatusExtras() ([]string, []string, []domain.HotspotClient) {
	return []string{"192.168.50.1"}, []string{"10.0.0.2"}, nil
}

func TestHotspotStartRequiresUplink(t *testing.T) {
	hs := &fakeHotspot{devs: []domain.WiFiDevice{{Iface: "wlan0", SupportsAP: true}}}
	svc := NewModemService(ModemServiceConfig{
		USB:     &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:      &fakeAT{port: "/dev/ttyUSB0"},
		Hotspot: hs,
	})
	svc.hotspotCfg.SSID = "Test"
	svc.hotspotCfg.Password = "password1"
	svc.hotspotCfg.WlanIface = "wlan0"

	_, err := svc.HotspotStart(domain.HotspotStartRequest{})
	if err == nil {
		t.Fatal("expected missing uplink error")
	}
}

func TestHotspotStartUsesSelectedNet(t *testing.T) {
	hs := &fakeHotspot{}
	svc := NewModemService(ModemServiceConfig{
		USB:     &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:      &fakeAT{port: "/dev/ttyUSB0"},
		Hotspot: hs,
		Net:     &fakeNet{},
	})
	svc.selectedNet = "enxabc"
	svc.hotspotCfg.SSID = "Test"
	svc.hotspotCfg.Password = "password1"
	svc.hotspotCfg.WlanIface = "wlan0"

	st, err := svc.HotspotStart(domain.HotspotStartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if hs.lastUp != "enxabc" {
		t.Fatalf("uplink=%s", hs.lastUp)
	}
	if st.State != domain.HotspotStateRunning {
		t.Fatalf("state=%s", st.State)
	}
}

func TestHotspotStartValidation(t *testing.T) {
	svc := NewModemService(ModemServiceConfig{
		USB:     &fakeUSB{},
		AT:      &fakeAT{},
		Hotspot: &fakeHotspot{},
	})
	svc.selectedNet = "enx1"
	_, err := svc.HotspotStart(domain.HotspotStartRequest{
		SSID: "x", Password: "short", WlanIface: "wlan0",
	})
	if err == nil {
		t.Fatal("expected short password error")
	}
}

func TestHotspotStop(t *testing.T) {
	hs := &fakeHotspot{running: true}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{}, AT: &fakeAT{}, Hotspot: hs,
	})
	svc.hotspotState = domain.HotspotStateRunning
	st, err := svc.HotspotStop()
	if err != nil {
		t.Fatal(err)
	}
	if hs.running || st.State != domain.HotspotStateStopped {
		t.Fatalf("running=%v state=%s", hs.running, st.State)
	}
}

func TestHotspotStartErrorState(t *testing.T) {
	hs := &fakeHotspot{
		startFn: func(domain.HotspotConfig, string) (string, error) {
			return "", errors.New("hostapd failed")
		},
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{}, AT: &fakeAT{}, Hotspot: hs,
	})
	svc.selectedNet = "enx1"
	svc.hotspotCfg = domain.HotspotConfig{
		SSID: "T", Password: "password1", WlanIface: "wlan0",
		Channel: 6, Band: domain.HotspotBand24, LANCIDR: "192.168.50.1/24",
	}
	st, err := svc.HotspotStart(domain.HotspotStartRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if st.State != domain.HotspotStateError {
		t.Fatalf("state=%s", st.State)
	}
}

func TestHotspotSetConfigKeepsPasswordOnRedact(t *testing.T) {
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{}, AT: &fakeAT{}, Hotspot: &fakeHotspot{},
	})
	svc.hotspotCfg.Password = "password1"
	err := svc.HotspotSetConfig(domain.HotspotConfig{
		SSID: "Keep", Password: "********", WlanIface: "wlan0",
		LANCIDR: "192.168.50.1/24", Band: "2.4", Channel: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.hotspotCfg.Password != "password1" {
		t.Fatalf("password overwritten: %q", svc.hotspotCfg.Password)
	}
}
