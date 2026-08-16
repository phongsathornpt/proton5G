package repository

import (
	"os"
	"path/filepath"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestWatchdogCheck(t *testing.T) {
	dir := t.TempDir()
	devDir := filepath.Join(dir, "1-1")
	if err := os.MkdirAll(filepath.Join(devDir, "power"), 0755); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(devDir, "idVendor"), []byte(domain.DefaultFM350.Vendor+"\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "idProduct"), []byte(domain.DefaultFM350.Product+"\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "power", "control"), []byte(string(domain.PowerAuto)+"\n"), 0644)

	wd := NewWatchdog(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
	status := wd.Check(filepath.Join(dir, "*"))

	if !status.Connected {
		t.Fatalf("expected connected=true")
	}
	if status.SysPath != devDir {
		t.Fatalf("expected SysPath=%s, got %s", devDir, status.SysPath)
	}
}

func TestWatchdogCheckAltPID7126(t *testing.T) {
	dir := t.TempDir()
	devDir := filepath.Join(dir, "2-1.4")
	if err := os.MkdirAll(filepath.Join(devDir, "power"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(devDir, "idVendor"), []byte("0e8d\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "idProduct"), []byte("7126\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "power", "control"), []byte("on\n"), 0644)

	// Default filter is 7127; live stick on this host is 7126.
	wd := NewWatchdog(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
	status := wd.Check(filepath.Join(dir, "*"))
	if !status.Connected || status.SysPath != devDir {
		t.Fatalf("expected 7126 match, got %+v", status)
	}
}

func TestWatchdogCheckMissing(t *testing.T) {
	dir := t.TempDir()
	wd := NewWatchdog(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
	status := wd.Check(filepath.Join(dir, "*"))

	if status.Connected {
		t.Fatalf("expected connected=false for empty sysfs")
	}
}
