package handler

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

func fixedStatus(pct int) func() domain.FullStatus {
	return func() domain.FullStatus {
		return domain.FullStatus{
			Signal: domain.SignalInfo{Percentage: pct},
			RATMode: domain.RATModeAuto,
		}
	}
}

func waitPayload(t *testing.T, ch <-chan []byte, timeout time.Duration) []byte {
	t.Helper()
	select {
	case p, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		return p
	case <-time.After(timeout):
		t.Fatal("timeout waiting for payload")
		return nil
	}
}

func TestSSEHubBroadcastsToSubscriber(t *testing.T) {
	hub := NewSSEHub(fixedStatus(70), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	ch, unsub := hub.Subscribe()
	defer unsub()

	payload := waitPayload(t, ch, 500*time.Millisecond)
	var st domain.FullStatus
	if err := json.Unmarshal(payload, &st); err != nil {
		t.Fatal(err)
	}
	if st.Signal.Percentage != 70 {
		t.Fatalf("got %+v", st)
	}
}

func TestSSEHubTwoSubscribersSamePayload(t *testing.T) {
	hub := NewSSEHub(fixedStatus(42), 15*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	ch1, unsub1 := hub.Subscribe()
	defer unsub1()
	ch2, unsub2 := hub.Subscribe()
	defer unsub2()

	p1 := waitPayload(t, ch1, 500*time.Millisecond)
	p2 := waitPayload(t, ch2, 500*time.Millisecond)
	if string(p1) != string(p2) {
		t.Fatalf("payloads differ:\n%s\n%s", p1, p2)
	}
}

func TestSSEHubUnsubscribeStopsDelivery(t *testing.T) {
	// Changing status so skip-if-unchanged does not silence the hub.
	var mu sync.Mutex
	n := 0
	status := func() domain.FullStatus {
		mu.Lock()
		defer mu.Unlock()
		n++
		return domain.FullStatus{Signal: domain.SignalInfo{Percentage: n}}
	}
	hub := NewSSEHub(status, 15*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	ch, unsub := hub.Subscribe()
	_ = waitPayload(t, ch, 500*time.Millisecond)
	unsub()

	// After unsub, further receives must not succeed (chan closed or no traffic).
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("received payload after unsubscribe")
		}
		// closed — expected
	case <-time.After(80 * time.Millisecond):
		// no delivery — also fine if close raced; prefer closed
	}

	// Second unsub must not panic.
	unsub()
}

func TestSSEHubSlowClientDoesNotBlockOthers(t *testing.T) {
	var mu sync.Mutex
	n := 0
	status := func() domain.FullStatus {
		mu.Lock()
		defer mu.Unlock()
		n++
		return domain.FullStatus{Signal: domain.SignalInfo{Percentage: n}}
	}
	hub := NewSSEHub(status, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Slow client: never drain after first fill.
	slow, unsubSlow := hub.Subscribe()
	defer unsubSlow()
	fast, unsubFast := hub.Subscribe()
	defer unsubFast()

	// Drain once so both buffers can fill; then only drain fast.
	_ = waitPayload(t, slow, 500*time.Millisecond)
	_ = waitPayload(t, fast, 500*time.Millisecond)

	// Collect several fast payloads while slow is blocked (buffer full).
	got := 0
	deadline := time.After(400 * time.Millisecond)
	for got < 3 {
		select {
		case <-fast:
			got++
		case <-deadline:
			t.Fatalf("fast client only got %d payloads (slow client blocked hub?)", got)
		}
	}
}

func TestSSEHubRunRespectsCancel(t *testing.T) {
	hub := NewSSEHub(fixedStatus(1), 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestSSEHubLateSubscriberGetsLast(t *testing.T) {
	hub := NewSSEHub(fixedStatus(99), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Wait until hub has produced at least one broadcast.
	time.Sleep(50 * time.Millisecond)

	ch, unsub := hub.Subscribe()
	defer unsub()

	payload := waitPayload(t, ch, 200*time.Millisecond)
	var st domain.FullStatus
	if err := json.Unmarshal(payload, &st); err != nil {
		t.Fatal(err)
	}
	if st.Signal.Percentage != 99 {
		t.Fatalf("late sub expected last snapshot, got %+v", st)
	}
}

func TestSSEHubSkipUnchangedDoesNotRetransmit(t *testing.T) {
	hub := NewSSEHub(fixedStatus(5), 15*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	ch, unsub := hub.Subscribe()
	defer unsub()

	_ = waitPayload(t, ch, 500*time.Millisecond)

	// Unchanged status should not keep flooding the buffer.
	extra := 0
	timeout := time.After(80 * time.Millisecond)
	for {
		select {
		case <-ch:
			extra++
		case <-timeout:
			if extra > 0 {
				t.Fatalf("expected no retransmit of unchanged status, got %d extra", extra)
			}
			return
		}
	}
}
