package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

// SSEHub fans out one marshaled FullStatus snapshot to all SSE subscribers.
// A single Run goroutine ticks, marshals changed CachedStatus once, and
// non-blocking sends the payload so slow clients cannot stall others.
//
// Only the hub closes subscriber channels (after unsubscribe removes them).
// Start exactly once via Server.Run from main.
type SSEHub struct {
	status   func() domain.FullStatus
	interval time.Duration

	mu         sync.Mutex
	clients    map[chan []byte]struct{}
	last       []byte // last successful payload for instant snapshot on Subscribe
	lastStatus domain.FullStatus
	hasLast    bool
}

// NewSSEHub builds a hub. status is typically ModemUsecase.CachedStatus.
// interval <= 0 falls back to appdefaults.SSEInterval.
func NewSSEHub(status func() domain.FullStatus, interval time.Duration) *SSEHub {
	if interval <= 0 {
		interval = appdefaults.SSEInterval
	}
	return &SSEHub{
		status:   status,
		interval: interval,
		clients:  make(map[chan []byte]struct{}),
	}
}

// Run ticks until ctx is cancelled. Call once from main.
func (h *SSEHub) Run(ctx context.Context) {
	// Immediate sample so early subscribers are not empty until first tick.
	h.broadcast()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.broadcast()
		}
	}
}

// Subscribe registers a client channel (buffer 1). Returns the receive channel
// and an idempotent unsubscribe. If a last payload exists it is delivered
// immediately (non-blocking).
func (h *SSEHub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 1)

	h.mu.Lock()
	h.clients[ch] = struct{}{}
	if len(h.last) > 0 {
		// Non-blocking: buffer is empty on fresh subscribe.
		select {
		case ch <- h.last:
		default:
		}
	}
	h.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			h.mu.Lock()
			if _, ok := h.clients[ch]; ok {
				delete(h.clients, ch)
				close(ch)
			}
			h.mu.Unlock()
		})
	}
	return ch, unsub
}

func (h *SSEHub) broadcast() {
	if h.status == nil {
		return
	}
	status := h.status()

	// UpdatedAt changes every successful poll even when the modem state does not.
	// Ignore it for change detection so an idle modem does not force a full JSON
	// marshal, fan-out, JSON.parse, and DOM update every two seconds.
	semantic := status
	semantic.UpdatedAt = time.Time{}

	h.mu.Lock()
	if h.hasLast {
		previous := h.lastStatus
		previous.UpdatedAt = time.Time{}
		if reflect.DeepEqual(semantic, previous) {
			h.mu.Unlock()
			return
		}
	}
	h.mu.Unlock()

	payload, err := json.Marshal(status)
	if err != nil {
		return
	}

	h.mu.Lock()
	// Another broadcast cannot normally race Run, but keep the state update atomic
	// with subscriber snapshotting for tests and future callers.
	h.lastStatus = status
	h.hasLast = true
	h.last = payload
	clients := make([]chan []byte, 0, len(h.clients))
	for ch := range h.clients {
		clients = append(clients, ch)
	}
	h.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- payload:
		default:
			// Drop frame for slow client.
		}
	}
}
