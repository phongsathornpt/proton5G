package repository

import "testing"

func TestParseIWListAP(t *testing.T) {
	sample := `
Wiphy phy0
	Supported interface modes:
		 * IBSS
		 * managed
		 * AP
		 * AP/VLAN
		 * monitor
	Band 1:
`
	modes, ap := parseIWListAP(sample)
	if !ap {
		t.Fatal("expected AP support")
	}
	found := false
	for _, m := range modes {
		if m == "AP" {
			found = true
		}
	}
	if !found {
		t.Fatalf("modes=%v", modes)
	}
}

func TestParseIWListNoAP(t *testing.T) {
	sample := `
	Supported interface modes:
		 * managed
		 * monitor
`
	_, ap := parseIWListAP(sample)
	if ap {
		t.Fatal("expected no AP")
	}
}
