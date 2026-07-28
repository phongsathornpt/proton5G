package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDataMode(t *testing.T) {
	cases := []struct {
		mbim, net int
		want      string
	}{
		{0, 1, "rndis"},
		{1, 0, "mbim"},
		{0, 0, "at_only"},
		{1, 1, "mixed"},
		{2, 3, "mixed"},
	}
	for _, c := range cases {
		if got := detectDataMode(c.mbim, c.net); got != c.want {
			t.Fatalf("detectDataMode(%d,%d)=%q want %q", c.mbim, c.net, got, c.want)
		}
	}
}

func TestNetLinkEmptyIface(t *testing.T) {
	if _, err := NetLinkUp(""); err == nil {
		t.Fatal("NetLinkUp empty should error")
	}
	if _, err := NetLinkDown(""); err == nil {
		t.Fatal("NetLinkDown empty should error")
	}
	if _, err := NetDHCP(""); err == nil {
		t.Fatal("NetDHCP empty should error")
	}
	if _, err := ConnectRNDIS(""); err == nil {
		t.Fatal("ConnectRNDIS empty should error")
	}
	if _, err := DisconnectRNDIS(""); err == nil {
		t.Fatal("DisconnectRNDIS empty should error")
	}
}

func TestNetIfacesUnder(t *testing.T) {
	root := t.TempDir()
	// Fake USB tree: …/net/enxdeadbeef
	netDir := filepath.Join(root, "1-1", "1-1:1.0", "net", "enxdeadbeef")
	if err := os.MkdirAll(netDir, 0755); err != nil {
		t.Fatal(err)
	}
	got := netIfacesUnder(filepath.Join(root, "1-1"))
	if len(got) != 1 || got[0] != "enxdeadbeef" {
		t.Fatalf("got %v", got)
	}
}
