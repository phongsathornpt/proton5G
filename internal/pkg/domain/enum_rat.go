package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// RATMode is the display / status preferred radio access technology mode.
type RATMode string

const (
	RATMode5GOnly      RATMode = "5G Only"
	RATModeLTEOnly     RATMode = "LTE Only"
	RATModeAuto        RATMode = "Auto (5G/LTE/3G)"
	RATModeUnspecified RATMode = "Auto"
)

// GTACT wire codes (Fibocom AT+GTACT).
const (
	GTACT5GOnly  = 14
	GTACTLTEOnly = 4
	GTACTAuto    = 20
)

// RATModePref is the API token used by POST /api/rat and the WebUI buttons.
type RATModePref string

const (
	RATPref5G   RATModePref = "5g"
	RATPrefLTE  RATModePref = "lte"
	RATPrefAuto RATModePref = "auto"
)

// ParseRATModePref parses API mode tokens (case-insensitive).
func ParseRATModePref(s string) (RATModePref, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(RATPref5G):
		return RATPref5G, nil
	case string(RATPrefLTE):
		return RATPrefLTE, nil
	case string(RATPrefAuto):
		return RATPrefAuto, nil
	default:
		return "", fmt.Errorf("invalid RAT mode %q (want 5g|lte|auto)", s)
	}
}

// GTACTCode returns the AT+GTACT numeric mode for this preference.
func (p RATModePref) GTACTCode() int {
	switch p {
	case RATPref5G:
		return GTACT5GOnly
	case RATPrefLTE:
		return GTACTLTEOnly
	default:
		return GTACTAuto
	}
}

// ToDisplay maps an API preference to a display RATMode.
func (p RATModePref) ToDisplay() RATMode {
	switch p {
	case RATPref5G:
		return RATMode5GOnly
	case RATPrefLTE:
		return RATModeLTEOnly
	default:
		return RATModeAuto
	}
}

// ParseGTACTCode maps a GTACT numeric code to display RATMode.
func ParseGTACTCode(code int) RATMode {
	switch code {
	case GTACT5GOnly:
		return RATMode5GOnly
	case GTACTLTEOnly:
		return RATModeLTEOnly
	case GTACTAuto:
		return RATModeAuto
	default:
		return RATMode(fmt.Sprintf("Mode %d", code))
	}
}

// ParseGTACTCodeString parses a numeric GTACT code string.
func ParseGTACTCodeString(s string) RATMode {
	code, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return RATMode(fmt.Sprintf("Mode %s", strings.TrimSpace(s)))
	}
	return ParseGTACTCode(code)
}

// GTACTCode returns wire code for known display modes (default auto).
func (m RATMode) GTACTCode() int {
	switch m {
	case RATMode5GOnly:
		return GTACT5GOnly
	case RATModeLTEOnly:
		return GTACTLTEOnly
	case RATModeAuto, RATModeUnspecified, "":
		return GTACTAuto
	default:
		return GTACTAuto
	}
}
