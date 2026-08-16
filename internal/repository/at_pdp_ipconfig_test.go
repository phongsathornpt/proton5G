package repository

import "testing"

func TestParseCGCONTRDPUsesReportedMaskAndGateway(t *testing.T) {
	resp := "+CGCONTRDP: 1,5,\"internet\",\"10.64.37.28.255.255.255.248\",\"10.64.37.25\",\"8.8.8.8\",\"1.1.1.1\",\"\",\"\",0,0,1428\r\nOK"
	cfg, err := ParseCGCONTRDP(resp, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AddressCIDR != "10.64.37.28/29" || cfg.Gateway != "10.64.37.25" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if len(cfg.DNS) != 2 || cfg.DNS[0] != "8.8.8.8" || cfg.DNS[1] != "1.1.1.1" {
		t.Fatalf("dns=%v", cfg.DNS)
	}
	if cfg.MTU != 1428 {
		t.Fatalf("mtu=%d", cfg.MTU)
	}
}

func TestParseCGCONTRDPRejectsAddressWithoutMask(t *testing.T) {
	resp := "+CGCONTRDP: 1,5,\"internet\",\"10.64.37.28\",\"10.64.37.25\"\r\nOK"
	if _, err := ParseCGCONTRDP(resp, 1); err == nil {
		t.Fatal("expected missing-mask error")
	}
}
