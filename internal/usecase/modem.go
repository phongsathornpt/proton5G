package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

// ModemService is the application layer for modem monitor/control workflows.
type ModemService struct {
	usb       USBRepository
	at        ATRepository
	history   HistoryRepository
	mbim      MBIMRepository
	net       NetRepository
	discover  ATDiscoverer
	inventory DeviceInventory
	vendor    string
	product   string

	mu              sync.RWMutex
	atMu            sync.Mutex // AT work queue: poll, control, rediscover, port lifecycle
	status          domain.FullStatus
	selectedModemID string
	selectedMBIM    string
	selectedNet     string
	atFailStreak    int
	lastResetAt     time.Time
	lastRediscover  time.Time
	lastPermLogAt   time.Time
	resetCooldown   time.Duration
	rediscoverEvery time.Duration
	failStreakMax   int
}

type ModemServiceConfig struct {
	USB       USBRepository
	AT        ATRepository
	History   HistoryRepository
	MBIM      MBIMRepository
	Net       NetRepository
	Discover  ATDiscoverer
	Inventory DeviceInventory
	Vendor    string
	Product   string
}

func NewModemService(cfg ModemServiceConfig) *ModemService {
	if cfg.Vendor == "" {
		cfg.Vendor = domain.DefaultFM350.Vendor
	}
	if cfg.Product == "" {
		cfg.Product = domain.DefaultFM350.Product
	}
	return &ModemService{
		usb:             cfg.USB,
		at:              cfg.AT,
		history:         cfg.History,
		mbim:            cfg.MBIM,
		net:             cfg.Net,
		discover:        cfg.Discover,
		inventory:       cfg.Inventory,
		vendor:          cfg.Vendor,
		product:         cfg.Product,
		resetCooldown:     appdefaults.ATResetCooldown,
		rediscoverEvery: appdefaults.ATRediscoverEvery,
		failStreakMax:   appdefaults.ATFailResetStreak,
	}
}

// Status forces a USB+AT poll (recovery policy, history sample). Used by tests and ?fresh=1.
// Prefer CachedStatus for hot paths (SSE) so many clients do not thrash the serial port.
func (s *ModemService) Status() domain.FullStatus {
	return s.pollStatus()
}

// CachedStatus returns the last polled snapshot without performing AT I/O.
func (s *ModemService) CachedStatus() domain.FullStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// withAT runs fn as the sole owner of the manager AT port (FIFO mutex "work queue").
// Covers control, poll, rediscover, and port lifecycle. Do not call withAT while
// holding s.mu (lock order: atMu → short s.mu only). fn must not re-enter withAT.
func (s *ModemService) withAT(fn func() error) error {
	s.atMu.Lock()
	defer s.atMu.Unlock()
	return fn()
}

// RunStatusPoller periodically refreshes the status cache until ctx is cancelled.
// Status samples and recovery share atMu with control commands; start once from main.
func (s *ModemService) RunStatusPoller(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = appdefaults.StatusPollInterval
	}
	// Immediate sample so UI is not empty until first tick.
	s.pollStatus()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollStatus()
		}
	}
}

// pollStatus performs one full USB+AT sample. AT I/O runs outside s.mu; all manager-port
// AT work is serialized on atMu so control cannot interleave with poll/recovery.
func (s *ModemService) pollStatus() domain.FullStatus {
	s.atMu.Lock()
	defer s.atMu.Unlock()

	modemStat := s.usb.Check("")
	now := time.Now().UTC()

	if !modemStat.Connected || s.at == nil {
		s.mu.Lock()
		s.status.Modem = modemStat
		s.status.Signal = domain.SignalInfo{}
		s.status.Network = domain.NetworkInfo{}
		s.status.SIM = domain.SIMInfo{}
		s.status.Error = ""
		if !modemStat.Connected {
			s.status.Error = "modem disconnected"
		}
		s.status.UpdatedAt = now
		s.atFailStreak = 0
		st := s.status
		s.mu.Unlock()
		return st
	}

	portPath := ""
	if s.at != nil {
		portPath = s.at.PortName()
	}
	modemStat.PortPath = portPath

	// AT I/O without holding s.mu (at.Client serializes on its own mutex).
	sig, netInfo, sim, apn, rat, err := s.at.GetFullStatus()
	if err != nil {
		return s.handleATFailure(modemStat, err)
	}

	s.mu.Lock()
	s.status.Modem = modemStat
	st := s.applyATSuccessLocked(sig, netInfo, sim, apn, rat, now)
	s.mu.Unlock()
	return st
}

// handleATFailure updates error state and may rediscover/reset. Called under atMu only.
func (s *ModemService) handleATFailure(modemStat domain.ModemStatus, err error) domain.FullStatus {
	now := time.Now().UTC()

	s.mu.Lock()
	s.status.Modem = modemStat
	if s.at != nil {
		s.status.Modem.PortPath = s.at.PortName()
	}
	s.atFailStreak++
	s.status.Error = formatDeviceError(err)
	s.status.UpdatedAt = now

	if isPermissionError(err) {
		s.logPermissionOnceLocked(err)
		st := s.status
		s.mu.Unlock()
		return st
	}

	doRediscover := time.Since(s.lastRediscover) >= s.rediscoverEvery
	if doRediscover {
		s.lastRediscover = time.Now()
		// Close/switch port while holding mu briefly; Discover may open serial — unlock first.
		s.mu.Unlock()
		s.rediscoverATPort()
		sig, netInfo, sim, apn, rat, retryErr := s.at.GetFullStatus()
		s.mu.Lock()
		if retryErr == nil {
			if s.at != nil {
				s.status.Modem.PortPath = s.at.PortName()
			}
			st := s.applyATSuccessLocked(sig, netInfo, sim, apn, rat, time.Now().UTC())
			s.mu.Unlock()
			return st
		}
		s.status.Error = formatDeviceError(retryErr)
		s.status.UpdatedAt = time.Now().UTC()
		if isPermissionError(retryErr) {
			s.logPermissionOnceLocked(retryErr)
			st := s.status
			s.mu.Unlock()
			return st
		}
		err = retryErr
	}

	s.maybeHardResetLocked(err)
	st := s.status
	s.mu.Unlock()
	return st
}

func (s *ModemService) applyATSuccessLocked(sig domain.SignalInfo, net domain.NetworkInfo, sim domain.SIMInfo, apn domain.APNConfig, rat domain.RATMode, now time.Time) domain.FullStatus {
	s.atFailStreak = 0
	s.status.Signal = sig
	s.status.Network = net
	s.status.SIM = sim
	s.status.APN = apn
	s.status.RATMode = rat
	s.status.Error = ""
	s.status.UpdatedAt = now
	if s.at != nil {
		s.status.Modem.PortPath = s.at.PortName()
	}

	if s.history != nil && (sig.RSSI != 0 || sig.Percentage != 0 || sig.RSRP != 0) {
		s.history.Add(domain.SignalSample{
			Timestamp:  now,
			RSSI:       sig.RSSI,
			RSRP:       sig.RSRP,
			RSRQ:       sig.RSRQ,
			Percentage: sig.Percentage,
			Tech:       net.Tech,
		})
	}
	return s.status
}

func (s *ModemService) logPermissionOnceLocked(err error) {
	// Avoid flooding logs every poll tick.
	if time.Since(s.lastPermLogAt) < s.rediscoverEvery {
		return
	}
	s.lastPermLogAt = time.Now()
	port := ""
	if s.at != nil {
		port = s.at.PortName()
	}
	log.Printf("[WARN] AT access denied on %s: %v — need root or membership in group 'dialout' (then re-login)", port, err)
}

func (s *ModemService) History() []domain.SignalSample {
	if s.history == nil {
		return nil
	}
	return s.history.Snapshot()
}

func (s *ModemService) SetAPN(cfg domain.APNConfig) error {
	cid := cfg.CID
	if cid == 0 {
		cid = appdefaults.DefaultCID
	}
	pdp := cfg.PDPType
	if pdp == "" {
		pdp = domain.DefaultPDPType()
	} else {
		parsed, err := domain.ParsePDPType(string(pdp))
		if err != nil {
			return err
		}
		pdp = parsed
	}
	return s.withAT(func() error {
		if s.at == nil {
			return errModemUnavailable
		}
		return s.at.SetAPN(cid, pdp, cfg.APN)
	})
}

func (s *ModemService) SetRAT(pref domain.RATModePref) error {
	return s.withAT(func() error {
		if s.at == nil {
			return errModemUnavailable
		}
		return s.at.SetRATMode(pref)
	})
}

func (s *ModemService) RawAT(cmd string) (string, error) {
	var resp string
	err := s.withAT(func() error {
		if s.at == nil {
			return errModemUnavailable
		}
		if err := s.at.EnsureConnected(); err != nil {
			return err
		}
		var err error
		resp, err = s.at.SendRaw(cmd)
		return err
	})
	return resp, err
}

func (s *ModemService) USBReset() (sysPath string, err error) {
	st := s.usb.Check("")
	if !st.Connected {
		return "", errModemUnavailable
	}
	// Serialize port close with other AT work; USB ioctl itself is not AT.
	err = s.withAT(func() error {
		if resetErr := s.usb.HardReset(st.SysPath); resetErr != nil {
			return resetErr
		}
		if s.at != nil {
			_ = s.at.Close()
		}
		return nil
	})
	return st.SysPath, err
}

func (s *ModemService) MBIMStatus() map[string]any {
	if s.mbim == nil {
		return map[string]any{"mbimcli_available": false, "device": "", "devices": []string{}, "device_present": false}
	}
	st := s.mbim.Status()
	s.mu.RLock()
	if s.selectedMBIM != "" {
		st["selected"] = s.selectedMBIM
		if st["device"] == "" || st["device"] == nil {
			st["device"] = s.selectedMBIM
		}
	}
	s.mu.RUnlock()
	return st
}

func (s *ModemService) MBIMConnect(device, apn string) (string, error) {
	if s.mbim == nil {
		return "", errModemUnavailable
	}
	if device == "" {
		s.mu.RLock()
		device = s.selectedMBIM
		s.mu.RUnlock()
	}
	if apn == "" {
		s.mu.RLock()
		apn = s.status.APN.APN
		s.mu.RUnlock()
	}
	return s.mbim.Connect(device, apn)
}

func (s *ModemService) MBIMDisconnect(device string) (string, error) {
	if s.mbim == nil {
		return "", errModemUnavailable
	}
	if device == "" {
		s.mu.RLock()
		device = s.selectedMBIM
		s.mu.RUnlock()
	}
	return s.mbim.Disconnect(device)
}

// ListModems returns discovered modems and current selection for the UI.
func (s *ModemService) ListModems() domain.ModemInventory {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildInventoryLocked()
}

// SelectModem switches active AT port / MBIM device for subsequent operations.
// Selection state uses s.mu; AT port switch uses atMu (never s.mu → atMu).
func (s *ModemService) SelectModem(req domain.ModemSelectRequest) (domain.ModemInventory, error) {
	s.mu.Lock()
	inv := s.buildInventoryLocked()
	if req.ModemID == "" && req.ATPort == "" && req.MBIMDevice == "" && req.NetIface == "" {
		s.mu.Unlock()
		return inv, fmt.Errorf("modem_id, at_port, mbim_device, or net_iface required")
	}

	var modem domain.ModemDevice
	var ok bool
	if req.ModemID != "" {
		for _, m := range inv.Modems {
			if m.ID == req.ModemID {
				modem = m
				ok = true
				break
			}
		}
		if !ok {
			s.mu.Unlock()
			return inv, fmt.Errorf("unknown modem_id %q", req.ModemID)
		}
	} else if req.ATPort != "" {
		// Find modem that owns this AT port
		for _, m := range inv.Modems {
			for _, p := range m.ATPorts {
				if p.Path == req.ATPort {
					modem = m
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			// Allow raw serial path selection
			modem = domain.ModemDevice{
				ID:   "serial:" + req.ATPort,
				Name: "Serial " + req.ATPort,
				ATPorts: []domain.ModemInterface{{
					Path: req.ATPort, Kind: domain.IfaceKindAT, ATReady: true, Label: req.ATPort,
				}},
			}
			ok = true
		}
	}

	atPort := req.ATPort
	if ok {
		if atPort == "" {
			atPort = preferredATFromModem(modem, inv.SelectedATPort)
		} else if len(modem.ATPorts) > 0 {
			found := false
			for _, p := range modem.ATPorts {
				if p.Path == atPort {
					found = true
					break
				}
			}
			if !found && !strings.HasPrefix(modem.ID, "serial:") {
				s.mu.Unlock()
				return inv, fmt.Errorf("at_port %q not on modem %s", atPort, modem.ID)
			}
		}
		s.selectedModemID = modem.ID
	}

	if req.MBIMDevice != "" {
		s.selectedMBIM = req.MBIMDevice
	} else if ok && len(modem.MBIMNodes) > 0 {
		// Keep previous if still on modem; else first
		keep := false
		for _, n := range modem.MBIMNodes {
			if n.Path == s.selectedMBIM {
				keep = true
				break
			}
		}
		if !keep {
			s.selectedMBIM = modem.MBIMNodes[0].Path
		}
	}

	if req.NetIface != "" {
		s.selectedNet = req.NetIface
	} else if ok && len(modem.NetIfaces) > 0 {
		keep := false
		for _, n := range modem.NetIfaces {
			if n.Path == s.selectedNet {
				keep = true
				break
			}
		}
		if !keep {
			s.selectedNet = modem.NetIfaces[0].Path
		}
	}

	selectedID := s.selectedModemID
	needPortSwitch := atPort != "" && s.at != nil
	s.mu.Unlock()

	if needPortSwitch {
		// Port lifecycle under atMu only (lock order: never s.mu → atMu).
		_ = s.withAT(func() error {
			if s.at == nil {
				return nil
			}
			if s.at.PortName() != atPort {
				log.Printf("[INFO] User selected AT port %s (modem %s)", atPort, selectedID)
				s.at.SetPortName(atPort)
				_ = s.at.Close()
			}
			return nil
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildInventoryLocked(), nil
}

func preferredATFromModem(m domain.ModemDevice, preferred string) string {
	if preferred != "" {
		for _, p := range m.ATPorts {
			if p.Path == preferred {
				return preferred
			}
		}
	}
	for _, p := range m.ATPorts {
		if p.ATReady {
			return p.Path
		}
	}
	if len(m.ATPorts) > 0 {
		return m.ATPorts[0].Path
	}
	return ""
}

func (s *ModemService) buildInventoryLocked() domain.ModemInventory {
	openAT := ""
	if s.at != nil {
		openAT = s.at.PortName()
	}

	var modems []domain.ModemDevice
	if s.inventory != nil {
		modems = s.inventory.ListModems(s.vendor, s.product, openAT)
	}

	// Default selection
	if s.selectedModemID == "" && openAT != "" {
		for _, m := range modems {
			for _, p := range m.ATPorts {
				if p.Path == openAT {
					s.selectedModemID = m.ID
					break
				}
			}
			if s.selectedModemID != "" {
				break
			}
		}
	}
	if s.selectedModemID == "" && len(modems) > 0 {
		s.selectedModemID = modems[0].ID
	}

	mbimCLI := false
	hint := ""
	if s.inventory != nil {
		mbimCLI = s.inventory.MBIMCLIAvailable()
		if !mbimCLI {
			hint = s.inventory.MBIMInstallHint()
		}
	}

	mbimCount, netCount := 0, 0
	for _, m := range modems {
		mbimCount += len(m.MBIMNodes)
		netCount += len(m.NetIfaces)
	}
	note := ""
	switch {
	case netCount > 0 && mbimCount == 0:
		note = "Modem is in RNDIS mode (no /dev/cdc-wdm*). Use the RNDIS network interface for data; MBIM is N/A until USB composition changes."
	case mbimCLI && mbimCount == 0 && netCount == 0:
		note = "No MBIM or RNDIS data interface found. AT monitoring still works."
	case !mbimCLI && mbimCount > 0:
		note = "mbimcli missing — install libmbim-utils for MBIM connect."
	}

	if s.selectedMBIM == "" && mbimCount > 0 {
		for _, m := range modems {
			if len(m.MBIMNodes) > 0 {
				s.selectedMBIM = m.MBIMNodes[0].Path
				break
			}
		}
	}
	if s.selectedNet == "" && netCount > 0 {
		for _, m := range modems {
			if len(m.NetIfaces) > 0 {
				s.selectedNet = m.NetIfaces[0].Path
				break
			}
		}
	}

	return domain.ModemInventory{
		Modems:          modems,
		SelectedModemID: s.selectedModemID,
		SelectedATPort:  openAT,
		SelectedMBIM:    s.selectedMBIM,
		SelectedNet:     s.selectedNet,
		MBIMCLI:         mbimCLI,
		InstallHint:     hint,
		Note:            note,
	}
}

// DataConnect brings up RNDIS (DHCP) or MBIM based on mode.
func (s *ModemService) DataConnect(req domain.DataConnectRequest) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	iface := strings.TrimSpace(req.Iface)

	s.mu.RLock()
	if iface == "" {
		switch mode {
		case domain.DataModeRNDIS, "net":
			iface = s.selectedNet
		case domain.DataModeMBIM:
			iface = s.selectedMBIM
		default:
			// auto: prefer RNDIS if selected, else MBIM
			if s.selectedNet != "" {
				mode = domain.DataModeRNDIS
				iface = s.selectedNet
			} else if s.selectedMBIM != "" {
				mode = domain.DataModeMBIM
				iface = s.selectedMBIM
			}
		}
	}
	apn := req.APN
	if apn == "" {
		apn = s.status.APN.APN
	}
	s.mu.RUnlock()

	if mode == "" || mode == "auto" {
		if strings.HasPrefix(iface, "/dev/cdc-wdm") {
			mode = domain.DataModeMBIM
		} else if iface != "" {
			mode = domain.DataModeRNDIS
		}
	}

	switch mode {
	case domain.DataModeRNDIS, "net":
		if iface == "" {
			return "", fmt.Errorf("no RNDIS/net interface selected (modem has no network iface under USB)")
		}
		if s.net == nil {
			return "", fmt.Errorf("RNDIS net helper not configured")
		}
		return s.net.ConnectRNDIS(iface)
	case domain.DataModeMBIM:
		if s.mbim == nil {
			return "", errModemUnavailable
		}
		if iface == "" {
			return "", fmt.Errorf("no MBIM device selected (no /dev/cdc-wdm*)")
		}
		return s.mbim.Connect(iface, apn)
	default:
		return "", fmt.Errorf("unknown data mode %q (use rndis or mbim)", mode)
	}
}

// DataDisconnect tears down RNDIS or MBIM session.
func (s *ModemService) DataDisconnect(req domain.DataConnectRequest) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	iface := strings.TrimSpace(req.Iface)
	s.mu.RLock()
	if iface == "" {
		if mode == domain.DataModeRNDIS || mode == "net" || mode == "" {
			iface = s.selectedNet
			if mode == "" && iface != "" {
				mode = domain.DataModeRNDIS
			}
		}
		if iface == "" || mode == domain.DataModeMBIM {
			if s.selectedMBIM != "" && (mode == domain.DataModeMBIM || mode == "") {
				iface = s.selectedMBIM
				mode = domain.DataModeMBIM
			}
		}
	}
	s.mu.RUnlock()

	switch mode {
	case domain.DataModeRNDIS, "net":
		if iface == "" {
			return "", fmt.Errorf("no RNDIS/net interface selected")
		}
		if s.net == nil {
			return "", fmt.Errorf("RNDIS net helper not configured")
		}
		return s.net.DisconnectRNDIS(iface)
	case domain.DataModeMBIM:
		if s.mbim == nil {
			return "", errModemUnavailable
		}
		return s.mbim.Disconnect(iface)
	default:
		return "", fmt.Errorf("unknown data mode %q", mode)
	}
}

// USBMode queries AT+GTUSBMODE? and returns known profiles for the UI.
func (s *ModemService) USBMode() domain.USBModeInfo {
	info := domain.USBModeInfo{
		Supported: domain.KnownUSBModes(),
		Note:      "Stock FM350 USB modes 40/41 are RNDIS+serial (not MBIM). Changing mode re-enumerates USB; reconnect may take a few seconds.",
	}
	_ = s.withAT(func() error {
		s.fillUSBModeLocked(&info)
		return nil
	})
	return info
}

// fillUSBModeLocked queries GTUSBMODE. Caller must hold atMu.
func (s *ModemService) fillUSBModeLocked(info *domain.USBModeInfo) {
	if s.at == nil {
		info.Error = "AT client unavailable"
		return
	}
	mode, err := s.at.GetUSBMode()
	if err != nil {
		info.Error = err.Error()
		return
	}
	info.Mode = mode
	info.Label = domain.USBModeLabel(mode)
}

// SetUSBMode applies AT+GTUSBMODE=<mode>, closes the AT port, and schedules rediscovery.
// Entire sequence runs under atMu so poller/control cannot race re-enumeration.
func (s *ModemService) SetUSBMode(mode int) (domain.USBModeInfo, error) {
	if mode <= 0 {
		return s.USBMode(), fmt.Errorf("invalid mode %d", mode)
	}
	var info domain.USBModeInfo
	err := s.withAT(func() error {
		info = domain.USBModeInfo{
			Supported: domain.KnownUSBModes(),
			Note:      "Stock FM350 USB modes 40/41 are RNDIS+serial (not MBIM). Changing mode re-enumerates USB; reconnect may take a few seconds.",
		}
		if s.at == nil {
			return errModemUnavailable
		}
		if setErr := s.at.SetUSBMode(mode); setErr != nil {
			s.fillUSBModeLocked(&info)
			return setErr
		}
		log.Printf("[INFO] USB composition set to GTUSBMODE=%d — waiting for re-enumeration", mode)
		// Allow device to reappear before other AT work resumes.
		time.Sleep(2 * time.Second)
		s.mu.Lock()
		s.lastRediscover = time.Time{} // allow immediate rediscover on next failure
		s.mu.Unlock()
		s.rediscoverATPort() // already under atMu
		s.fillUSBModeLocked(&info)
		if info.Mode == 0 {
			// Query may fail mid-reenumerate; report requested mode.
			info.Mode = mode
			info.Label = domain.USBModeLabel(mode)
			info.Note = "Mode command accepted; if query is empty, refresh after re-enumeration."
		}
		return nil
	})
	if info.Supported == nil {
		info.Supported = domain.KnownUSBModes()
	}
	return info, err
}

// RunWatchdog periodically enforces USB power policy until ctx is cancelled.
func (s *ModemService) RunWatchdog(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = appdefaults.WatchInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st := s.usb.Check("")
			if !st.Connected {
				log.Printf("[ALERT] FM350-GL (%s:%s) disconnected/missing", s.vendor, s.product)
			}
		}
	}
}

// rediscoverATPort closes the current AT port and probes for a working one.
// Caller must hold atMu. Must not hold s.mu (Discover may block on serial I/O).
func (s *ModemService) rediscoverATPort() {
	if s.at == nil || s.discover == nil {
		return
	}
	cur := s.at.PortName()
	_ = s.at.Close()
	discovered, err := s.discover.DiscoverATPort(s.vendor, s.product)
	if err != nil || discovered == "" {
		return
	}
	if discovered != cur {
		log.Printf("[INFO] Switching AT port %s -> %s", cur, discovered)
	} else {
		log.Printf("[INFO] Re-probing AT port %s", discovered)
	}
	s.at.SetPortName(discovered)
}

func (s *ModemService) maybeHardResetLocked(cause error) {
	if isPermissionError(cause) {
		return
	}
	if s.atFailStreak < s.failStreakMax {
		return
	}
	if time.Since(s.lastResetAt) < s.resetCooldown {
		return
	}

	// Always advance cooldown so a failed reset cannot spam every SSE tick.
	s.lastResetAt = time.Now()
	s.atFailStreak = 0

	sysPath := s.status.Modem.SysPath
	if err := s.usb.HardReset(sysPath); err != nil {
		if isPermissionError(err) {
			log.Printf("[WARN] USB hard reset skipped/failed (permission): %v — run daemon as root for USBDEVFS_RESET", err)
		} else {
			log.Printf("[WARN] USB hard reset failed: %v", err)
		}
		return
	}
	log.Printf("[INFO] USBDEVFS_RESET issued for %s", sysPath)
	if s.at != nil {
		_ = s.at.Close()
	}
}
