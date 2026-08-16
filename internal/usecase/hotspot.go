package usecase

import (
	"fmt"
	"log"
	"strings"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

func defaultHotspotConfig() domain.HotspotConfig {
	return domain.HotspotConfig{
		SSID:      appdefaults.HotspotSSID,
		Password:  "", // must be set before start
		WlanIface: "",
		Channel:   appdefaults.HotspotChannel,
		Band:      appdefaults.HotspotBand,
		LANCIDR:   appdefaults.HotspotLANCIDR,
	}
}

func hotspotDefaultChannel(band string) int {
	if band == domain.HotspotBand5 {
		return 36
	}
	return appdefaults.HotspotChannel
}

// HotspotStatus returns tools, devices, config (password redacted), and runtime state.
func (s *ModemService) HotspotStatus() domain.HotspotStatus {
	st := domain.HotspotStatus{
		Note: "LTE/RNDIS is the internet uplink; WiFi radio broadcasts the hotspot (NAT).",
	}
	if s.hotspot == nil {
		st.State = domain.HotspotStateError
		st.Error = "hotspot repository not configured"
		st.Config = s.hotspotCfg.RedactedConfig()
		return st
	}

	s.hotspotMu.Lock()
	cfg := s.hotspotCfg
	state := s.hotspotState
	hsErr := s.hotspotErr
	s.hotspotMu.Unlock()

	running := s.hotspot.IsRunning()
	if running {
		state = domain.HotspotStateRunning
	} else if state == domain.HotspotStateRunning {
		state = domain.HotspotStateStopped
	}

	s.mu.RLock()
	uplink := cfg.UplinkIface
	if uplink == "" {
		uplink = s.selectedNet
	}
	s.mu.RUnlock()

	lanAddrs, uplinkAddrs, clients := s.hotspot.StatusExtras()
	// Prefer live uplink addrs from net repo if empty
	if len(uplinkAddrs) == 0 && uplink != "" && s.net != nil {
		uplinkAddrs = s.net.IfaceAddrs(uplink)
	}

	// Default empty wlan to first discovered wireless iface (e.g. wlp3s0).
	if cfg.WlanIface == "" {
		if devs := s.hotspot.ListWiFiDevices(); len(devs) > 0 {
			cfg.WlanIface = devs[0].Iface
		}
	}

	diag := s.hotspot.Diagnostics()
	st.State = state
	st.Config = cfg.RedactedConfig()
	st.Uplink = uplink
	st.UplinkAddrs = uplinkAddrs
	st.LANAddrs = lanAddrs
	st.Tools = diag.Tools
	st.InstallHint = diag.InstallHint
	st.Devices = diag.Interfaces
	if len(st.Devices) == 0 {
		st.Devices = s.hotspot.ListWiFiDevices()
	}
	st.Diagnostics = diag
	st.Clients = clients
	st.Error = hsErr
	if uplink == "" {
		st.Note += " Select/connect a RNDIS data interface before starting the hotspot."
	}
	if diag.InstallHint != "" {
		st.Note += " Install tools: " + diag.InstallHint + "."
	}
	return st
}

// LoadHotspotConfigFile loads persisted config if HotspotConfigFile is set.
// Missing file is fine. Call once from main after NewModemService.
func (s *ModemService) LoadHotspotConfigFile() error {
	if s.hotspotFile == "" {
		return nil
	}
	cfg, err := loadHotspotConfigFile(s.hotspotFile)
	if err != nil {
		return err
	}
	if cfg.SSID == "" && cfg.Password == "" && cfg.WlanIface == "" {
		return nil
	}
	// Merge onto defaults so empty band/channel get filled.
	merged := mergeHotspotConfig(defaultHotspotConfig(), cfg)
	if merged.LANCIDR == "" {
		merged.LANCIDR = appdefaults.HotspotLANCIDR
	}
	if merged.Band == "" {
		merged.Band = appdefaults.HotspotBand
	}
	if cfg.Channel <= 0 {
		merged.Channel = hotspotDefaultChannel(merged.Band)
	}
	s.hotspotMu.Lock()
	s.hotspotCfg = merged
	s.hotspotMu.Unlock()
	log.Printf("[INFO] Loaded hotspot config from %s (ssid=%q wlan=%s)", s.hotspotFile, merged.SSID, merged.WlanIface)
	return nil
}

func (s *ModemService) persistHotspotConfigLocked() {
	if s.hotspotFile == "" {
		return
	}
	if err := saveHotspotConfigFile(s.hotspotFile, s.hotspotCfg); err != nil {
		log.Printf("[WARN] Save hotspot config: %v", err)
	}
}

// HotspotSetConfig stores validated hotspot settings and persists when configured.
func (s *ModemService) HotspotSetConfig(cfg domain.HotspotConfig) error {
	s.hotspotMu.Lock()
	base := s.hotspotCfg
	s.hotspotMu.Unlock()

	patch := cfg
	cfg = mergeHotspotConfig(base, cfg)
	// Allow saving incomplete password only if not starting — still require SSID/iface when set.
	if strings.TrimSpace(cfg.SSID) == "" {
		return fmt.Errorf("ssid is required")
	}
	if cfg.WlanIface != "" {
		if err := domain.ValidateIfaceName(cfg.WlanIface); err != nil {
			return fmt.Errorf("wlan_iface: %w", err)
		}
	}
	if cfg.LANCIDR == "" {
		cfg.LANCIDR = appdefaults.HotspotLANCIDR
	}
	if cfg.Band == "" {
		cfg.Band = appdefaults.HotspotBand
	}
	if patch.Band != "" && patch.Channel == 0 {
		cfg.Channel = hotspotDefaultChannel(cfg.Band)
	} else if cfg.Channel <= 0 {
		cfg.Channel = hotspotDefaultChannel(cfg.Band)
	}
	// Full WPA2 validation only if password present
	if cfg.Password != "" && cfg.Password != "********" {
		if err := domain.ValidateHotspotConfig(cfg); err != nil {
			// iface may still be empty when only saving SSID/pass
			if cfg.WlanIface == "" {
				tmp := cfg
				tmp.WlanIface = "wlan0"
				if err2 := domain.ValidateHotspotConfig(tmp); err2 != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	s.hotspotMu.Lock()
	// Keep previous password if client sent redacted placeholder
	if cfg.Password == "********" || cfg.Password == "" {
		if s.hotspotCfg.Password != "" && cfg.Password == "********" {
			cfg.Password = s.hotspotCfg.Password
		}
	}
	s.hotspotCfg = cfg
	s.persistHotspotConfigLocked()
	s.hotspotMu.Unlock()
	return nil
}

func mergeHotspotConfig(base, patch domain.HotspotConfig) domain.HotspotConfig {
	out := base
	if patch.SSID != "" {
		out.SSID = patch.SSID
	}
	if patch.Password != "" {
		out.Password = patch.Password
	}
	if patch.WlanIface != "" {
		out.WlanIface = patch.WlanIface
	}
	if patch.UplinkIface != "" {
		out.UplinkIface = patch.UplinkIface
	}
	if patch.Channel != 0 {
		out.Channel = patch.Channel
	}
	if patch.Band != "" {
		out.Band = patch.Band
	}
	if patch.LANCIDR != "" {
		out.LANCIDR = patch.LANCIDR
	}
	if patch.Country != "" {
		out.Country = patch.Country
	}
	out.Enabled = patch.Enabled
	return out
}

// HotspotStart starts the software AP NATing to the LTE uplink.
func (s *ModemService) HotspotStart(req domain.HotspotStartRequest) (domain.HotspotStatus, error) {
	if s.hotspot == nil {
		return s.HotspotStatus(), fmt.Errorf("hotspot repository not configured")
	}
	s.hotspotMu.Lock()
	cfg := s.hotspotCfg
	s.hotspotMu.Unlock()

	// Apply request overrides
	if req.SSID != "" {
		cfg.SSID = req.SSID
	}
	if req.Password != "" {
		cfg.Password = req.Password
	}
	if req.WlanIface != "" {
		cfg.WlanIface = req.WlanIface
	}
	if req.UplinkIface != "" {
		cfg.UplinkIface = req.UplinkIface
	}
	if req.Channel != 0 {
		cfg.Channel = req.Channel
	}
	if req.Band != "" {
		cfg.Band = req.Band
	}
	if req.LANCIDR != "" {
		cfg.LANCIDR = req.LANCIDR
	}
	if req.Country != "" {
		cfg.Country = req.Country
	}
	if cfg.LANCIDR == "" {
		cfg.LANCIDR = appdefaults.HotspotLANCIDR
	}
	if cfg.Band == "" {
		cfg.Band = appdefaults.HotspotBand
	}
	if req.Band != "" && req.Channel == 0 {
		cfg.Channel = hotspotDefaultChannel(cfg.Band)
	} else if cfg.Channel <= 0 {
		cfg.Channel = hotspotDefaultChannel(cfg.Band)
	}
	if cfg.SSID == "" {
		cfg.SSID = appdefaults.HotspotSSID
	}
	if cfg.WlanIface == "" {
		if devs := s.hotspot.ListWiFiDevices(); len(devs) > 0 {
			cfg.WlanIface = devs[0].Iface
		}
	}

	if err := domain.ValidateHotspotConfig(cfg); err != nil {
		return s.HotspotStatus(), err
	}

	s.mu.RLock()
	uplink := cfg.UplinkIface
	if uplink == "" {
		uplink = s.selectedNet
	}
	s.mu.RUnlock()
	if uplink == "" {
		return s.HotspotStatus(), fmt.Errorf("no uplink interface — select RNDIS data iface and connect first")
	}

	s.hotspotMu.Lock()
	s.hotspotState = domain.HotspotStateStarting
	s.hotspotErr = ""
	s.hotspotCfg = cfg
	s.hotspotMu.Unlock()

	_, err := s.hotspot.Start(cfg, uplink)

	s.hotspotMu.Lock()
	if err != nil {
		s.hotspotState = domain.HotspotStateError
		s.hotspotErr = err.Error()
	} else {
		s.hotspotState = domain.HotspotStateRunning
		s.hotspotErr = ""
		s.hotspotCfg.Enabled = true
		s.persistHotspotConfigLocked()
	}
	s.hotspotMu.Unlock()

	st := s.HotspotStatus()
	return st, err
}

// HotspotStop tears down the software AP and NAT rules.
func (s *ModemService) HotspotStop() (domain.HotspotStatus, error) {
	if s.hotspot == nil {
		return s.HotspotStatus(), fmt.Errorf("hotspot repository not configured")
	}
	s.hotspotMu.Lock()
	s.hotspotState = domain.HotspotStateStopping
	s.hotspotMu.Unlock()

	_, err := s.hotspot.Stop()

	s.hotspotMu.Lock()
	if err != nil {
		s.hotspotState = domain.HotspotStateError
		s.hotspotErr = err.Error()
	} else {
		s.hotspotState = domain.HotspotStateStopped
		s.hotspotErr = ""
		s.hotspotCfg.Enabled = false
		s.persistHotspotConfigLocked()
	}
	s.hotspotMu.Unlock()
	return s.HotspotStatus(), err
}

// ListWiFi returns host wireless interfaces.
func (s *ModemService) ListWiFi() []domain.WiFiDevice {
	if s.hotspot == nil {
		return nil
	}
	return s.hotspot.ListWiFiDevices()
}
