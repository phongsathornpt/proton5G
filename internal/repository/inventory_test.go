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

func TestListUSBDevicesAcceptsLiveFM350(t *testing.T) {
	paths := ListUSBDevices(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
	if len(paths) == 0 {
		t.Skip("no FM350-GL in sysfs (0e8d:7126/7127)")
	}
	for _, p := range paths {
		v, prod := readUSBID(p)
		if !domain.IsFM350(v, prod) {
			t.Fatalf("%s is not an FM350 id (%s:%s)", p, v, prod)
		}
		t.Logf("live FM350 %s:%s at %s", v, prod, p)
	}
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

func TestTTYAndNetWalkFollowsSysfsSymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real", "2-1.4")
	if err := os.MkdirAll(filepath.Join(real, "2-1.4:1.2", "ttyUSB0"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(real, "2-1.4:1.0", "net", "enx000011121314"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "bus", "2-1.4")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	ttys := ttyNodesUnder(link)
	if len(ttys) != 1 || ttys[0] != "/dev/ttyUSB0" {
		t.Fatalf("ttys via symlink=%v", ttys)
	}
	nets := netIfacesUnder(link)
	if len(nets) != 1 || nets[0] != "enx000011121314" {
		t.Fatalf("nets via symlink=%v", nets)
	}
}

func TestProbeATPortsCachedSkipAndEmpty(t *testing.T) {
	m := ProbeATPortsCached(nil, "")
	if len(m) != 0 {
		t.Fatalf("empty: %v", m)
	}
	// Active port is never opened — always reported ready.
	m = ProbeATPortsCached([]string{"/dev/ttyUSB_does_not_exist_xyz", "/dev/ttyUSB_active"}, "/dev/ttyUSB_active")
	if !m["/dev/ttyUSB_active"] {
		t.Fatal("skip path should be ready")
	}
	// Nonexistent path should fail probe (and be cached).
	if m["/dev/ttyUSB_does_not_exist_xyz"] {
		t.Fatal("missing device should not be AT-ready")
	}
	// Second call should hit cache (still false).
	m2 := ProbeATPortsCached([]string{"/dev/ttyUSB_does_not_exist_xyz"}, "")
	if m2["/dev/ttyUSB_does_not_exist_xyz"] {
		t.Fatal("cached miss expected")
	}
}

func TestProbeATPortsCachedParallelDistinct(t *testing.T) {
	// Several missing paths probed concurrently — all false, no hang/panic.
	paths := []string{
		"/dev/ttyUSB_probe_a_zzz",
		"/dev/ttyUSB_probe_b_zzz",
		"/dev/ttyUSB_probe_c_zzz",
		"/dev/ttyUSB_probe_d_zzz",
	}
	m := ProbeATPortsCached(paths, "")
	for _, p := range paths {
		if m[p] {
			t.Fatalf("%s unexpectedly ready", p)
		}
	}
}
