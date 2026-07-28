package repository

import (
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

func TestRingOverwrite(t *testing.T) {
	r := NewRing(3)
	for i := 1; i <= 5; i++ {
		r.Add(domain.SignalSample{
			Timestamp:  time.Unix(int64(i), 0).UTC(),
			RSSI:       i,
			Percentage: i * 10,
		})
	}
	if r.Len() != 3 {
		t.Fatalf("expected len 3, got %d", r.Len())
	}
	snap := r.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(snap))
	}
	// Oldest kept after overwrite of 1,2 should be 3,4,5
	if snap[0].RSSI != 3 || snap[1].RSSI != 4 || snap[2].RSSI != 5 {
		t.Fatalf("unexpected order: %+v", snap)
	}
}

func TestRingEmpty(t *testing.T) {
	r := NewRing(10)
	if r.Snapshot() != nil {
		t.Fatalf("expected nil snapshot for empty ring")
	}
}
