package usecase

import (
	"errors"
	"strings"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

type wanFakeMBIM struct {
	fakeMBIM
	cfg      domain.WANIPConfig
	queryErr error
}

func (f *wanFakeMBIM) QueryIPConfig(string) (domain.WANIPConfig, error) {
	return f.cfg, f.queryErr
}

type wanFakeNet struct {
	fakeNet
	configuredIface string
	configuredCIDR  string
	configuredGW    string
	dnsIface        string
	dns             []string
	clearedIface    string
	configureErr    error
	clearErr        error
}

func (f *wanFakeNet) ConfigureStatic(iface, cidr, gateway string) (string, error) {
	f.configuredIface = iface
	f.configuredCIDR = cidr
	f.configuredGW = gateway
	return "ip configured", f.configureErr
}

func (f *wanFakeNet) ConfigureDNS(iface string, servers []string) (string, error) {
	f.dnsIface = iface
	f.dns = append([]string(nil), servers...)
	return "dns configured", nil
}

func (f *wanFakeNet) ClearInterface(iface string) (string, error) {
	f.clearedIface = iface
	return "cleared", f.clearErr
}

func wanTestInventory() *fakeInventory {
	return &fakeInventory{modems: []domain.ModemDevice{{
		ID:       "usb:fm350",
		DataMode: domain.DataModeMBIM,
		ATPorts: []domain.ModemInterface{{
			Path: "/dev/ttyUSB0", Kind: domain.IfaceKindAT, ATReady: true,
		}},
		MBIMNodes: []domain.ModemInterface{{
			Path: "/dev/cdc-wdm0", Kind: domain.IfaceKindMBIM,
		}},
		NetIfaces: []domain.ModemInterface{{
			Path: "wwan0", Kind: domain.IfaceKindNet,
		}},
	}}}
}

func TestWANServiceAutoPrefersMBIMAndConfiguresHost(t *testing.T) {
	mbim := &wanFakeMBIM{cfg: domain.WANIPConfig{
		AddressCIDR: "10.51.226.4/29",
		Gateway:     "10.51.226.5",
		DNS:         []string{"8.8.8.8", "1.1.1.1"},
	}}
	netRepo := &wanFakeNet{}
	svc := NewModemService(ModemServiceConfig{
		USB:       &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:        &fakeAT{port: "/dev/ttyUSB0"},
		MBIM:      mbim,
		Net:       netRepo,
		Inventory: wanTestInventory(),
	})
	wan := NewWANService(svc)

	if _, err := wan.DataConnect(domain.DataConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	if mbim.lastConnectDev != "/dev/cdc-wdm0" {
		t.Fatalf("auto did not choose MBIM: %q", mbim.lastConnectDev)
	}
	if netRepo.lastConnect != "" {
		t.Fatalf("MBIM host iface was incorrectly sent through RNDIS: %q", netRepo.lastConnect)
	}
	if netRepo.configuredIface != "wwan0" || netRepo.configuredCIDR != "10.51.226.4/29" || netRepo.configuredGW != "10.51.226.5" {
		t.Fatalf("host config mismatch: iface=%q cidr=%q gw=%q", netRepo.configuredIface, netRepo.configuredCIDR, netRepo.configuredGW)
	}
	if len(netRepo.dns) != 2 || netRepo.dnsIface != "wwan0" {
		t.Fatalf("DNS not configured: iface=%q dns=%v", netRepo.dnsIface, netRepo.dns)
	}
	st := wan.CachedStatus()
	if st.WAN.Session != domain.WANSessionConnected || st.WAN.Iface != "wwan0" {
		t.Fatalf("WAN state not connected: %+v", st.WAN)
	}
	if st.PDP.IP != "10.51.226.4" || st.PDP.Gateway != "10.51.226.5" || st.PDP.DNS1 != "8.8.8.8" {
		t.Fatalf("PDP state not sourced from MBIM config: %+v", st.PDP)
	}
}

func TestWANServiceMBIMSetupFailureRollsBackBearer(t *testing.T) {
	mbim := &wanFakeMBIM{cfg: domain.WANIPConfig{AddressCIDR: "10.0.0.2/30", Gateway: "10.0.0.1"}}
	netRepo := &wanFakeNet{configureErr: errors.New("route failed")}
	svc := NewModemService(ModemServiceConfig{
		USB:       &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:        &fakeAT{port: "/dev/ttyUSB0"},
		MBIM:      mbim,
		Net:       netRepo,
		Inventory: wanTestInventory(),
	})
	_, err := NewWANService(svc).DataConnect(domain.DataConnectRequest{})
	if err == nil {
		t.Fatal("expected host configuration failure")
	}
	if mbim.lastDisconnect != "/dev/cdc-wdm0" {
		t.Fatalf("expected bearer rollback, disconnect=%q", mbim.lastDisconnect)
	}
}

func TestWANServiceSelectRejectsForeignNetInterface(t *testing.T) {
	inv := wanTestInventory()
	inv.modems = append(inv.modems, domain.ModemDevice{
		ID:        "usb:other",
		NetIfaces: []domain.ModemInterface{{Path: "eth-other", Kind: domain.IfaceKindNet}},
	})
	svc := NewModemService(ModemServiceConfig{
		USB:       &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:        &fakeAT{port: "/dev/ttyUSB0"},
		Inventory: inv,
	})
	_, err := NewWANService(svc).SelectModem(domain.ModemSelectRequest{
		ModemID: "usb:fm350", NetIface: "eth-other",
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected endpoint ownership error, got %v", err)
	}
}

func TestWANServiceMBIMDisconnectCleansHost(t *testing.T) {
	mbim := &wanFakeMBIM{cfg: domain.WANIPConfig{AddressCIDR: "10.0.0.2/30", Gateway: "10.0.0.1"}}
	netRepo := &wanFakeNet{}
	svc := NewModemService(ModemServiceConfig{
		USB:       &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:        &fakeAT{port: "/dev/ttyUSB0"},
		MBIM:      mbim,
		Net:       netRepo,
		Inventory: wanTestInventory(),
	})
	wan := NewWANService(svc)
	if _, err := wan.DataConnect(domain.DataConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := wan.DataDisconnect(domain.DataConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	if netRepo.clearedIface != "wwan0" {
		t.Fatalf("host interface not cleaned: %q", netRepo.clearedIface)
	}
	if st := wan.CachedStatus(); st.WAN.Session != domain.WANSessionDisconnected || st.PDP.IP != "" {
		t.Fatalf("disconnect state is stale: WAN=%+v PDP=%+v", st.WAN, st.PDP)
	}
}
