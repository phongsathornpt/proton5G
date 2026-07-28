package repository

import "fm350-monitor/internal/pkg/domain"

// HotspotRepo adapts HotspotManager to the usecase HotspotRepository port.
type HotspotRepo struct {
	m *HotspotManager
}

// NewHotspotRepo constructs a hotspot adapter using the given runtime dir.
func NewHotspotRepo(runtimeDir string) *HotspotRepo {
	return &HotspotRepo{m: NewHotspotManager(runtimeDir)}
}

func (r *HotspotRepo) ListWiFiDevices() []domain.WiFiDevice {
	return ListWiFiDevices()
}

func (r *HotspotRepo) Tools() domain.HotspotTools {
	return HotspotToolsPresent()
}

func (r *HotspotRepo) Start(cfg domain.HotspotConfig, uplink string) (string, error) {
	return r.m.Start(cfg, uplink)
}

func (r *HotspotRepo) Stop() (string, error) {
	return r.m.Stop()
}

func (r *HotspotRepo) IsRunning() bool {
	return r.m.IsRunning()
}

func (r *HotspotRepo) StatusExtras() (lanAddrs, uplinkAddrs []string, clients []domain.HotspotClient) {
	return r.m.StatusExtras()
}
