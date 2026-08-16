package usecase

import (
	"errors"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

type fakeATWithIPConfig struct {
	fakeAT
	cfg    domain.WANIPConfig
	cfgErr error
}

func (f *fakeATWithIPConfig) QueryPDPIPConfig(int) (domain.WANIPConfig, error) {
	return f.cfg, f.cfgErr
}

type fakeNetWithDNS struct {
	fakeNet
	dnsIface string
	dns      []string
}

func (f *fakeNetWithDNS) ConfigureDNS(iface string, servers []string) (string, error) {
	f.dnsIface = iface
	f.dns = append([]string(nil), servers...)
	return "dns ok", nil
}

func TestRNDISAutoUsesModemReportedConfigBeforeDHCP(t *testing.T) {
	netRepo := &fakeNetWithDNS{}
	at := &fakeATWithIPConfig{
		fakeAT: fakeAT{port: "/dev/ttyUSB0", pdp: domain.PDPSession{CID: 1, IP: "10.64.37.28"}},
		cfg: domain.WANIPConfig{
			AddressCIDR: "10.64.37.28/29",
			Gateway:     "10.64.37.25",
			DNS:         []string{"8.8.8.8", "1.1.1.1"},
		},
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  at,
		Net: netRepo,
	})
	out, err := svc.DataConnect(domain.DataConnectRequest{Mode: domain.DataModeRNDIS, Iface: "enxabc"})
	if err != nil {
		t.Fatalf("connect: %v\n%s", err, out)
	}
	if netRepo.lastConnect != "" {
		t.Fatalf("DHCP must not run when modem config is usable: %q", netRepo.lastConnect)
	}
	if netRepo.lastStaticAddr != "10.64.37.28/29" || netRepo.lastStaticGW != "10.64.37.25" {
		t.Fatalf("static addr=%q gw=%q", netRepo.lastStaticAddr, netRepo.lastStaticGW)
	}
	if netRepo.dnsIface != "enxabc" || len(netRepo.dns) != 2 {
		t.Fatalf("dns iface=%q servers=%v", netRepo.dnsIface, netRepo.dns)
	}
	st := svc.CachedStatus()
	if st.WAN.Method != domain.WANMethodStatic || st.PDP.Gateway != "10.64.37.25" {
		t.Fatalf("status WAN=%+v PDP=%+v", st.WAN, st.PDP)
	}
}

func TestRNDISStaticRequiresModemReportedConfig(t *testing.T) {
	netRepo := &fakeNet{}
	at := &fakeATWithIPConfig{
		fakeAT: fakeAT{port: "/dev/ttyUSB0", pdp: domain.PDPSession{IP: "10.1.2.3"}},
		cfgErr: errors.New("no CGCONTRDP data"),
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  at,
		Net: netRepo,
	})
	_, err := svc.DataConnect(domain.DataConnectRequest{
		Mode: domain.DataModeRNDIS, Iface: "enx1", Method: domain.DataMethodStatic,
	})
	if err == nil {
		t.Fatal("expected static mode to require modem-reported prefix/gateway")
	}
	if netRepo.lastStatic != "" {
		t.Fatalf("must not invent config from CGPADDR, got %q %q", netRepo.lastStaticAddr, netRepo.lastStaticGW)
	}
}

func TestRNDISAutoFallsBackToDHCPWhenConfigUnavailable(t *testing.T) {
	netRepo := &fakeNet{connectOut: "dhcp ok"}
	at := &fakeATWithIPConfig{
		fakeAT: fakeAT{port: "/dev/ttyUSB0"},
		cfgErr: errors.New("CGCONTRDP unsupported"),
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  at,
		Net: netRepo,
	})
	if _, err := svc.DataConnect(domain.DataConnectRequest{Mode: domain.DataModeRNDIS, Iface: "enx1"}); err != nil {
		t.Fatal(err)
	}
	if netRepo.lastConnect != "enx1" || netRepo.lastStatic != "" {
		t.Fatalf("dhcp=%q static=%q", netRepo.lastConnect, netRepo.lastStatic)
	}
	if svc.CachedStatus().WAN.Method != domain.WANMethodDHCP {
		t.Fatalf("method=%q", svc.CachedStatus().WAN.Method)
	}
}
