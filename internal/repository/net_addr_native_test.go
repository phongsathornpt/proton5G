package repository

import "testing"

func TestNetIfaceAddrsNativeLoopback(t *testing.T) {
	got := NetIfaceAddrsNative("lo")
	if len(got) == 0 {
		t.Skip("loopback interface unavailable in test environment")
	}
	for _, ip := range got {
		if ip == "127.0.0.1" {
			return
		}
	}
	t.Fatalf("loopback IPv4 missing from %v", got)
}

func TestNetIfaceAddrsNativeRejectsInvalidName(t *testing.T) {
	if got := NetIfaceAddrsNative("bad iface;rm -rf /"); got != nil {
		t.Fatalf("invalid interface returned addresses: %v", got)
	}
}
