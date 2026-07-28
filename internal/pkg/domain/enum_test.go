package domain

import "testing"

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
