package domain

import (
	"encoding/json"
	"testing"
)

func TestParseDataMode(t *testing.T) {
	for raw, want := range map[string]DataMode{
		"":      DataModeAuto,
		"AUTO":  DataModeAuto,
		"rndis": DataModeRNDIS,
		"net":   DataModeRNDIS,
		"MBIM":  DataModeMBIM,
	} {
		got, err := ParseDataMode(raw)
		if err != nil || got != want {
			t.Fatalf("ParseDataMode(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := ParseDataMode("bogus"); err == nil {
		t.Fatal("ParseDataMode(bogus) unexpectedly succeeded")
	}
}

func TestDataModeJSONCompatibility(t *testing.T) {
	raw, err := json.Marshal(DataConnectRequest{Mode: DataModeRNDIS, Method: DataMethodDHCP})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"mode":"rndis","iface":"","method":"dhcp"}` {
		t.Fatalf("unexpected JSON: %s", raw)
	}
}

func TestParseRegStatUnknown(t *testing.T) {
	if got := ParseRegStat(RegStatUnknown); got != RegUnknown {
		t.Fatalf("ParseRegStat(unknown) = %q, want %q", got, RegUnknown)
	}
	if got := ParseRegStatString("not-a-number"); got != RegUnknown {
		t.Fatalf("ParseRegStatString(invalid) = %q, want %q", got, RegUnknown)
	}
}
