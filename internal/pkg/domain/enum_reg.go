package domain

import "strconv"

// RegState is 3GPP network registration state (human-readable).
type RegState string

const (
	RegNotRegistered RegState = "Not Registered"
	RegHome          RegState = "Registered (Home)"
	RegSearching     RegState = "Searching"
	RegDenied        RegState = "Denied"
	RegUnknown       RegState = "Unknown"
	RegRoaming       RegState = "Registered (Roaming)"
)

// 3GPP <stat> codes used by +CEREG / +C5GREG.
const (
	RegStatNotRegistered = 0
	RegStatHome          = 1
	RegStatSearching     = 2
	RegStatDenied        = 3
	RegStatUnknown       = 4
	RegStatRoaming       = 5
)

// ParseRegStat maps a 3GPP registration stat code to RegState.
func ParseRegStat(code int) RegState {
	switch code {
	case RegStatHome:
		return RegHome
	case RegStatSearching:
		return RegSearching
	case RegStatDenied:
		return RegDenied
	case RegStatUnknown:
		return RegUnknown
	case RegStatRoaming:
		return RegRoaming
	default:
		return RegNotRegistered
	}
}

// ParseRegStatString parses a numeric stat field from AT responses.
func ParseRegStatString(s string) RegState {
	code, err := strconv.Atoi(s)
	if err != nil {
		return RegUnknown
	}
	return ParseRegStat(code)
}

// IsRegistered reports home or roaming registration.
func (r RegState) IsRegistered() bool {
	return r == RegHome || r == RegRoaming
}
