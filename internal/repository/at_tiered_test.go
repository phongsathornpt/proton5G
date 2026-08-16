package repository

import (
	"reflect"
	"testing"
	"time"
)

func TestTieredClientPollCadence(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var calls []string
	c := &TieredClient{
		Client:   NewClient("/dev/test"),
		nowFn:    func() time.Time { return now },
		ensureFn: func() error { return nil },
	}
	c.sendFn = func(cmd string) (string, error) {
		calls = append(calls, cmd)
		switch cmd {
		case CmdCSQ:
			return "+CSQ: 20,99\r\nOK", nil
		case CmdCESQ:
			return "+CESQ: 99,99,255,255,20,50\r\nOK", nil
		case CmdC5GREG:
			return "+C5GREG: 0,0\r\nOK", nil
		case CmdCEREG:
			return "+CEREG: 0,1\r\nOK", nil
		case CmdGTCAINFO, CmdGTCCINFO:
			return "OK", nil
		case CmdCOPS:
			return "+COPS: 0,0,\"AIS\",7\r\nOK", nil
		case CmdGTSENRDTEMP:
			return "+GTSENRDTEMP: 1,45000\r\nOK", nil
		case CmdCGPADDR(1):
			return "+CGPADDR: 1,\"10.0.0.2\"\r\nOK", nil
		case CmdGTDNS(1):
			return "+GTDNS: 1,\"8.8.8.8\",\"1.1.1.1\"\r\nOK", nil
		case CmdCPIN:
			return "+CPIN: READY\r\nOK", nil
		case CmdCIMI:
			return "520001234567890\r\nOK", nil
		case CmdCCID:
			return "+CCID: 8966001234567890123\r\nOK", nil
		case CmdCGDCONTQ:
			return "+CGDCONT: 1,\"IPV4V6\",\"internet\"\r\nOK", nil
		case CmdGTACTQ:
			return "+GTACT: 20\r\nOK", nil
		case CmdCGMI:
			return "Fibocom\r\nOK", nil
		case CmdCGMM:
			return "FM350-GL\r\nOK", nil
		case CmdCGMR:
			return "1.0\r\nOK", nil
		case CmdCGSN:
			return "867530900000001\r\nOK", nil
		default:
			return "OK", nil
		}
	}

	first, err := c.GetFullStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 19 {
		t.Fatalf("first poll commands=%d want 19: %v", len(calls), calls)
	}
	if first.Network.Operator != "AIS" || first.TempC != 45 || first.SIM.IMSI == "" || first.APN.APN != "internet" {
		t.Fatalf("first poll cache fields not populated: %+v", first)
	}

	calls = nil
	now = now.Add(2 * time.Second)
	second, err := c.GetFullStatus()
	if err != nil {
		t.Fatal(err)
	}
	wantFast := []string{CmdCSQ, CmdCESQ, CmdC5GREG, CmdCEREG, CmdGTCAINFO, CmdGTCCINFO}
	if !reflect.DeepEqual(calls, wantFast) {
		t.Fatalf("steady poll commands=%v want %v", calls, wantFast)
	}
	if second.Network.Operator != "AIS" || second.TempC != 45 || second.SIM.ICCID == "" || second.Identity.Model != "FM350-GL" {
		t.Fatalf("steady poll lost cached fields: %+v", second)
	}

	calls = nil
	now = now.Add(9 * time.Second) // 11s since initial sample: medium tier is due.
	if _, err := c.GetFullStatus(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 10 {
		t.Fatalf("medium poll commands=%d want 10: %v", len(calls), calls)
	}
	for _, slow := range []string{CmdCPIN, CmdCIMI, CmdCCID, CmdCGDCONTQ, CmdGTACTQ, CmdCGMI, CmdCGMM, CmdCGMR, CmdCGSN} {
		if containsString(calls, slow) {
			t.Fatalf("slow command %q unexpectedly ran in medium tier: %v", slow, calls)
		}
	}

	calls = nil
	now = time.Unix(1_700_000_000, 0).Add(61 * time.Second)
	if _, err := c.GetFullStatus(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 19 {
		t.Fatalf("slow poll commands=%d want 19: %v", len(calls), calls)
	}
}

func TestTieredClientPortChangeClearsCache(t *testing.T) {
	c := NewTieredClient("/dev/ttyUSB2")
	c.cache = tieredPollCache{operator: "AIS", mediumAt: time.Now(), slowAt: time.Now()}
	c.SetPortName("/dev/ttyUSB3")
	got := c.snapshotCache()
	if got.operator != "" || !got.mediumAt.IsZero() || !got.slowAt.IsZero() {
		t.Fatalf("cache not reset after port change: %+v", got)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
