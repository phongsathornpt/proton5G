package repository

import (
	"os"
	"path/filepath"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func makeFakeFM350Net(t *testing.T, driver string) (classRoot, iface string) {
	t.Helper()
	root := t.TempDir()
	usb := filepath.Join(root, "sys", "devices", "pci0000:00", "usb2", "2-1")
	ifaceDevice := filepath.Join(usb, "2-1:1.0")
	if err := os.MkdirAll(ifaceDevice, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "idVendor"), []byte(domain.FM350Vendor+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "idProduct"), []byte(domain.FM350Product7127+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if driver != "" {
		driverDir := filepath.Join(root, "sys", "bus", "usb", "drivers", driver)
		if err := os.MkdirAll(driverDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(driverDir, filepath.Join(ifaceDevice, "driver")); err != nil {
			t.Fatal(err)
		}
	}

	classRoot = filepath.Join(root, "sys", "class", "net")
	iface = "enxfm350"
	if err := os.MkdirAll(filepath.Join(classRoot, iface), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ifaceDevice, filepath.Join(classRoot, iface, "device")); err != nil {
		t.Fatal(err)
	}
	return classRoot, iface
}

func TestValidateFM350NetIfaceAt(t *testing.T) {
	classRoot, iface := makeFakeFM350Net(t, "rndis_host")
	if err := validateFM350NetIfaceAt(classRoot, iface); err != nil {
		t.Fatalf("expected FM350 iface to validate: %v", err)
	}
	if err := validateFM350RNDISIfaceAt(classRoot, iface); err != nil {
		t.Fatalf("expected RNDIS-backed FM350 iface to validate: %v", err)
	}
}

func TestValidateFM350RNDISIfaceAtRejectsMBIMDriver(t *testing.T) {
	classRoot, iface := makeFakeFM350Net(t, "cdc_mbim")
	if err := validateFM350RNDISIfaceAt(classRoot, iface); err == nil {
		t.Fatal("expected cdc_mbim interface to be rejected by RNDIS path")
	}
}

func TestValidateFM350NetIfaceAtRejectsForeignUSB(t *testing.T) {
	root := t.TempDir()
	usb := filepath.Join(root, "usb", "1-1")
	ifaceDevice := filepath.Join(usb, "1-1:1.0")
	if err := os.MkdirAll(ifaceDevice, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "idVendor"), []byte("1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "idProduct"), []byte("5678\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	classRoot := filepath.Join(root, "class", "net")
	if err := os.MkdirAll(filepath.Join(classRoot, "eth-test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ifaceDevice, filepath.Join(classRoot, "eth-test", "device")); err != nil {
		t.Fatal(err)
	}
	if err := validateFM350NetIfaceAt(classRoot, "eth-test"); err == nil {
		t.Fatal("expected foreign USB interface to be rejected")
	}
}
