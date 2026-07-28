package repository

import (
	"os"
	"strings"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

// ParseIWStationDump extracts stations from `iw dev <wlan> station dump` output.
func ParseIWStationDump(text string) []domain.HotspotClient {
	var out []domain.HotspotClient
	var cur *domain.HotspotClient
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "Station ") {
			// Station aa:bb:cc:dd:ee:ff (on wlan0)
			fields := strings.Fields(trim)
			if len(fields) >= 2 {
				mac := strings.ToLower(fields[1])
				c := domain.HotspotClient{MAC: mac}
				out = append(out, c)
				cur = &out[len(out)-1]
			}
			continue
		}
		if cur == nil {
			continue
		}
		// optional: signal, etc. — leave for later
		_ = trim
	}
	return out
}

// ParseDnsmasqLeases parses dnsmasq dhcp.leases format:
// expiry mac ip hostname client-id
func ParseDnsmasqLeases(text string) []domain.HotspotClient {
	var out []domain.HotspotClient
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// duid lines for ipv6 start with duid — skip short MAC-less rows carefully
		mac := strings.ToLower(fields[1])
		if !strings.Contains(mac, ":") {
			continue
		}
		ip := fields[2]
		name := fields[3]
		if name == "*" {
			name = ""
		}
		out = append(out, domain.HotspotClient{
			MAC:  mac,
			IP:   ip,
			Name: name,
		})
	}
	return out
}

// MergeHotspotClients prefers station MACs and fills IP/name from leases.
func MergeHotspotClients(stations, leases []domain.HotspotClient) []domain.HotspotClient {
	byMAC := make(map[string]domain.HotspotClient)
	for _, l := range leases {
		byMAC[l.MAC] = l
	}
	var out []domain.HotspotClient
	seen := make(map[string]bool)
	for _, s := range stations {
		c := s
		if l, ok := byMAC[s.MAC]; ok {
			if c.IP == "" {
				c.IP = l.IP
			}
			if c.Name == "" {
				c.Name = l.Name
			}
		}
		out = append(out, c)
		seen[s.MAC] = true
	}
	// leases without a live station still useful (recent DHCP)
	for _, l := range leases {
		if seen[l.MAC] {
			continue
		}
		out = append(out, l)
	}
	return out
}

// ListHotspotClients gathers associated stations and DHCP leases.
func ListHotspotClients(wlan, leaseFile string) []domain.HotspotClient {
	var stations, leases []domain.HotspotClient
	if wlan != "" && lookPath("iw") {
		if out, err := runCmd(3*time.Second, "iw", "dev", wlan, "station", "dump"); err == nil {
			stations = ParseIWStationDump(out)
		}
	}
	if leaseFile != "" {
		if data, err := os.ReadFile(leaseFile); err == nil {
			leases = ParseDnsmasqLeases(string(data))
		}
	}
	return MergeHotspotClients(stations, leases)
}
