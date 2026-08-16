package handler

import (
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

func TestSSEHubIgnoresUpdatedAtOnlyChanges(t *testing.T) {
	status := domain.FullStatus{
		Modem:     domain.ModemStatus{Connected: true},
		Signal:    domain.SignalInfo{RSSI: -70, Percentage: 70},
		UpdatedAt: time.Unix(100, 0),
	}
	h := NewSSEHub(func() domain.FullStatus { return status }, time.Second)
	ch, unsub := h.Subscribe()
	defer unsub()

	h.broadcast()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("missing initial SSE frame")
	}

	status.UpdatedAt = time.Unix(102, 0)
	h.broadcast()
	select {
	case <-ch:
		t.Fatal("timestamp-only change should not broadcast")
	default:
	}

	status.Signal.RSSI = -69
	h.broadcast()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("semantic status change should broadcast")
	}
}
