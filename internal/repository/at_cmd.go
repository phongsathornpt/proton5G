package repository

import (
	"fmt"

	"fm350-monitor/internal/pkg/domain"
)

// AT command strings and serial parameters.
const (
	CmdAT       = "AT"
	CmdCSQ      = "AT+CSQ"
	CmdCESQ     = "AT+CESQ"
	CmdCOPS     = "AT+COPS?"
	CmdC5GREG   = "AT+C5GREG?"
	CmdCEREG    = "AT+CEREG?"
	CmdCPIN     = "AT+CPIN?"
	CmdCIMI     = "AT+CIMI"
	CmdCCID     = "AT+CCID"
	CmdICCID    = "AT+ICCID"
	CmdCGDCONTQ = "AT+CGDCONT?"
	CmdGTACTQ   = "AT+GTACT?"
	CmdGTUSBMODEQ = "AT+GTUSBMODE?"
	// Proprietary Fibocom signal / cell info (fallback when CESQ is empty).
	CmdGTCAINFO = "AT+GTCAINFO?"
	CmdGTCCINFO = "AT+GTCCINFO?"

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

// CmdGTUSBMODESet builds AT+GTUSBMODE=<mode>.
func CmdGTUSBMODESet(mode int) string {
	return fmt.Sprintf("AT+GTUSBMODE=%d", mode)
}
