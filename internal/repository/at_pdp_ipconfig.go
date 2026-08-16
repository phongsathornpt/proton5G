package repository

import (
	"encoding/csv"
	"fmt"
	"net"
	"strconv"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

// CmdCGCONTRDP builds AT+CGCONTRDP=<cid>, which returns dynamic PDP
// parameters including the local address/subnet mask and gateway.
func CmdCGCONTRDP(cid int) string {
	if cid <= 0 {
		cid = 1
	}
	return fmt.Sprintf("AT+CGCONTRDP=%d", cid)
}

// QueryPDPIPConfig returns network-assigned host parameters for an active PDP
// context. Unlike CGPADDR, CGCONTRDP carries prefix and gateway data.
func (c *Client) QueryPDPIPConfig(cid int) (domain.WANIPConfig, error) {
	if cid <= 0 {
		cid = 1
	}
	if err := c.EnsureConnected(); err != nil {
		return domain.WANIPConfig{}, err
	}
	resp, err := c.SendRaw(CmdCGCONTRDP(cid))
	if err != nil {
		return domain.WANIPConfig{}, err
	}
	return ParseCGCONTRDP(resp, cid)
}

// ParseCGCONTRDP reads one IPv4 dynamic-parameter row. 3GPP represents
// IPv4 local address + mask as a.b.c.d.m1.m2.m3.m4. We convert that mask
// to CIDR and require the gateway to be present; nothing is inferred.
func ParseCGCONTRDP(response string, cid int) (domain.WANIPConfig, error) {
	if cid <= 0 {
		cid = 1
	}
	var firstErr error
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CGCONTRDP:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "+CGCONTRDP:"))
		r := csv.NewReader(strings.NewReader(payload))
		r.TrimLeadingSpace = true
		parts, err := r.Read()
		if err != nil || len(parts) < 5 {
			if firstErr == nil {
				firstErr = fmt.Errorf("invalid CGCONTRDP row %q", line)
			}
			continue
		}
		lineCID, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || lineCID != cid {
			continue
		}

		addrCIDR, err := parseCGCONTRDPIPv4Local(parts[3])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		gateway := strings.TrimSpace(parts[4])
		if !isIPv4Addr(gateway) || gateway == "0.0.0.0" {
			if firstErr == nil {
				firstErr = fmt.Errorf("CGCONTRDP returned no usable IPv4 gateway for CID %d", cid)
			}
			continue
		}

		cfg := domain.WANIPConfig{AddressCIDR: addrCIDR, Gateway: gateway}
		for _, idx := range []int{5, 6} {
			if idx < len(parts) {
				dns := strings.TrimSpace(parts[idx])
				if isIPv4Addr(dns) && dns != "0.0.0.0" {
					cfg.DNS = append(cfg.DNS, dns)
				}
			}
		}
		if len(parts) > 11 {
			if mtu, err := strconv.Atoi(strings.TrimSpace(parts[11])); err == nil && mtu > 0 {
				cfg.MTU = mtu
			}
		}
		return cfg, nil
	}
	if firstErr != nil {
		return domain.WANIPConfig{}, firstErr
	}
	return domain.WANIPConfig{}, fmt.Errorf("no IPv4 CGCONTRDP configuration for CID %d", cid)
}

func parseCGCONTRDPIPv4Local(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") {
		ip, network, err := net.ParseCIDR(raw)
		if err != nil || ip.To4() == nil {
			return "", fmt.Errorf("invalid CGCONTRDP IPv4 CIDR %q", raw)
		}
		ones, bits := network.Mask.Size()
		if bits != 32 {
			return "", fmt.Errorf("invalid CGCONTRDP IPv4 mask %q", raw)
		}
		return fmt.Sprintf("%s/%d", ip.To4().String(), ones), nil
	}

	octets := strings.Split(raw, ".")
	if len(octets) != 8 {
		return "", fmt.Errorf("CGCONTRDP IPv4 local address lacks subnet mask: %q", raw)
	}
	vals := make([]byte, 8)
	for i, octet := range octets {
		n, err := strconv.Atoi(octet)
		if err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("invalid CGCONTRDP IPv4 local address %q", raw)
		}
		vals[i] = byte(n)
	}
	mask := net.IPMask(vals[4:8])
	ones, bits := mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("non-contiguous CGCONTRDP IPv4 subnet mask %q", raw)
	}
	ip := net.IPv4(vals[0], vals[1], vals[2], vals[3]).To4()
	return fmt.Sprintf("%s/%d", ip.String(), ones), nil
}
