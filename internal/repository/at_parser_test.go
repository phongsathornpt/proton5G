package repository

import (
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestParseCSQ(t *testing.T) {
	resp := "\r\n+CSQ: 20,99\r\n\r\nOK\r\n"
	sig := ParseCSQ(resp)

	if sig.RSSI != -73 {
		t.Fatalf("expected RSSI -73, got %d", sig.RSSI)
	}
	if sig.Percentage != 64 {
		t.Fatalf("expected Percentage 64, got %d", sig.Percentage)
	}
}

func TestParseCESQ(t *testing.T) {
	resp := "+CESQ: 99,99,255,255,14,40\r\nOK\r\n"
	rsrp, rsrq, ok := ParseCESQ(resp)
	if !ok {
		t.Fatal("expected ok")
	}
	if rsrp != -100 {
		t.Fatalf("expected rsrp -100, got %d", rsrp)
	}
	if rsrq != -12 && rsrq != -13 {
		t.Fatalf("expected rsrq ~-12.5 rounded, got %d", rsrq)
	}

	base := ParseCSQ("+CSQ: 99,99")
	merged := MergeExtendedSignal(base, resp)
	if merged.RSRP != -100 {
		t.Fatalf("merge rsrp: %d", merged.RSRP)
	}
	if merged.Percentage == 0 {
		t.Fatal("expected percentage from rsrp when csq unknown")
	}
}

func TestParseCIMI(t *testing.T) {
	resp := "\r\n310260123456789\r\n\r\nOK\r\n"
	if got := ParseCIMI(resp); got != "310260123456789" {
		t.Fatalf("got %q", got)
	}
}

func TestParseICCID(t *testing.T) {
	resp := `+CCID: "89014103211118510720"` + "\r\nOK\r\n"
	if got := ParseICCID(resp); got != "89014103211118510720" {
		t.Fatalf("got %q", got)
	}
	resp2 := "+ICCID: 89014103211118510720,0\r\nOK\r\n"
	if got := ParseICCID(resp2); got != "89014103211118510720" {
		t.Fatalf("got %q", got)
	}
}

func TestParseCOPS(t *testing.T) {
	resp := `+COPS: 0,0,"T-Mobile",13`
	oper := ParseCOPS(resp)
	if oper != "T-Mobile" {
		t.Fatalf("expected T-Mobile, got %s", oper)
	}
}

func TestParseRegistration(t *testing.T) {
	reg := ParseRegistration("+C5GREG: 0,1")
	if reg != domain.RegHome {
		t.Fatalf("expected RegHome, got %s", reg)
	}
}

func TestParseCGDCONT(t *testing.T) {
	resp := `+CGDCONT: 1,"IPV4V6","internet","0.0.0.0",0,0`
	apn := ParseCGDCONT(resp)
	if apn.APN != "internet" || apn.PDPType != domain.PDPIPV4V6 {
		t.Fatalf("expected internet/IPV4V6, got %s/%s", apn.APN, apn.PDPType)
	}
}

func TestParseGTACT(t *testing.T) {
	mode := ParseGTACT("+GTACT: 14,14")
	if mode != domain.RATMode5GOnly {
		t.Fatalf("expected 5G Only, got %s", mode)
	}
}

func TestParseGTCAINFO(t *testing.T) {
	resp := `+GTCAINFO:
PCC:5078,940,641760,450,2,1,1,3,19,-1,-80
SCC 3:2,0,103,216,1444,50,255,4,255,3,255,13,-9,-81
OK`
	rsrp, rsrq, ok := ParseGTCAINFO(resp)
	if !ok {
		t.Fatal("expected ok")
	}
	// Best RSRP among -80 and -81 is -80
	if rsrp != -80 {
		t.Fatalf("rsrp=%d", rsrp)
	}
	_ = rsrq
}

func TestParseGTCCINFO(t *testing.T) {
	resp := `+GTCCINFO:
1,4,262,1,05D5,0019BF801,1300,358,103,100,13,60,60,22
OK`
	rsrp, _, ok := ParseGTCCINFO(resp)
	if !ok {
		t.Fatal("expected ok")
	}
	// strongest raw among trailing fields: 60 → -80 dBm
	if rsrp != -80 {
		t.Fatalf("expected rsrp -80, got %d", rsrp)
	}
}

func TestMergeExtendedSignalProprietaryFallback(t *testing.T) {
	base := ParseCSQ("+CSQ: 20,99")
	// CESQ unknown
	cesq := "+CESQ: 99,99,255,255,255,255"
	gtca := "PCC:5078,940,641760,450,2,1,1,3,19,-1,-90\nOK"
	merged := MergeExtendedSignal(base, cesq, gtca)
	if merged.RSRP != -90 {
		t.Fatalf("expected proprietary rsrp -90, got %d", merged.RSRP)
	}
	if merged.RSSI != -73 {
		t.Fatalf("CSQ RSSI should remain, got %d", merged.RSSI)
	}
}

func TestParseGTUSBMODE(t *testing.T) {
	if n := ParseGTUSBMODE("+GTUSBMODE: 41\r\nOK"); n != 41 {
		t.Fatalf("got %d", n)
	}
}
