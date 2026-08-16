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

func TestParseCOPSFull(t *testing.T) {
	resp := `+COPS: 0,0,"T-Mobile",13`
	oper, act, hasAct := ParseCOPSFull(resp)
	if oper != "T-Mobile" || !hasAct || act != 13 {
		t.Fatalf("expected T-Mobile / 13, got %s / %d (hasAct=%v)", oper, act, hasAct)
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
	modeENDC := ParseGTACT("+GTACT: 17,3,6,0")
	if modeENDC != domain.RATModeENDC {
		t.Fatalf("expected 5G NSA (EN-DC), got %s", modeENDC)
	}
}

func TestDetectRadioTech(t *testing.T) {
	// Test 1: COPS AcT 13 (EN-DC) with cell list
	cells := []domain.CellInfo{
		{Serving: true, RAT: domain.TechLTE, Band: "3", PCI: "100"},
		{Serving: false, RAT: domain.Tech5GNR, Band: "78", PCI: "200"},
	}
	tech, endc := DetectRadioTech(13, true, domain.RegHome, domain.RegNotRegistered, cells, nil)
	if tech != domain.Tech5GNSA || !endc.Active || endc.AnchorBand != "B3" || endc.NRBand != "n78" {
		t.Fatalf("expected active EN-DC, got tech=%s endc=%+v", tech, endc)
	}

	// Test 2: 5G SA
	cellsSA := []domain.CellInfo{
		{Serving: true, RAT: domain.Tech5GNR, Band: "78", PCI: "200"},
	}
	techSA, endcSA := DetectRadioTech(11, true, domain.RegNotRegistered, domain.RegHome, cellsSA, nil)
	if techSA != domain.Tech5GSA || endcSA.Active || endcSA.NRBand != "n78" {
		t.Fatalf("expected 5G SA, got tech=%s endc=%+v", techSA, endcSA)
	}

	// Test 3: LTE Only
	cellsLTE := []domain.CellInfo{
		{Serving: true, RAT: domain.TechLTE, Band: "20", PCI: "300"},
	}
	techLTE, endcLTE := DetectRadioTech(7, true, domain.RegHome, domain.RegNotRegistered, cellsLTE, nil)
	if techLTE != domain.TechLTE || endcLTE.Active || endcLTE.AnchorBand != "B20" {
		t.Fatalf("expected LTE, got tech=%s endc=%+v", techLTE, endcLTE)
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

func TestParseGTSENRDTEMP(t *testing.T) {
	c, ok := ParseGTSENRDTEMP("+GTSENRDTEMP: 1,56736\r\nOK")
	if !ok || c < 56.7 || c > 56.8 {
		t.Fatalf("got %v ok=%v", c, ok)
	}
	c, ok = ParseGTSENRDTEMP("+GTSENRDTEMP: 42.5")
	if !ok || c != 42.5 {
		t.Fatalf("celsius passthrough %v %v", c, ok)
	}
}

func TestParseCGPADDRAndGTDNS(t *testing.T) {
	if ip := ParseCGPADDR(`+CGPADDR: 1,"10.64.1.2"`); ip != "10.64.1.2" {
		t.Fatalf("ip=%q", ip)
	}
	if ip := ParseCGPADDR(`+CGPADDR: 1,"0.0.0.0"`); ip != "" {
		t.Fatalf("zero ip=%q", ip)
	}
	d1, d2 := ParseGTDNS(`+GTDNS: 1,"8.8.8.8","1.1.1.1"`)
	if d1 != "8.8.8.8" || d2 != "1.1.1.1" {
		t.Fatalf("%q %q", d1, d2)
	}
}

func TestParseInfoLine(t *testing.T) {
	if got := ParseInfoLine("AT+CGMI\r\nFibocom\r\nOK"); got != "Fibocom" {
		t.Fatalf("got %q", got)
	}
	if got := ParseInfoLine("+CGSN: 123456789012345\r\nOK"); got != "123456789012345" {
		t.Fatalf("got %q", got)
	}
}

func TestParseGTCCINFOCells(t *testing.T) {
	resp := `+GTCCINFO:
1,4,262,1,05D5,0019BF801,1300,358,103,100,13,60,60,22
0,9,262,1,05D5,0019BF802,640000,100,78,100,20,70,18
OK`
	cells := ParseGTCCINFOCells(resp)
	if len(cells) != 2 {
		t.Fatalf("len=%d", len(cells))
	}
	if !cells[0].Serving || cells[0].RAT != domain.TechLTE || cells[0].RSRP != -80 {
		t.Fatalf("lte serving: %+v", cells[0])
	}
	if cells[0].SINR != 6 { // 13/2
		t.Fatalf("sinr=%d", cells[0].SINR)
	}
	if cells[1].Serving || cells[1].RAT != domain.Tech5GNR || cells[1].RSRP != -70 {
		t.Fatalf("nr: %+v", cells[1])
	}
	sig := ApplyServingCell(domain.SignalInfo{}, cells)
	if sig.RSRP != -80 || sig.SINR != 6 {
		t.Fatalf("apply %+v", sig)
	}
}

func TestParseGTCAINFOComponents(t *testing.T) {
	resp := `+GTCAINFO:
PCC:5078,940,641760,450,2,1,1,3,19,-1,-80
SCC 3:2,0,103,216,1444,50,255,4,255,3,255,13,-9,-81
OK`
	ca := ParseGTCAINFOComponents(resp)
	if len(ca) != 2 {
		t.Fatalf("len=%d %+v", len(ca), ca)
	}
	if ca[0].Component != "PCC" || ca[0].RSRP != -80 || ca[0].ARFCN != "641760" || ca[0].Band != "n78" || ca[0].ULARFCN != "940" || !ca[0].ULActive {
		t.Fatalf("pcc %+v", ca[0])
	}
	if ca[1].Component != "SCC3" || ca[1].RSRP != -81 || ca[1].RSRQ != -9 || ca[1].Band != "B3" || ca[1].DLBW != "10 MHz" || ca[1].PCI != "216" || ca[1].ARFCN != "1444" {
		t.Fatalf("scc %+v", ca[1])
	}
}

func TestFormatBandAndBandwidth(t *testing.T) {
	if b := FormatBand("103", false); b != "B3" {
		t.Fatalf("expected B3, got %s", b)
	}
	if b := FormatBand("78", true); b != "n78" {
		t.Fatalf("expected n78, got %s", b)
	}
	if b := FormatBand("B20", false); b != "B20" {
		t.Fatalf("expected B20, got %s", b)
	}
	if b := DeriveBandFromARFCN(641760); b != "n78" {
		t.Fatalf("expected n78, got %s", b)
	}
	if b := DeriveBandFromARFCN(1444); b != "B3" {
		t.Fatalf("expected B3, got %s", b)
	}

	if bw := FormatBandwidth("100", false); bw != "20 MHz" {
		t.Fatalf("expected 20 MHz, got %s", bw)
	}
	if bw := FormatBandwidth("100", true); bw != "100 MHz" {
		t.Fatalf("expected 100 MHz, got %s", bw)
	}
	if bw := FormatBandwidth("50", false); bw != "10 MHz" {
		t.Fatalf("expected 10 MHz, got %s", bw)
	}
	if bw := FormatBandwidth("6", false); bw != "1.4 MHz" {
		t.Fatalf("expected 1.4 MHz, got %s", bw)
	}
	if bw := FormatBandwidth("20000", false); bw != "20 MHz" {
		t.Fatalf("expected 20 MHz, got %s", bw)
	}

	if m := FormatModulation("4"); m != "256QAM" {
		t.Fatalf("expected 256QAM, got %s", m)
	}
	if m := FormatModulation("1"); m != "QPSK" {
		t.Fatalf("expected QPSK, got %s", m)
	}
}

func TestCorrelateCAWithCells(t *testing.T) {
	ca := []domain.CAComponent{
		{Component: "PCC", PCI: "358"},
		{Component: "SCC1", PCI: "100", ARFCN: "640000"},
	}
	cells := []domain.CellInfo{
		{Serving: true, RAT: domain.TechLTE, PCI: "358", Band: "103", Bandwidth: "100", RSRP: -80, SINR: 10},
		{Serving: false, RAT: domain.Tech5GNR, PCI: "100", ARFCN: "640000", Band: "78", Bandwidth: "100"},
	}

	enriched := CorrelateCAWithCells(ca, cells)
	if len(enriched) != 2 {
		t.Fatalf("len=%d", len(enriched))
	}
	if enriched[0].Band != "B3" || enriched[0].DLBW != "20 MHz" || enriched[0].RSRP != -80 || enriched[0].SINR != 10 {
		t.Fatalf("enriched PCC: %+v", enriched[0])
	}
	if enriched[1].Band != "n78" || enriched[1].DLBW != "100 MHz" {
		t.Fatalf("enriched SCC1: %+v", enriched[1])
	}
}

func TestParseGTUSBMODE(t *testing.T) {
	if n := ParseGTUSBMODE("+GTUSBMODE: 41\r\nOK"); n != 41 {
		t.Fatalf("got %d", n)
	}
}
