package repository

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

var mbimIPv4Token = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?:/\d{1,2})?\b`)

// ParseMBIMIPConfig extracts IPv4 host configuration from mbimcli
// --query-ip-configuration output. Only an address that includes an explicit
// prefix is accepted for host configuration.
func ParseMBIMIPConfig(raw string) (domain.WANIPConfig, error) {
	var cfg domain.WANIPConfig
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		matches := mbimIPv4Token.FindAllString(trimmed, -1)

		switch {
		case strings.Contains(lower, "ip [") || strings.Contains(lower, "ipv4 address"):
			for _, value := range matches {
				if strings.Contains(value, "/") {
					if ip, _, err := net.ParseCIDR(value); err == nil && ip.To4() != nil {
						cfg.AddressCIDR = value
						break
					}
				}
			}
		case strings.Contains(lower, "gateway"):
			for _, value := range matches {
				ip := strings.SplitN(value, "/", 2)[0]
				if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
					cfg.Gateway = ip
					break
				}
			}
		case strings.Contains(lower, "dns"):
			for _, value := range matches {
				ip := strings.SplitN(value, "/", 2)[0]
				if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil && !containsStringValue(cfg.DNS, ip) {
					cfg.DNS = append(cfg.DNS, ip)
				}
			}
		case strings.Contains(lower, "mtu"):
			fields := strings.FieldsFunc(trimmed, func(r rune) bool { return r < '0' || r > '9' })
			for _, field := range fields {
				if n, err := strconv.Atoi(field); err == nil && n >= 576 && n <= 65535 {
					cfg.MTU = n
				}
			}
		}
	}
	if cfg.AddressCIDR == "" {
		return cfg, fmt.Errorf("MBIM IP configuration did not contain an IPv4 address with prefix")
	}
	return cfg, nil
}

func containsStringValue(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// QueryIPConfig returns parsed host IP configuration for an active MBIM bearer.
func QueryIPConfig(device string) (domain.WANIPConfig, error) {
	raw, err := QueryIP(device)
	if err != nil {
		return domain.WANIPConfig{}, err
	}
	cfg, err := ParseMBIMIPConfig(raw)
	if err != nil {
		return cfg, fmt.Errorf("parse mbim IP configuration: %w", err)
	}
	return cfg, nil
}
