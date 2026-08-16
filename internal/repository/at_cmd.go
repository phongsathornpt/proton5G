package repository

import (
	"fmt"

	"fm350-monitor/internal/pkg/domain"
)

// AT command strings and serial parameters.
const (
	CmdAT         = "AT"
	CmdCSQ        = "AT+CSQ"
	CmdCESQ       = "AT+CESQ"
	CmdCOPS       = "AT+COPS?"
	CmdC5GREG     = "AT+C5GREG?"
	CmdCEREG      = "AT+CEREG?"
	CmdCPIN       = "AT+CPIN?"
	CmdCIMI       = "AT+CIMI"
	CmdCCID       = "AT+CCID"
	CmdICCID      = "AT+ICCID"
	CmdCGDCONTQ   = "AT+CGDCONT?"
	CmdGTACTQ     = "AT+GTACT?"
	CmdGTUSBMODEQ = "AT+GTUSBMODE?"
	CmdE5GOPTQ    = "AT+E5GOPT?"
	// Proprietary Fibocom signal / cell info (fallback when CESQ is empty).
	CmdGTCAINFO = "AT+GTCAINFO?"
	CmdGTCCINFO = "AT+GTCCINFO?"
	// Parameterized Fibocom GTACT mode strings.
	CmdGTACTSetENDC = "AT+GTACT=17,3,6,0"
	CmdGTACTSet5GSA = "AT+GTACT=14,6,6,0"
	CmdGTACTSetAuto = "AT+GTACT=20,6,3,0"
	CmdGTACTSetLTE  = "AT+GTACT=2,3,3,0"
	// Thailand radio presets. These stay in the repository layer because they are
	// Fibocom wire-protocol values. B40/B41 use the 100+LTE-band encoding.
	// NR n41 band filtering is firmware-dependent; profiles therefore prefer
	// Thailand LTE anchors and EN-DC, then leave NR carrier choice to the modem/network.
	CmdGTACTSetTHNSA       = "AT+GTACT=17,3,6,101,103,108,128,140,141"
	CmdGTACTSetTHNSAB40N41 = "AT+GTACT=17,3,6,140,141"
	CmdGTACTSetTHLTE       = "AT+GTACT=2,3,3,101,103,108,128,140,141"
	CmdGTACTSetTHLTEB40B41 = "AT+GTACT=2,3,3,140,141"
	// Identity (polled once until IMEI is known).
	CmdCGMI = "AT+CGMI"
	CmdCGMM = "AT+CGMM"
	CmdCGMR = "AT+CGMR"
	CmdCGSN = "AT+CGSN"
	// Temperature: +GTSENRDTEMP: 1,<milliC>
	CmdGTSENRDTEMP = "AT+GTSENRDTEMP=1"

	ATResultOK    = "OK"
	ATResultERROR = "ERROR"

	SerialBaud     = 115200
	SerialDataBits = 8

	// CSQ / CESQ mapping constants
	CSQUnknown  = 99
	CSQMinDBm   = -113
	CSQMaxRaw   = 31
	CESQUnknown = 255
	RSRPMinDBm  = -140
	RSRPMaxRaw  = 97
	RSRQMaxRaw  = 34
)

// CmdCGDCONTSet builds AT+CGDCONT=<cid>,"<pdp>","<apn>".
func CmdCGDCONTSet(cid int, pdp domain.PDPType, apn string) string {
	return fmt.Sprintf("AT+CGDCONT=%d,\"%s\",\"%s\"", cid, pdp, apn)
}

// CmdGTACTSet builds AT+GTACT=<code>.
func CmdGTACTSet(code int) string {
	return fmt.Sprintf("AT+GTACT=%d", code)
}

// CmdGTACTSetByPref returns the preferred full GTACT command string for a preference.
func CmdGTACTSetByPref(pref domain.RATModePref) string {
	switch pref {
	case domain.RATPrefENDC, domain.RATPrefNSA:
		return CmdGTACTSetENDC
	case domain.RATPrefTHNSA:
		return CmdGTACTSetTHNSA
	case domain.RATPrefTHNSAB40N41:
		return CmdGTACTSetTHNSAB40N41
	case domain.RATPref5GSA:
		return CmdGTACTSet5GSA
	case domain.RATPref5G:
		return CmdGTACTSet(domain.GTACT5GOnly)
	case domain.RATPrefLTE:
		return CmdGTACTSet(domain.GTACTLTEOnly)
	case domain.RATPrefTHLTE:
		return CmdGTACTSetTHLTE
	case domain.RATPrefTHLTEB40B41:
		return CmdGTACTSetTHLTEB40B41
	default:
		return CmdGTACTSetAuto
	}
}

// CmdE5GOPTSet builds AT+E5GOPT=<mode>.
func CmdE5GOPTSet(mode int) string {
	return fmt.Sprintf("AT+E5GOPT=%d", mode)
}

// CmdGTUSBMODESet builds AT+GTUSBMODE=<mode>.
func CmdGTUSBMODESet(mode int) string {
	return fmt.Sprintf("AT+GTUSBMODE=%d", mode)
}

// CmdCGPADDR builds AT+CGPADDR=<cid>.
func CmdCGPADDR(cid int) string {
	if cid <= 0 {
		cid = 1
	}
	return fmt.Sprintf("AT+CGPADDR=%d", cid)
}

// CmdGTDNS builds AT+GTDNS=<cid> (query DNS for PDP context).
func CmdGTDNS(cid int) string {
	if cid <= 0 {
		cid = 1
	}
	return fmt.Sprintf("AT+GTDNS=%d", cid)
}

// CmdCGACTSet builds AT+CGACT=<state>,<cid>.
func CmdCGACTSet(cid int, active bool) string {
	if cid <= 0 {
		cid = 1
	}
	state := 0
	if active {
		state = 1
	}
	return fmt.Sprintf("AT+CGACT=%d,%d", state, cid)
}
