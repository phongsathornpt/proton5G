package usecase

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

type mbimIPConfigProvider interface {
	QueryIPConfig(device string) (domain.WANIPConfig, error)
}

type hostStaticConfigurer interface {
	ConfigureStatic(iface, addrCIDR, gateway string) (string, error)
}

type hostDNSConfigurer interface {
	ConfigureDNS(iface string, servers []string) (string, error)
}

type hostInterfaceCleaner interface {
	ClearInterface(iface string) (string, error)
}

type pdpDeactivator interface {
	DeactivatePDP(cid int) error
}

// WANService decorates ModemService at the HTTP boundary. It keeps the proven
// poll/watchdog implementation intact while making data-session selection and
// MBIM host configuration explicit and safe.
type WANService struct {
	*ModemService

	wanMu       sync.RWMutex
	wanOverride domain.WANInfo
	hasWAN      bool
	pdpOverride domain.PDPSession
	hasPDP      bool
}

func NewWANService(svc *ModemService) *WANService {
	return &WANService{ModemService: svc}
}

func (w *WANService) Status() domain.FullStatus {
	return w.decorateStatus(w.ModemService.Status())
}

func (w *WANService) CachedStatus() domain.FullStatus {
	return w.decorateStatus(w.ModemService.CachedStatus())
}

func (w *WANService) decorateStatus(st domain.FullStatus) domain.FullStatus {
	w.wanMu.RLock()
	wan := w.wanOverride
	hasWAN := w.hasWAN
	pdp := w.pdpOverride
	hasPDP := w.hasPDP
	w.wanMu.RUnlock()

	if hasWAN {
		// The underlying poller already samples the selected host interface. Reuse
		// only its live counters/addresses; session/method truth comes from this facade.
		if wan.Session == domain.WANSessionConnected && wan.Iface != "" && st.WAN.Iface == wan.Iface {
			wan.Addrs = append([]string(nil), st.WAN.Addrs...)
			wan.RxBytes = st.WAN.RxBytes
			wan.TxBytes = st.WAN.TxBytes
			wan.RxRateBps = st.WAN.RxRateBps
			wan.TxRateBps = st.WAN.TxRateBps
		}
		st.WAN = wan
	}
	if hasPDP {
		st.PDP = pdp
		st.APN.IPAddr = pdp.IP
	}
	return st
}

func (w *WANService) setOverrides(wan domain.WANInfo, pdp domain.PDPSession, hasPDP bool) {
	w.wanMu.Lock()
	w.wanOverride = wan
	w.hasWAN = true
	w.pdpOverride = pdp
	w.hasPDP = hasPDP
	w.wanMu.Unlock()
}

func (w *WANService) clearOverrides() {
	w.wanMu.Lock()
	w.wanOverride = domain.WANInfo{}
	w.hasWAN = false
	w.pdpOverride = domain.PDPSession{}
	w.hasPDP = false
	w.wanMu.Unlock()
}

func modemByID(inv domain.ModemInventory, id string) (domain.ModemDevice, bool) {
	for _, modem := range inv.Modems {
		if modem.ID == id {
			return modem, true
		}
	}
	return domain.ModemDevice{}, false
}

func modemOwns(items []domain.ModemInterface, path string) bool {
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func modemByPath(inv domain.ModemInventory, path string, kind domain.InterfaceKind) (domain.ModemDevice, bool) {
	for _, modem := range inv.Modems {
		var items []domain.ModemInterface
		switch kind {
		case domain.IfaceKindAT:
			items = modem.ATPorts
		case domain.IfaceKindMBIM:
			items = modem.MBIMNodes
		case domain.IfaceKindNet:
			items = modem.NetIfaces
		}
		if modemOwns(items, path) {
			return modem, true
		}
	}
	return domain.ModemDevice{}, false
}

// SelectModem validates data endpoint ownership before the underlying service is
// allowed to persist selections that can later drive host-network operations.
func (w *WANService) SelectModem(req domain.ModemSelectRequest) (domain.ModemInventory, error) {
	inv := w.ModemService.ListModems()
	var target domain.ModemDevice
	var targetOK bool

	if req.ModemID != "" {
		target, targetOK = modemByID(inv, req.ModemID)
		if !targetOK {
			return inv, fmt.Errorf("unknown modem_id %q", req.ModemID)
		}
	} else if req.ATPort != "" {
		target, targetOK = modemByPath(inv, req.ATPort, domain.IfaceKindAT)
	}

	validateDataPath := func(path string, kind domain.InterfaceKind, field string) error {
		if path == "" {
			return nil
		}
		owner, ok := modemByPath(inv, path, kind)
		if !ok {
			return fmt.Errorf("unknown %s %q; use an endpoint discovered under the modem", field, path)
		}
		if targetOK && owner.ID != target.ID {
			return fmt.Errorf("%s %q does not belong to modem %s", field, path, target.ID)
		}
		if !targetOK {
			target, targetOK = owner, true
		}
		return nil
	}
	if err := validateDataPath(req.MBIMDevice, domain.IfaceKindMBIM, "mbim_device"); err != nil {
		return inv, err
	}
	if err := validateDataPath(req.NetIface, domain.IfaceKindNet, "net_iface"); err != nil {
		return inv, err
	}
	if req.ModemID == "" && req.ATPort == "" && targetOK {
		req.ModemID = target.ID
	}

	out, err := w.ModemService.SelectModem(req)
	if err == nil {
		w.clearOverrides()
	}
	return out, err
}

type resolvedDataEndpoint struct {
	mode      domain.DataMode
	control   string
	hostIface string
	apn       string
	cid       int
}

func (w *WANService) resolveDataEndpoint(req domain.DataConnectRequest) (resolvedDataEndpoint, error) {
	mode, err := domain.ParseDataMode(string(req.Mode))
	if err != nil {
		return resolvedDataEndpoint{}, err
	}
	inv := w.ModemService.ListModems()
	selected, selectedOK := modemByID(inv, inv.SelectedModemID)
	iface := strings.TrimSpace(req.Iface)

	if mode == domain.DataModeAuto {
		switch {
		case iface != "" && iface == inv.SelectedMBIM:
			mode = domain.DataModeMBIM
		case iface != "" && selectedOK && modemOwns(selected.MBIMNodes, iface):
			mode = domain.DataModeMBIM
		case iface != "" && selectedOK && modemOwns(selected.NetIfaces, iface) && len(selected.MBIMNodes) > 0:
			// A host iface paired with cdc-wdm is part of the MBIM endpoint, not RNDIS.
			mode = domain.DataModeMBIM
			iface = inv.SelectedMBIM
		case iface != "":
			mode = domain.DataModeRNDIS
		case inv.SelectedMBIM != "":
			mode = domain.DataModeMBIM
			iface = inv.SelectedMBIM
		default:
			mode = domain.DataModeRNDIS
			iface = inv.SelectedNet
		}
	}

	endpoint := resolvedDataEndpoint{mode: mode, apn: strings.TrimSpace(req.APN)}
	st := w.ModemService.CachedStatus()
	endpoint.cid = st.APN.CID
	if endpoint.cid <= 0 {
		endpoint.cid = appdefaults.DefaultCID
	}
	if endpoint.apn == "" {
		endpoint.apn = st.APN.APN
	}

	switch mode {
	case domain.DataModeMBIM:
		if iface == "" {
			iface = inv.SelectedMBIM
		}
		if iface == "" || inv.SelectedMBIM == "" || iface != inv.SelectedMBIM {
			return endpoint, fmt.Errorf("MBIM device %q is not the selected modem control endpoint", iface)
		}
		if !selectedOK || !modemOwns(selected.MBIMNodes, iface) {
			return endpoint, fmt.Errorf("MBIM device %q does not belong to the selected modem", iface)
		}
		endpoint.control = iface
		if inv.SelectedNet != "" && modemOwns(selected.NetIfaces, inv.SelectedNet) {
			endpoint.hostIface = inv.SelectedNet
		} else if len(selected.NetIfaces) > 0 {
			endpoint.hostIface = selected.NetIfaces[0].Path
		}
		if endpoint.hostIface == "" {
			return endpoint, fmt.Errorf("selected MBIM modem has no paired host network interface")
		}
	case domain.DataModeRNDIS:
		if iface == "" {
			iface = inv.SelectedNet
		}
		if iface == "" || inv.SelectedNet == "" || iface != inv.SelectedNet {
			return endpoint, fmt.Errorf("RNDIS interface %q is not the selected modem network endpoint", iface)
		}
		if !selectedOK || !modemOwns(selected.NetIfaces, iface) {
			return endpoint, fmt.Errorf("RNDIS interface %q does not belong to the selected modem", iface)
		}
		endpoint.hostIface = iface
	default:
		return endpoint, fmt.Errorf("unknown data mode %q", mode)
	}
	return endpoint, nil
}

func (w *WANService) DataConnect(req domain.DataConnectRequest) (string, error) {
	endpoint, err := w.resolveDataEndpoint(req)
	if err != nil {
		return "", err
	}
	if endpoint.mode == domain.DataModeMBIM {
		return w.connectMBIM(endpoint.control, endpoint.hostIface, endpoint.apn)
	}

	req.Mode = domain.DataModeRNDIS
	req.Iface = endpoint.hostIface
	out, err := w.ModemService.DataConnect(req)
	if err == nil {
		w.clearOverrides()
	}
	return out, err
}

func (w *WANService) DataDisconnect(req domain.DataConnectRequest) (string, error) {
	mode, err := domain.ParseDataMode(string(req.Mode))
	if err != nil {
		return "", err
	}
	if mode == domain.DataModeAuto || req.Iface == "" {
		w.wanMu.RLock()
		if w.hasWAN && w.wanOverride.Session == domain.WANSessionConnected && w.wanOverride.Iface != "" {
			if w.hasPDP {
				mode = domain.DataModeMBIM
			}
		}
		w.wanMu.RUnlock()
	}
	if mode == domain.DataModeMBIM {
		inv := w.ModemService.ListModems()
		device := strings.TrimSpace(req.Iface)
		if device == "" || !strings.HasPrefix(device, "/dev/cdc-wdm") {
			device = inv.SelectedMBIM
		}
		return w.MBIMDisconnect(device)
	}

	endpoint, err := w.resolveDataEndpoint(domain.DataConnectRequest{Mode: domain.DataModeRNDIS, Iface: req.Iface})
	if err != nil {
		return "", err
	}
	before := w.CachedStatus().WAN
	out, disconnectErr := w.ModemService.DataDisconnect(domain.DataConnectRequest{Mode: domain.DataModeRNDIS, Iface: endpoint.hostIface})
	if disconnectErr != nil {
		// The underlying service currently marks RNDIS disconnected even when its
		// host teardown errors; preserve the pre-call truth at the HTTP boundary.
		w.setOverrides(before, domain.PDPSession{}, false)
		return out, disconnectErr
	}
	w.clearOverrides()

	if deactivator, ok := w.ModemService.at.(pdpDeactivator); ok {
		var pdpErr error
		_ = w.ModemService.withAT(func() error {
			pdpErr = deactivator.DeactivatePDP(endpoint.cid)
			return nil
		})
		if pdpErr != nil {
			return out, fmt.Errorf("host interface disconnected; PDP deactivate failed: %w", pdpErr)
		}
	}
	return out, nil
}

func (w *WANService) MBIMConnect(device, apn string) (string, error) {
	endpoint, err := w.resolveDataEndpoint(domain.DataConnectRequest{Mode: domain.DataModeMBIM, Iface: device, APN: apn})
	if err != nil {
		return "", err
	}
	return w.connectMBIM(endpoint.control, endpoint.hostIface, endpoint.apn)
}

func (w *WANService) connectMBIM(device, hostIface, apn string) (string, error) {
	out, err := w.ModemService.MBIMConnect(device, apn)
	if err != nil {
		return out, err
	}

	querier, ok := w.ModemService.mbim.(mbimIPConfigProvider)
	if !ok {
		_, _ = w.ModemService.MBIMDisconnect(device)
		return out, fmt.Errorf("MBIM repository does not expose IP configuration")
	}
	var cfg domain.WANIPConfig
	var queryErr error
	for attempt := 0; attempt < 3; attempt++ {
		cfg, queryErr = querier.QueryIPConfig(device)
		if queryErr == nil && cfg.Valid() {
			break
		}
		if attempt < 2 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if queryErr != nil || !cfg.Valid() {
		_, _ = w.ModemService.MBIMDisconnect(device)
		if queryErr == nil {
			queryErr = fmt.Errorf("MBIM returned no usable IPv4 configuration")
		}
		return out, queryErr
	}

	configurer, ok := w.ModemService.net.(hostStaticConfigurer)
	if !ok {
		_, _ = w.ModemService.MBIMDisconnect(device)
		return out, fmt.Errorf("network repository does not support protocol-provided host IP configuration")
	}
	ipOut, err := configurer.ConfigureStatic(hostIface, cfg.AddressCIDR, cfg.Gateway)
	if err != nil {
		_, _ = w.ModemService.MBIMDisconnect(device)
		return joinWANOutput(out, ipOut), fmt.Errorf("configure MBIM host interface: %w", err)
	}

	var notes []string
	if out != "" {
		notes = append(notes, strings.TrimSpace(out))
	}
	if ipOut != "" {
		notes = append(notes, strings.TrimSpace(ipOut))
	}
	if dns, ok := w.ModemService.net.(hostDNSConfigurer); ok && len(cfg.DNS) > 0 {
		dnsOut, dnsErr := dns.ConfigureDNS(hostIface, cfg.DNS)
		if strings.TrimSpace(dnsOut) != "" {
			notes = append(notes, strings.TrimSpace(dnsOut))
		}
		if dnsErr != nil {
			notes = append(notes, "DNS warning: "+dnsErr.Error())
		}
	}
	if cfg.MTU > 0 {
		notes = append(notes, fmt.Sprintf("MBIM reported MTU %d", cfg.MTU))
	}

	ip, _, _ := net.ParseCIDR(cfg.AddressCIDR)
	cid := w.ModemService.CachedStatus().APN.CID
	if cid <= 0 {
		cid = appdefaults.DefaultCID
	}
	pdp := domain.PDPSession{CID: cid, Gateway: cfg.Gateway}
	if ip != nil {
		pdp.IP = ip.String()
	}
	if len(cfg.DNS) > 0 {
		pdp.DNS1 = cfg.DNS[0]
	}
	if len(cfg.DNS) > 1 {
		pdp.DNS2 = cfg.DNS[1]
	}
	w.setOverrides(domain.WANInfo{
		Iface:   hostIface,
		Session: domain.WANSessionConnected,
	}, pdp, true)
	return strings.TrimSpace(strings.Join(notes, "\n")), nil
}

func (w *WANService) MBIMDisconnect(device string) (string, error) {
	inv := w.ModemService.ListModems()
	if device == "" {
		device = inv.SelectedMBIM
	}
	if device == "" || device != inv.SelectedMBIM {
		return "", fmt.Errorf("MBIM device %q is not the selected modem control endpoint", device)
	}
	selected, ok := modemByID(inv, inv.SelectedModemID)
	if !ok || !modemOwns(selected.MBIMNodes, device) {
		return "", fmt.Errorf("MBIM device %q does not belong to the selected modem", device)
	}
	hostIface := inv.SelectedNet
	if hostIface == "" || !modemOwns(selected.NetIfaces, hostIface) {
		if len(selected.NetIfaces) > 0 {
			hostIface = selected.NetIfaces[0].Path
		}
	}

	out, err := w.ModemService.MBIMDisconnect(device)
	if err != nil {
		return out, err
	}
	var cleanupOut string
	var cleanupErr error
	if hostIface != "" {
		if cleaner, ok := w.ModemService.net.(hostInterfaceCleaner); ok {
			cleanupOut, cleanupErr = cleaner.ClearInterface(hostIface)
		}
	}
	cid := w.ModemService.CachedStatus().APN.CID
	if cid <= 0 {
		cid = appdefaults.DefaultCID
	}
	w.setOverrides(domain.WANInfo{
		Iface:   hostIface,
		Session: domain.WANSessionDisconnected,
	}, domain.PDPSession{CID: cid}, true)
	if cleanupErr != nil {
		return joinWANOutput(out, cleanupOut), fmt.Errorf("MBIM bearer disconnected; host cleanup failed: %w", cleanupErr)
	}
	return joinWANOutput(out, cleanupOut), nil
}

func (w *WANService) MBIMStatus() map[string]any {
	st := w.ModemService.MBIMStatus()
	w.wanMu.RLock()
	defer w.wanMu.RUnlock()
	if w.hasWAN {
		st["session"] = w.wanOverride.Session
		st["host_iface"] = w.wanOverride.Iface
	}
	if w.hasPDP && w.pdpOverride.IP != "" {
		st["ip"] = w.pdpOverride.IP
	}
	return st
}

func joinWANOutput(parts ...string) string {
	var out []string
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return strings.Join(out, "\n")
}
