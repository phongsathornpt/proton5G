package repository

import (
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestFormatMIMO(t *testing.T) {
	tests := map[string]string{
		"1":   "1x1",
		"2":   "2x2",
		"4":   "4x4",
		"4x4": "4x4",
		"255": "",
		"0":   "",
		"-1":  "",
	}
	for in, want := range tests {
		if got := FormatMIMO(in); got != want {
			t.Fatalf("FormatMIMO(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEnrichGTCAINFOComponentsLongSCC(t *testing.T) {
	raw := `+GTCAINFO:
SCC 3:2,0,103,216,1444,50,255,4,255,3,255,13,-9,-81
OK`
	ca := []domain.CAComponent{{
		Component: "SCC3",
		Band:      "B3",
		DLMod:     "256QAM", // legacy parser incorrectly reads the DL-MIMO field.
	}}

	got := EnrichGTCAINFOComponents(ca, raw)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].DLMIMO != "4x4" {
		t.Fatalf("dl mimo=%q", got[0].DLMIMO)
	}
	if got[0].ULMIMO != "" {
		t.Fatalf("ul mimo=%q", got[0].ULMIMO)
	}
	if got[0].DLMod != "64QAM" {
		t.Fatalf("dl modulation=%q", got[0].DLMod)
	}
	if got[0].ULMod != "" {
		t.Fatalf("ul modulation=%q", got[0].ULMod)
	}
	if got[0].DLBW != "10 MHz" {
		t.Fatalf("dl bw=%q", got[0].DLBW)
	}
}

func TestThailandGTACTProfiles(t *testing.T) {
	tests := map[domain.RATModePref]string{
		domain.RATPrefTHNSA:       CmdGTACTSetTHNSA,
		domain.RATPrefTHNSAB40N41: CmdGTACTSetTHNSAB40N41,
		domain.RATPrefTHLTE:       CmdGTACTSetTHLTE,
		domain.RATPrefTHLTEB40B41: CmdGTACTSetTHLTEB40B41,
	}
	for pref, want := range tests {
		if got := CmdGTACTSetByPref(pref); got != want {
			t.Fatalf("CmdGTACTSetByPref(%q)=%q want %q", pref, got, want)
		}
	}
}
