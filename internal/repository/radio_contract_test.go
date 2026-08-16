package repository

import (
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestDeriveBandFromARFCNHandlesN77N78Overlap(t *testing.T) {
	tests := []struct {
		arfcn int
		want  string
	}{
		{620000, "n78"},
		{641760, "n78"},
		{653333, "n78"},
		{653334, "n77"},
		{660000, "n77"},
		{680000, "n77"},
	}
	for _, tt := range tests {
		if got := DeriveBandFromARFCN(tt.arfcn); got != tt.want {
			t.Fatalf("DeriveBandFromARFCN(%d) = %q, want %q", tt.arfcn, got, tt.want)
		}
	}
}

func TestDetectRadioTechUMTSFromCell(t *testing.T) {
	tech, _ := DetectRadioTech(0, false, domain.RegNotRegistered, domain.RegNotRegistered, []domain.CellInfo{{Serving: true, RAT: domain.TechUMTS}}, nil)
	if tech != domain.TechUMTS {
		t.Fatalf("tech = %q, want %q", tech, domain.TechUMTS)
	}
}

func TestDetectRadioTechLTERegistrationDoesNotFallThroughToUMTS(t *testing.T) {
	tech, _ := DetectRadioTech(0, false, domain.RegHome, domain.RegNotRegistered, nil, nil)
	if tech != domain.TechLTE {
		t.Fatalf("tech = %q, want %q", tech, domain.TechLTE)
	}
}
