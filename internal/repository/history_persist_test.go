package repository

import (
	"path/filepath"
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

func TestSaveLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hist.json")

	r := NewRing(5)
	r.Add(domain.SignalSample{Timestamp: time.Unix(1, 0).UTC(), RSSI: -70, Percentage: 50})
	r.Add(domain.SignalSample{Timestamp: time.Unix(2, 0).UTC(), RSSI: -65, Percentage: 60, RSRP: -95})

	if err := r.SaveFile(path); err != nil {
		t.Fatal(err)
	}

	r2 := NewRing(5)
	if err := r2.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if r2.Len() != 2 {
		t.Fatalf("expected 2, got %d", r2.Len())
	}
	snap := r2.Snapshot()
	if snap[1].RSRP != -95 || snap[1].Percentage != 60 {
		t.Fatalf("unexpected sample: %+v", snap[1])
	}
}

func TestLoadMissingFile(t *testing.T) {
	r := NewRing(3)
	if err := r.LoadFile(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 0 {
		t.Fatal("expected empty")
	}
}
