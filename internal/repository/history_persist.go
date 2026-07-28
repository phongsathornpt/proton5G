package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"fm350-monitor/internal/pkg/domain"
)

// LoadFile replaces ring contents with samples from a JSON file (array of SignalSample).
// Missing file is a no-op. Truncates to ring capacity (keeps newest).
func (r *Ring) LoadFile(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var samples []domain.SignalSample
	if err := json.Unmarshal(data, &samples); err != nil {
		return fmt.Errorf("parse history file: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// reset
	r.head = 0
	r.size = 0
	for i := range r.buf {
		r.buf[i] = domain.SignalSample{}
	}
	// keep newest if file longer than capacity
	if len(samples) > r.cap {
		samples = samples[len(samples)-r.cap:]
	}
	for _, s := range samples {
		r.buf[r.head] = s
		r.head = (r.head + 1) % r.cap
		if r.size < r.cap {
			r.size++
		}
	}
	return nil
}

// SaveFile writes the current snapshot as pretty JSON. Creates parent dirs.
func (r *Ring) SaveFile(path string) error {
	if path == "" {
		return nil
	}
	snap := r.Snapshot()
	if snap == nil {
		snap = []domain.SignalSample{}
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
