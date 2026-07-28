package domain

import (
	"fmt"
	"strings"
)

// PDPType is the PDP context type for AT+CGDCONT.
type PDPType string

const (
	PDPIPV4V6 PDPType = "IPV4V6"
	PDPIP     PDPType = "IP"
	PDPIPV6   PDPType = "IPV6"
)

// DefaultPDPType is used when APN requests omit pdp_type.
func DefaultPDPType() PDPType { return PDPIPV4V6 }

// ParsePDPType validates a PDP type string.
func ParsePDPType(s string) (PDPType, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", string(PDPIPV4V6):
		return PDPIPV4V6, nil
	case string(PDPIP):
		return PDPIP, nil
	case string(PDPIPV6):
		return PDPIPV6, nil
	default:
		return "", fmt.Errorf("invalid PDP type %q (want IP|IPV6|IPV4V6)", s)
	}
}

// NormalizePDPType returns a known type or the raw value if non-empty unknown.
func NormalizePDPType(s string) PDPType {
	p, err := ParsePDPType(s)
	if err != nil {
		if strings.TrimSpace(s) == "" {
			return DefaultPDPType()
		}
		return PDPType(strings.TrimSpace(s))
	}
	return p
}
