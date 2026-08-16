package usecase

import (
	"fmt"
	"strings"
	"time"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

type atPDPIPConfigProvider interface {
	QueryPDPIPConfig(cid int) (domain.WANIPConfig, error)
}

const (
	rndisPDPConfigWait         = 6 * time.Second
	rndisPDPConfigPollInterval = 500 * time.Millisecond
)

func (s *ModemService) connectRNDISConfigured(iface string, rawMethod domain.DataMethod, cid int) (string, error) {
	method, err := domain.ParseDataMethod(string(rawMethod))
	if err != nil {
		return "", err
	}
	if cid <= 0 {
		cid = appdefaults.DefaultCID
	}

	var notes []string
	var pdp domain.PDPSession
	if s.at != nil {
		var activateErr error
		var queryErr error
		_ = s.withAT(func() error {
			activateErr = s.at.ActivatePDP(cid)
			if activateErr == nil {
				pdp, queryErr = s.at.QueryPDP(cid)
			}
			return nil
		})
		if activateErr != nil {
			return "", fmt.Errorf("activate PDP context %d: %w", cid, activateErr)
		}
		if queryErr != nil {
			notes = append(notes, "PDP telemetry: "+queryErr.Error())
		}
		if pdp.IP != "" {
			notes = append(notes, "PDP "+pdp.IP)
			s.cachePDPSession(pdp)
		}
	}

	if method == domain.DataMethodAuto || method == domain.DataMethodStatic {
		cfg, cfgErr := s.waitRNDISIPConfig(cid)
		if cfgErr == nil {
			staticOut, staticErr := s.net.ConnectRNDISStatic(iface, cfg.AddressCIDR, cfg.Gateway)
			if strings.TrimSpace(staticOut) != "" {
				notes = append(notes, strings.TrimSpace(staticOut))
			}
			if staticErr == nil {
				pdp.Gateway = cfg.Gateway
				if pdp.IP == "" {
					pdp.IP = strings.SplitN(cfg.AddressCIDR, "/", 2)[0]
				}
				if len(cfg.DNS) > 0 {
					pdp.DNS1 = cfg.DNS[0]
				}
				if len(cfg.DNS) > 1 {
					pdp.DNS2 = cfg.DNS[1]
				}
				s.cachePDPSession(pdp)
				s.configureRNDISDNS(iface, cfg, &notes)
				s.setWANMethod(domain.WANMethodStatic)
				notes = append(notes, fmt.Sprintf("RNDIS %s via %s (modem-reported)", cfg.AddressCIDR, cfg.Gateway))
				return strings.TrimSpace(strings.Join(notes, "\n")), nil
			}
			notes = append(notes, "static: "+staticErr.Error())
			if method == domain.DataMethodStatic {
				return strings.TrimSpace(strings.Join(notes, "\n")), staticErr
			}
		} else {
			notes = append(notes, "modem config: "+cfgErr.Error())
			if method == domain.DataMethodStatic {
				return strings.TrimSpace(strings.Join(notes, "\n")), cfgErr
			}
		}
	}

	if method != domain.DataMethodAuto && method != domain.DataMethodDHCP {
		return strings.TrimSpace(strings.Join(notes, "\n")), fmt.Errorf("unsupported RNDIS address method %q", method)
	}
	out, dhcpErr := s.net.ConnectRNDIS(iface)
	if strings.TrimSpace(out) != "" {
		notes = append(notes, strings.TrimSpace(out))
	}
	if dhcpErr != nil {
		return strings.TrimSpace(strings.Join(notes, "\n")), dhcpErr
	}
	s.setWANMethod(domain.WANMethodDHCP)
	return strings.TrimSpace(strings.Join(notes, "\n")), nil
}

func (s *ModemService) waitRNDISIPConfig(cid int) (domain.WANIPConfig, error) {
	provider, ok := s.at.(atPDPIPConfigProvider)
	if !ok {
		return domain.WANIPConfig{}, fmt.Errorf("AT repository does not expose +CGCONTRDP dynamic PDP configuration")
	}
	deadline := time.Now().Add(rndisPDPConfigWait)
	var lastErr error
	for {
		var cfg domain.WANIPConfig
		var queryErr error
		_ = s.withAT(func() error {
			cfg, queryErr = provider.QueryPDPIPConfig(cid)
			return nil
		})
		if queryErr == nil && cfg.AddressCIDR != "" && cfg.Gateway != "" {
			return cfg, nil
		}
		if queryErr != nil {
			lastErr = queryErr
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(rndisPDPConfigPollInterval)
	}
	if lastErr != nil {
		return domain.WANIPConfig{}, fmt.Errorf("CGCONTRDP unavailable for CID %d: %w", cid, lastErr)
	}
	return domain.WANIPConfig{}, fmt.Errorf("CGCONTRDP returned no usable IPv4 address/prefix/gateway for CID %d", cid)
}

func (s *ModemService) cachePDPSession(pdp domain.PDPSession) {
	s.mu.Lock()
	s.status.PDP = pdp
	if pdp.IP != "" {
		s.status.APN.IPAddr = pdp.IP
	}
	s.mu.Unlock()
}

func (s *ModemService) configureRNDISDNS(iface string, cfg domain.WANIPConfig, notes *[]string) {
	if len(cfg.DNS) == 0 {
		return
	}
	dns, ok := s.net.(hostDNSConfigurer)
	if !ok {
		*notes = append(*notes, "DNS warning: network repository cannot apply modem DNS")
		return
	}
	out, err := dns.ConfigureDNS(iface, cfg.DNS)
	if strings.TrimSpace(out) != "" {
		*notes = append(*notes, strings.TrimSpace(out))
	}
	if err != nil {
		*notes = append(*notes, "DNS warning: "+err.Error())
	}
}
