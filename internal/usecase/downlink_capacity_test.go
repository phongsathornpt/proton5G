package usecase

import (
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestDeriveDownlinkCapacityThailandNSA(t *testing.T) {
	ca := []domain.CAComponent{
		{Component: "PCC", Band: "B40", DLBW: "20 MHz", DLMIMO: "4x4", DLMod: "256QAM"},
		{Component: "SCC1", Band: "B3", DLBW: "20 MHz", DLMIMO: "4x4", DLMod: "256QAM"},
		{Component: "SCC2", Band: "n41", DLBW: "90 MHz", DLMIMO: "4x4", DLMod: "256QAM"},
	}

	got := DeriveDownlinkCapacity(ca)
	if got.ActiveCC != 3 || got.LTECC != 2 || got.NRCC != 1 {
		t.Fatalf("cc summary=%+v", got)
	}
	if got.TotalBandwidthMHz != 130 || got.LTEBandwidthMHz != 40 || got.NRBandwidthMHz != 90 {
		t.Fatalf("bandwidth summary=%+v", got)
	}
	if got.EstimatedPeakMbps != 2912 {
		t.Fatalf("estimated peak=%d want 2912", got.EstimatedPeakMbps)
	}
	if !got.EstimateComplete || got.EstimatedFromCC != 3 {
		t.Fatalf("expected complete estimate: %+v", got)
	}
	if got.BestDLMIMO != "4x4" || got.BestDLModulation != "256QAM" {
		t.Fatalf("best radio fields=%+v", got)
	}
	if got.DeviceCeilingMbps != FM350GLMaxDLMbps {
		t.Fatalf("device ceiling=%d", got.DeviceCeilingMbps)
	}
}

func TestDeriveDownlinkCapacityPartialTelemetry(t *testing.T) {
	ca := []domain.CAComponent{
		{Component: "PCC", Band: "n41", DLBW: "100 MHz", DLMIMO: "4x4", DLMod: "256QAM"},
		{Component: "SCC1", Band: "B3", DLBW: "20 MHz"},
	}

	got := DeriveDownlinkCapacity(ca)
	if got.ActiveCC != 2 || got.EstimatedFromCC != 1 || got.EstimateComplete {
		t.Fatalf("expected partial estimate: %+v", got)
	}
	if got.TotalBandwidthMHz != 120 {
		t.Fatalf("total bw=%v", got.TotalBandwidthMHz)
	}
	if got.EstimatedPeakMbps != 2240 {
		t.Fatalf("estimated peak=%d want 2240", got.EstimatedPeakMbps)
	}
}

func TestDeriveDownlinkCapacityCapsAtDeviceCeiling(t *testing.T) {
	ca := []domain.CAComponent{
		{Component: "PCC", Band: "n41", DLBW: "100 MHz", DLMIMO: "8x8", DLMod: "1024QAM"},
		{Component: "SCC1", Band: "n78", DLBW: "100 MHz", DLMIMO: "8x8", DLMod: "1024QAM"},
	}

	got := DeriveDownlinkCapacity(ca)
	if got.EstimatedPeakMbps != FM350GLMaxDLMbps {
		t.Fatalf("estimated peak=%d want cap %d", got.EstimatedPeakMbps, FM350GLMaxDLMbps)
	}
	if got.Limiter != "FM350-GL device ceiling" {
		t.Fatalf("limiter=%q", got.Limiter)
	}
}

func TestCapacityParsingHelpers(t *testing.T) {
	if got := parseBandwidthMHz("1.4 MHz"); got != 1.4 {
		t.Fatalf("bandwidth=%v", got)
	}
	if got := parseMIMOLayers("4x4"); got != 4 {
		t.Fatalf("mimo=%d", got)
	}
	if got := modulationBits("256QAM"); got != 8 {
		t.Fatalf("mod bits=%d", got)
	}
}
