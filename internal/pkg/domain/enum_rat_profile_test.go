package domain

import "testing"

func TestParseThailandRATProfiles(t *testing.T) {
	tests := map[string]RATModePref{
		"th-nsa":           RATPrefTHNSA,
		"thai-nsa":         RATPrefTHNSA,
		"th-nsa-b40-n41":   RATPrefTHNSAB40N41,
		"th-lte":           RATPrefTHLTE,
		"th-lte-b40-b41":   RATPrefTHLTEB40B41,
		"thailand-lte":     RATPrefTHLTE,
		"thai-lte-b40-b41": RATPrefTHLTEB40B41,
	}
	for input, want := range tests {
		got, err := ParseRATModePref(input)
		if err != nil {
			t.Fatalf("ParseRATModePref(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseRATModePref(%q)=%q want %q", input, got, want)
		}
	}
}

func TestThailandRATProfileDisplayAndFallback(t *testing.T) {
	if RATPrefTHNSA.GTACTCode() != GTACTENDC || RATPrefTHNSA.ToDisplay() != RATModeENDC {
		t.Fatalf("Thailand NSA profile must fall back to EN-DC")
	}
	if RATPrefTHLTE.GTACTCode() != GTACTLTEOnly || RATPrefTHLTE.ToDisplay() != RATModeLTEOnly {
		t.Fatalf("Thailand LTE profile must fall back to LTE-only")
	}
}
