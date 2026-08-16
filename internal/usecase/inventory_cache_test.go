package usecase

import (
	"sync"
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

type countingInventory struct {
	mu    sync.Mutex
	calls int
}

func (f *countingInventory) ListModems(vendor, product, openATPort string) []domain.ModemDevice {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return []domain.ModemDevice{{
		ID:   "usb:test",
		Name: "FM350",
		ATPorts: []domain.ModemInterface{{
			Path: openATPort, Kind: domain.IfaceKindAT, ATReady: true,
		}},
	}}
}

func (f *countingInventory) ListMBIMDevices() []string { return nil }
func (f *countingInventory) MBIMCLIAvailable() bool    { return true }
func (f *countingInventory) MBIMInstallHint() string   { return "" }

func (f *countingInventory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestCachedInventoryPrimeServesWithoutRediscovery(t *testing.T) {
	base := &countingInventory{}
	now := time.Unix(1_700_000_000, 0)
	cache := NewCachedInventory(base, 15*time.Second)
	cache.nowFn = func() time.Time { return now }

	cache.Prime("2cb7", "01a2", "/dev/ttyUSB2")
	if base.callCount() != 1 {
		t.Fatalf("prime calls=%d want 1", base.callCount())
	}

	got := cache.ListModems("2cb7", "01a2", "/dev/ttyUSB2")
	if base.callCount() != 1 {
		t.Fatalf("fresh cached read rediscovered modems: calls=%d", base.callCount())
	}
	if len(got) != 1 || got[0].ID != "usb:test" || len(got[0].ATPorts) != 1 {
		t.Fatalf("unexpected cached inventory: %+v", got)
	}

	// Returned slices must be detached from the cached snapshot.
	got[0].ATPorts[0].Path = "/dev/mutated"
	again := cache.ListModems("2cb7", "01a2", "/dev/ttyUSB2")
	if again[0].ATPorts[0].Path != "/dev/ttyUSB2" {
		t.Fatalf("cached inventory mutated through caller: %+v", again)
	}
}
