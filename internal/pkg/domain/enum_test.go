package domain

import "testing"

func TestIsFM350AndFilter(t *testing.T) {
	if !IsFM350("0e8d", "7127") || !IsFM350("0E8D", "7126") {
		t.Fatal("known PIDs")
	}
	if IsFM350("0e8d", "7125") || IsFM350("8087", "7127") {
		t.Fatal("should reject")
	}
	if !MatchFM350Filter("0e8d", "7127", "0e8d", "7126") {
		t.Fatal("family 7127 filter should accept live 7126")
	}
	if !MatchFM350Filter("0e8d", "", "0e8d", "7126") {
		t.Fatal("empty product")
	}
	if MatchFM350Filter("0e8d", "abcd", "0e8d", "7126") {
		t.Fatal("custom PID is exact-only")
	}
}

func TestRATModePref(t *testing.T) {
	p, err := ParseRATModePref("5G")
	if err != nil || p != RATPref5G || p.GTACTCode() != GTACT5GOnly {
		t.Fatalf("got %v %v", p, err)
	}
	if _, err := ParseRATModePref("nsa"); err == nil {
		t.Fatal("expected error")
	}
	if ParseGTACTCode(GTACT5GOnly) != RATMode5GOnly {
		t.Fatal("gtact map")
	}
	if ParseGTACTCode(GTACTLTEOnly) != RATModeLTEOnly {
		t.Fatal("lte map")
	}
	if ParseGTACTCode(GTACTAuto) != RATModeAuto {
		t.Fatal("auto map")
	}
}

func TestRegState(t *testing.T) {
	if ParseRegStat(RegStatHome) != RegHome {
		t.Fatal()
	}
	if !RegHome.IsRegistered() || !RegRoaming.IsRegistered() {
		t.Fatal("registered")
	}
	if RegSearching.IsRegistered() {
		t.Fatal("searching not registered")
	}
}

func TestPDPType(t *testing.T) {
	p, err := ParsePDPType("ip")
	if err != nil || p != PDPIP {
		t.Fatalf("%v %v", p, err)
	}
	if _, err := ParsePDPType("PPP"); err == nil {
		t.Fatal("expected error")
	}
	if DefaultPDPType() != PDPIPV4V6 {
		t.Fatal()
	}
}

func TestRadioTechFromCellRAT(t *testing.T) {
	if RadioTechFromCellRAT("4") != TechLTE {
		t.Fatal("lte")
	}
	if RadioTechFromCellRAT("9") != Tech5GNR {
		t.Fatal("nr")
	}
	if RadioTechFromCellRAT("2") != TechUMTS {
		t.Fatal("umts")
	}
	if RadioTechFromCellRAT("x") != TechUnknown {
		t.Fatal("unknown")
	}
}

func TestSIMAndPower(t *testing.T) {
	if NormalizeSIMState("READY") != SIMReady {
		t.Fatal()
	}
	if NormalizeSIMState("") != SIMMissing {
		t.Fatal()
	}
	if NormalizePowerControl("on") != PowerOn {
		t.Fatal()
	}
}
