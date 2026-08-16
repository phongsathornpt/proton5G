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

const (
	fm350RNDISPDPWait         = 6 * time.Second
	fm350RNDISPDPQueryInterval = 500 * time.Millisecond
)

// RNDISService adds the FM350-specific USB RNDIS data path on top of WANService.
// FM350 RNDIS is not ordinary USB tethering: after PDP activation the host must
// configure the rndis_host interface from AT+CGPADDR and add the modem peer as
// the default gateway. DHCP remains available only as an explicit/fallback path.
type RNDISService struct {
	*WANService

	rndisMu  sync.RWMutex
	rndisWAN domain.WANInfo
	rndisPDP domain.PDPSession
	rndisSet bool
}

func NewRNDISService(wan *WANService) *RNDISService {
	return &RNDISService{WANService: wan}
}

func (r *RNDISService) Status() domain.FullStatus {
	return r.decorateRNDIS(r.WANService.Status())
}

func (r *RNDISService) CachedStatus() domain.FullStatus {
	return r.decorateRNDIS(r.WANService.CachedStatus())
}

func (r *RNDISService) decorateRNDIS(st domain.FullStatus) domain.FullStatus {
	r.rndisMu.RLock()
	wan := r.rndisWAN
	pdp := r.rndisPDP
	set := r.rndisSet
	r.rndisMu.RUnlock()
	if !set {
		return st
	}
	if wan.Session == domain.WANSessionConnected && wan.Iface != "" && st.WAN.Iface == wan.Iface {
		wan.Addrs = append([]string(nil), st.WAN.Addrs...)
		wan.RxBytes = st.WAN.RxBytes
		wan.TxBytes = st.WAN.TxBytes
		wan.RxRateBps = st.WAN.RxRateBps
		wan.TxRateBps = st.WAN.TxRateBps
	}
	st.WAN = wan
	st.PDP = pdp
	st.APN.IPAddr = pdp.IP
	return st
}

func (r *RNDISService) setRNDISState(wan domain.WANInfo, pdp domain.PDPSession) {
	r.rndisMu.Lock()
	r.rndisWAN = wan
	r.rndisPDP = pdp
	r.rndisSet = true
	r.rndisMu.Unlock()
}

func (r *RNDISService) clearRNDISState() {
	r.rndisMu.Lock()
	r.rndisWAN = domain.WANInfo{}
	r.rndisPDP = domain.PDPSession{}
	r.rndisSet = false
	r.rndisMu.Unlock()
}

func (r *RNDISService) SelectModem(req domain.ModemSelectRequest) (domain.ModemInventory, error) {
	inv, err := r.WANService.SelectModem(req)
	if err == nil {
		r.clearRNDISState()
	}
	return inv, err
}

func (r *RNDISService) MBIMConnect(device, apn string) (string, error) {
	r.clearRNDISState()
	return r.WANService.MBIMConnect(device, apn)
}

func (r *RNDISService) MBIMDisconnect(device string) (string, error) {
	r.clearRNDISState()
	return r.WANService.MBIMDisconnect(device)
}

func (r *RNDISService) DataConnect(req domain.DataConnectRequest) (string, error) {
	endpoint, err := r.resolveDataEndpoint(req)
	if err != nil {
		return "", err
	}
	if endpoint.mode != domain.DataModeRNDIS {
		r.clearRNDISState()
		return r.WANService.DataConnect(req)
	}

	// RNDIS and MBIM overrides are mutually exclusive.
	r.WANService.clearOverrides()
	return r.connectFM350RNDIS(endpoint, req.Method)
}

func (r *RNDISService) DataDisconnect(req domain.DataConnectRequest) (string, error) {
	mode, parseErr := domain.ParseDataMode(string(req.Mode))
	if parseErr != nil {
		return "", parseErr
	}

	r.rndisMu.RLock()
	currentWAN := r.rndisWAN
	currentPDP := r.rndisPDP
	currentSet := r.rndisSet
	r.rndisMu.RUnlock()

	isRNDIS := mode == domain.DataModeRNDIS
	if mode == domain.DataModeAuto || strings.TrimSpace(req.Iface) == "" {
		if currentSet && currentWAN.Session == domain.WANSessionConnected && currentWAN.Method != "" {
			isRNDIS = true
			req.Mode = domain.DataModeRNDIS
			req.Iface = currentWAN.Iface
		}
	}
	if !isRNDIS {
		r.clearRNDISState()
		return r.WANService.DataDisconnect(req)
	}

	out, err := r.WANService.DataDisconnect(req)
	if err == nil {
		cid := currentPDP.CID
		if cid <= 0 {
			cid = appdefaults.DefaultCID
		}
		r.setRNDISState(domain.WANInfo{
			Iface:   req.Iface,
			Session: domain.WANSessionDisconnected,
		}, domain.PDPSession{CID: cid})
		return out, nil
	}
	if strings.Contains(err.Error(), "PDP deactivate failed") {
		// Host teardown succeeded; only modem bearer teardown failed.
		r.setRNDISState(domain.WANInfo{
			Iface:   req.Iface,
			Session: domain.WANSessionDisconnected,
		}, currentPDP)
	}
	return out, err
}

func (r *RNDISService) connectFM350RNDIS(endpoint resolvedDataEndpoint, rawMethod domain.DataMethod) (string, error) {
	if r.ModemService.net == nil {
		return "", fmt.Errorf("RNDIS net helper not configured")
	}
	if r.ModemService.at == nil {
		return "", fmt.Errorf("FM350 RNDIS requires an AT control port")
	}
	method, err := domain.ParseDataMethod(string(rawMethod))
	if err != nil {
		return "", err
	}

	waitForPDP := method != domain.DataMethodDHCP
	pdp, pdpErr := r.activateAndQueryPDP(endpoint.cid, waitForPDP)
	var notes []string
	if pdpErr != nil {
		notes = append(notes, "PDP: "+pdpErr.Error())
	}
	if pdp.IP != "" {
		notes = append(notes, "PDP IPv4 "+pdp.IP)
	}

	// FM350 RNDIS primary path: the modem exposes no normal DHCP lease. Use the
	// PDP IPv4 as a /24 host address and the first address of that /24 as gateway.
	if method == domain.DataMethodAuto || method == domain.DataMethodStatic {
		if pdp.IP != "" {
			addrCIDR, gateway, deriveErr := deriveFM350RNDISIPv4(pdp.IP)
			if deriveErr == nil {
				pdp.Gateway = gateway
				staticOut, staticErr := r.ModemService.net.ConnectRNDISStatic(endpoint.hostIface, addrCIDR, gateway)
				if strings.TrimSpace(staticOut) != "" {
					notes = append(notes, strings.TrimSpace(staticOut))
				}
				if staticErr == nil {
					notes = append(notes, fmt.Sprintf("FM350 RNDIS %s via %s", addrCIDR, gateway))
					r.configureRNDISDNS(endpoint.hostIface, pdp, &notes)
					r.markRNDISConnected(endpoint.hostIface, domain.WANMethodStatic, pdp)
					return strings.TrimSpace(strings.Join(notes, "\n")), nil
				}
				notes = append(notes, "FM350 static: "+staticErr.Error())
				if method == domain.DataMethodStatic {
					return strings.TrimSpace(strings.Join(notes, "\n")), staticErr
				}
			} else if method == domain.DataMethodStatic {
				return strings.TrimSpace(strings.Join(notes, "\n")), deriveErr
			}
		} else if method == domain.DataMethodStatic {
			if pdpErr != nil {
				return strings.TrimSpace(strings.Join(notes, "\n")), pdpErr
			}
			return strings.TrimSpace(strings.Join(notes, "\n")), fmt.Errorf("no PDP IPv4 from AT+CGPADDR after activation")
		}
	}

	// Explicit DHCP, or Auto fallback when PDP/static setup was unavailable.
	dhcpOut, dhcpErr := r.ModemService.net.ConnectRNDIS(endpoint.hostIface)
	if strings.TrimSpace(dhcpOut) != "" {
		notes = append(notes, strings.TrimSpace(dhcpOut))
	}
	if dhcpErr != nil {
		return strings.TrimSpace(strings.Join(notes, "\n")), fmt.Errorf("RNDIS DHCP fallback failed: %w", dhcpErr)
	}
	addrs := r.ModemService.net.IfaceAddrs(endpoint.hostIface)
	if len(addrs) == 0 {
		return strings.TrimSpace(strings.Join(notes, "\n")), fmt.Errorf("RNDIS DHCP completed but %s has no IPv4 address", endpoint.hostIface)
	}
	r.markRNDISConnected(endpoint.hostIface, domain.WANMethodDHCP, pdp)
	return strings.TrimSpace(strings.Join(notes, "\n")), nil
}

func (r *RNDISService) activateAndQueryPDP(cid int, wait bool) (domain.PDPSession, error) {
	if cid <= 0 {
		cid = appdefaults.DefaultCID
	}
	var sess domain.PDPSession
	var firstErr error
	if err := r.ModemService.withAT(func() error {
		if err := r.ModemService.at.ActivatePDP(cid); err != nil {
			return err
		}
		var err error
		sess, err = r.ModemService.at.QueryPDP(cid)
		return err
	}); err != nil {
		firstErr = err
	}
	if sess.IP != "" || !wait {
		return sess, firstErr
	}

	deadline := time.Now().Add(fm350RNDISPDPWait)
	lastErr := firstErr
	for time.Now().Before(deadline) {
		time.Sleep(fm350RNDISPDPQueryInterval)
		var queryErr error
		_ = r.ModemService.withAT(func() error {
			sess, queryErr = r.ModemService.at.QueryPDP(cid)
			return nil
		})
		if queryErr == nil && sess.IP != "" {
			return sess, nil
		}
		if queryErr != nil {
			lastErr = queryErr
		}
	}
	if lastErr != nil {
		return sess, fmt.Errorf("PDP activation/query failed: %w", lastErr)
	}
	return sess, fmt.Errorf("timed out waiting for PDP IPv4 after AT+CGACT")
}

func deriveFM350RNDISIPv4(raw string) (addrCIDR, gateway string, err error) {
	ip := net.ParseIP(strings.TrimSpace(raw)).To4()
	if ip == nil {
		return "", "", fmt.Errorf("invalid PDP IPv4 %q", raw)
	}
	gw := net.IPv4(ip[0], ip[1], ip[2], 1).To4()
	if ip.Equal(gw) {
		return "", "", fmt.Errorf("PDP IPv4 %s conflicts with FM350 RNDIS gateway", ip.String())
	}
	return ip.String() + "/24", gw.String(), nil
}

func (r *RNDISService) configureRNDISDNS(iface string, pdp domain.PDPSession, notes *[]string) {
	var servers []string
	if pdp.DNS1 != "" {
		servers = append(servers, pdp.DNS1)
	}
	if pdp.DNS2 != "" && pdp.DNS2 != pdp.DNS1 {
		servers = append(servers, pdp.DNS2)
	}
	if len(servers) == 0 {
		*notes = append(*notes, "DNS warning: modem reported no DNS servers")
		return
	}
	*notes = append(*notes, "PDP DNS "+strings.Join(servers, ", "))
	if dns, ok := r.ModemService.net.(hostDNSConfigurer); ok {
		out, err := dns.ConfigureDNS(iface, servers)
		if strings.TrimSpace(out) != "" {
			*notes = append(*notes, strings.TrimSpace(out))
		}
		if err != nil {
			*notes = append(*notes, "DNS warning: "+err.Error())
		}
	}
}

func (r *RNDISService) markRNDISConnected(iface string, method domain.WANMethod, pdp domain.PDPSession) {
	r.ModemService.mu.Lock()
	r.ModemService.status.PDP = pdp
	r.ModemService.status.APN.IPAddr = pdp.IP
	r.ModemService.status.WAN.Iface = iface
	r.ModemService.status.WAN.Method = method
	r.ModemService.status.WAN.Session = domain.WANSessionConnected
	r.ModemService.mu.Unlock()

	r.setRNDISState(domain.WANInfo{
		Iface:   iface,
		Method:  method,
		Session: domain.WANSessionConnected,
	}, pdp)
}
