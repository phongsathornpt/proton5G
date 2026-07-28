package domain

import "strings"

// SIMState is SIM / PIN readiness from AT+CPIN.
type SIMState string

const (
	SIMMissing SIMState = "MISSING"
	SIMReady   SIMState = "READY"
	SIMPIN     SIMState = "SIM PIN"
	SIMPUK     SIMState = "SIM PUK"
)

// NormalizeSIMState maps known CPIN values; unknown firmware strings are preserved.
func NormalizeSIMState(raw string) SIMState {
	s := strings.TrimSpace(raw)
	if s == "" {
		return SIMMissing
	}
	switch strings.ToUpper(s) {
	case string(SIMReady):
		return SIMReady
	case "SIM PIN":
		return SIMPIN
	case "SIM PUK":
		return SIMPUK
	case string(SIMMissing):
		return SIMMissing
	default:
		return SIMState(s)
	}
}
