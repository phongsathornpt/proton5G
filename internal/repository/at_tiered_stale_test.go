package repository

import (
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestRefreshMediumClearsStalePDPStateOnSuccessfulEmptyReplies(t *testing.T) {
	c := &TieredClient{Client: NewClient("/dev/test")}
	c.sendFn = func(cmd string) (string, error) {
		switch cmd {
		case CmdCGPADDR(1):
			return "+CGPADDR: 1,\"0.0.0.0\"\r\nOK", nil
		case CmdGTDNS(1):
			return "+GTDNS: 1,\"0.0.0.0\",\"0.0.0.0\"\r\nOK", nil
		default:
			return "OK", nil
		}
	}
	cache := tieredPollCache{
		apn: domain.APNConfig{CID: 1},
		pdp: domain.PDPSession{
			CID:     1,
			IP:      "10.0.0.2",
			Gateway: "10.0.0.1",
			DNS1:    "8.8.8.8",
			DNS2:    "1.1.1.1",
		},
	}

	c.refreshMedium(&cache)
	if cache.pdp.IP != "" || cache.pdp.Gateway != "" || cache.pdp.DNS1 != "" || cache.pdp.DNS2 != "" {
		t.Fatalf("stale PDP state survived successful empty reply: %+v", cache.pdp)
	}
}
