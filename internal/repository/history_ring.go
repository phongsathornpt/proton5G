package repository

import (
	"sync"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

// Ring is a fixed-capacity in-memory ring buffer of signal samples.
type Ring struct {
	mu   sync.RWMutex
	buf  []domain.SignalSample
	cap  int
	head int // next write index
	size int
}

func NewRing(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring{
		buf: make([]domain.SignalSample, capacity),
		cap: capacity,
	}
}

func (r *Ring) Cap() int {
	return r.cap
}

func (r *Ring) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// Add appends a sample, overwriting the oldest when full.
func (r *Ring) Add(s domain.SignalSample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Timestamp.IsZero() {
		s.Timestamp = time.Now().UTC()
	}
	r.buf[r.head] = s
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

// Snapshot returns samples from oldest to newest.
func (r *Ring) Snapshot() []domain.SignalSample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.size == 0 {
		return nil
	}
	out := make([]domain.SignalSample, r.size)
	start := 0
	if r.size == r.cap {
		start = r.head
	}
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%r.cap]
	}
	return out
}
