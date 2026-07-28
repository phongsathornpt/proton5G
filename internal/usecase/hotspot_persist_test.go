package usecase

import (
	"os"
	"path/filepath"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestHotspotConfigFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hotspot.json")
	in := domain.HotspotConfig{
		SSID: "Lab", Password: "password1", WlanIface: "wlan0",
		Channel: 6, Band: "2.4", LANCIDR: "192.168.50.1/24",
	}
	if err := saveHotspotConfigFile(path, in); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	out, err := loadHotspotConfigFile(path)
	if err != nil || out.SSID != "Lab" || out.Password != "password1" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestLoadHotspotConfigFileIntoService(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hs.json")
	_ = saveHotspotConfigFile(path, domain.HotspotConfig{
		SSID: "Saved", Password: "password1", WlanIface: "wlan1",
		Band: "2.4", Channel: 11, LANCIDR: "192.168.50.1/24",
	})
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{}, AT: &fakeAT{}, Hotspot: &fakeHotspot{},
		HotspotConfigFile: path,
	})
	if err := svc.LoadHotspotConfigFile(); err != nil {
		t.Fatal(err)
	}
	if svc.hotspotCfg.SSID != "Saved" || svc.hotspotCfg.WlanIface != "wlan1" {
		t.Fatalf("%+v", svc.hotspotCfg)
	}
}
