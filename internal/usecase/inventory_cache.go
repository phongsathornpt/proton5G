package usecase

import (
	"sync"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

const defaultInventoryCacheTTL = 15 * time.Second

// CachedInventory keeps sysfs walks and serial-port probes out of the service
// state mutex. Prime it once during startup; later stale reads return the previous
// snapshot immediately and refresh in the background.
type CachedInventory struct {
	base DeviceInventory
	ttl  time.Duration

	mu         sync.Mutex
	initialized bool
	refreshing  bool
	vendor      string
	product     string
	openAT      string
	modems      []domain.ModemDevice
	expiresAt   time.Time
	mbimCLI     bool
	installHint string

	nowFn func() time.Time
}

func NewCachedInventory(base DeviceInventory, ttl time.Duration) *CachedInventory {
	if ttl <= 0 {
		ttl = defaultInventoryCacheTTL
	}
	return &CachedInventory{base: base, ttl: ttl}
}

func (c *CachedInventory) now() time.Time {
	if c.nowFn != nil {
		return c.nowFn()
	}
	return time.Now()
}

// Prime performs the first expensive discovery synchronously. Call this before
// constructing ModemService so no service lock is held while probing USB/serial.
func (c *CachedInventory) Prime(vendor, product, openATPort string) {
	c.refresh(vendor, product, openATPort)
}

func (c *CachedInventory) ListModems(vendor, product, openATPort string) []domain.ModemDevice {
	if c == nil || c.base == nil {
		return nil
	}
	now := c.now()

	c.mu.Lock()
	initialized := c.initialized
	stale := !initialized || now.After(c.expiresAt) || vendor != c.vendor || product != c.product || openATPort != c.openAT
	if stale && initialized && !c.refreshing {
		c.refreshing = true
		go c.refreshAsync(vendor, product, openATPort)
	}
	cached := cloneModems(c.modems)
	c.mu.Unlock()

	if initialized {
		return cached
	}

	// Safe fallback for callers that forgot to Prime. This can block, but startup
	// wiring primes the cache before ModemService is exposed.
	c.refresh(vendor, product, openATPort)
	c.mu.Lock()
	cached = cloneModems(c.modems)
	c.mu.Unlock()
	return cached
}

func (c *CachedInventory) refreshAsync(vendor, product, openATPort string) {
	c.refresh(vendor, product, openATPort)
}

func (c *CachedInventory) refresh(vendor, product, openATPort string) {
	if c == nil || c.base == nil {
		return
	}
	modems := c.base.ListModems(vendor, product, openATPort)
	mbimCLI := c.base.MBIMCLIAvailable()
	installHint := ""
	if !mbimCLI {
		installHint = c.base.MBIMInstallHint()
	}

	c.mu.Lock()
	c.modems = cloneModems(modems)
	c.vendor = vendor
	c.product = product
	c.openAT = openATPort
	c.mbimCLI = mbimCLI
	c.installHint = installHint
	c.expiresAt = c.now().Add(c.ttl)
	c.initialized = true
	c.refreshing = false
	c.mu.Unlock()
}

func (c *CachedInventory) ListMBIMDevices() []string {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.ListMBIMDevices()
}

func (c *CachedInventory) MBIMCLIAvailable() bool {
	if c == nil || c.base == nil {
		return false
	}
	c.mu.Lock()
	if c.initialized {
		v := c.mbimCLI
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()
	return c.base.MBIMCLIAvailable()
}

func (c *CachedInventory) MBIMInstallHint() string {
	if c == nil || c.base == nil {
		return ""
	}
	c.mu.Lock()
	if c.initialized {
		v := c.installHint
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()
	return c.base.MBIMInstallHint()
}

func cloneModems(in []domain.ModemDevice) []domain.ModemDevice {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ModemDevice, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ATPorts = append([]domain.ModemInterface(nil), in[i].ATPorts...)
		out[i].MBIMNodes = append([]domain.ModemInterface(nil), in[i].MBIMNodes...)
		out[i].NetIfaces = append([]domain.ModemInterface(nil), in[i].NetIfaces...)
	}
	return out
}
