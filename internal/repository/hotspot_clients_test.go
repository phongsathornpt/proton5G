package repository

import (
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestParseIWStationDump(t *testing.T) {
	sample := `Station aa:bb:cc:dd:ee:01 (on wlan0)
	inactive time:	120 ms
	rx bytes:	1000
Station 11:22:33:44:55:66 (on wlan0)
	inactive time:	10 ms
`
	got := ParseIWStationDump(sample)
	if len(got) != 2 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if got[0].MAC != "aa:bb:cc:dd:ee:01" || got[1].MAC != "11:22:33:44:55:66" {
		t.Fatalf("%+v", got)
	}
}

func TestParseDnsmasqLeases(t *testing.T) {
	sample := `1700000000 aa:bb:cc:dd:ee:01 192.168.50.10 phone *
1700000001 11:22:33:44:55:66 192.168.50.11 * *
`
	got := ParseDnsmasqLeases(sample)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].IP != "192.168.50.10" || got[0].Name != "phone" {
		t.Fatalf("%+v", got[0])
	}
	if got[1].Name != "" {
		t.Fatalf("star name should clear: %+v", got[1])
	}
}

func TestMergeHotspotClients(t *testing.T) {
	stations := []domain.HotspotClient{{MAC: "aa:bb:cc:dd:ee:01"}}
	leases := []domain.HotspotClient{
		{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.50.10", Name: "phone"},
		{MAC: "ff:ff:ff:ff:ff:ff", IP: "192.168.50.99", Name: "gone"},
	}
	got := MergeHotspotClients(stations, leases)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].IP != "192.168.50.10" || got[0].Name != "phone" {
		t.Fatalf("%+v", got[0])
	}
}
