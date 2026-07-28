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

func TestWatchdogCheckMissing(t *testing.T) {
	dir := t.TempDir()
	wd := NewWatchdog(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
	status := wd.Check(filepath.Join(dir, "*"))

	if status.Connected {
		t.Fatalf("expected connected=false for empty sysfs")
	}
}
