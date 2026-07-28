package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUSBDeviceNode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "busnum"), []byte("2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devnum"), []byte("5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	node, err := USBDeviceNode(dir)
	if err != nil {
		t.Fatal(err)
	}
	if node != "/dev/bus/usb/002/005" {
		t.Fatalf("got %s", node)
	}
}
