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
	RATMode5GSA        RATMode = "5G SA"
	RATModeENDC        RATMode = "5G NSA (EN-DC)"
	RATModeLTEOnly     RATMode = "LTE Only"
	RATModeAuto        RATMode = "Auto (5G/LTE/3G)"
	RATModeUnspecified RATMode = "Auto"
)

// GTACT wire codes (Fibocom AT+GTACT).
const (
	GTACTLTEOnly = 4
	GTACT5GOnly  = 14
	GTACT5GSA    = 14
	GTACTENDC    = 17
	GTACTAuto    = 20
)

// RATModePref is the API token used by POST /api/rat and the WebUI buttons.
type RATModePref string

const (
	RATPref5G   RATModePref = "5g"
	RATPref5GSA RATModePref = "5g-sa"
	RATPrefENDC RATModePref = "endc"
	RATPrefNSA  RATModePref = "nsa"
	RATPrefLTE  RATModePref = "lte"
	RATPrefAuto RATModePref = "auto"
)

// ParseRATModePref parses API mode tokens (case-insensitive).
func ParseRATModePref(s string) (RATModePref, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(RATPrefENDC), string(RATPrefNSA), "5g-nsa", "5g_nsa", "5g-endc", "5g_endc":
		return RATPrefENDC, nil
	case string(RATPref5GSA), "5g_sa", "sa":
		return RATPref5GSA, nil
	case string(RATPref5G), "5g-only", "5g_only":
		return RATPref5G, nil
	case string(RATPrefLTE), "4g", "lte-only", "lte_only":
		return RATPrefLTE, nil
	case string(RATPrefAuto), "all":
		return RATPrefAuto, nil
	default:
		return "", fmt.Errorf("invalid RAT mode %q (want endc|nsa|5g-sa|5g|lte|auto)", s)
	}
}

// GTACTCode returns the AT+GTACT numeric mode for this preference.
func (p RATModePref) GTACTCode() int {
	switch p {
	case RATPrefENDC, RATPrefNSA:
		return GTACTENDC
	case RATPref5G, RATPref5GSA:
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
	case RATPrefENDC, RATPrefNSA:
		return RATModeENDC
	case RATPref5GSA:
		return RATMode5GSA
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
	case GTACTENDC:
		return RATModeENDC
	case GTACT5GOnly:
		return RATMode5GOnly
	case GTACTLTEOnly, 2:
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
	case RATModeENDC:
		return GTACTENDC
	case RATMode5GOnly, RATMode5GSA:
		return GTACT5GOnly
	case RATModeLTEOnly:
		return GTACTLTEOnly
	case RATModeAuto, RATModeUnspecified, "":
		return GTACTAuto
	default:
		return GTACTAuto
	}
}
