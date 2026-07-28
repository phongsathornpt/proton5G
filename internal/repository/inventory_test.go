package repository

import (
	"os"
	"path/filepath"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestListUSBDevicesTemp(t *testing.T) {
	// Uses real SysfsUSBGlob — just ensure function doesn't panic.
	_ = ListUSBDevices(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
}

func TestTTYAndMBIMWalk(t *testing.T) {
	root := t.TempDir()
	// Fake tree: root/1-1/…/ttyUSB2 and cdc-wdm0
	ttyDir := filepath.Join(root, "1-1", "1-1:1.0", "tty")
	if err := os.MkdirAll(ttyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(ttyDir, "ttyUSB9"), 0755); err != nil {
		t.Fatal(err)
	}
	wdmDir := filepath.Join(root, "1-1", "1-1:1.2", "usbmisc", "cdc-wdm9")
	if err := os.MkdirAll(wdmDir, 0755); err != nil {
		t.Fatal(err)
	}

	ttys := ttyNodesUnder(filepath.Join(root, "1-1"))
	if len(ttys) != 1 || ttys[0] != "/dev/ttyUSB9" {
		t.Fatalf("ttys=%v", ttys)
	}
	// mbimNodesUnder only adds if /dev/cdc-wdm9 exists — may not on CI
	_ = mbimNodesUnder(filepath.Join(root, "1-1"))
}

func TestPreferredATPort(t *testing.T) {
	m := domain.ModemDevice{
		ATPorts: []domain.ModemInterface{
			{Path: "/dev/ttyUSB0", ATReady: false},
			{Path: "/dev/ttyUSB2", ATReady: true},
		},
	}
	if p := PreferredATPort(m, ""); p != "/dev/ttyUSB2" {
		t.Fatalf("got %s", p)
	}
	if p := PreferredATPort(m, "/dev/ttyUSB0"); p != "/dev/ttyUSB0" {
		t.Fatalf("preferred got %s", p)
	}
}

func TestFindModem(t *testing.T) {
	list := []domain.ModemDevice{{ID: "a"}, {ID: "b"}}
	if _, ok := FindModem(list, "b"); !ok {
		t.Fatal("expected find")
	}
	if _, ok := FindModem(list, "z"); ok {
		t.Fatal("expected miss")
	}
}
