package repository

import "testing"

func TestParseMBIMIPConfig(t *testing.T) {
	raw := `[/dev/cdc-wdm0] IPv4 configuration available: 'address, gateway, dns, mtu'
             IP [0]: '10.51.226.4/29'
            Gateway: '10.51.226.5'
            DNS [0]: '8.8.8.8'
            DNS [1]: '1.1.1.1'
                MTU: '1500'`

	cfg, err := ParseMBIMIPConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AddressCIDR != "10.51.226.4/29" || cfg.Gateway != "10.51.226.5" {
		t.Fatalf("unexpected address config: %+v", cfg)
	}
	if len(cfg.DNS) != 2 || cfg.DNS[0] != "8.8.8.8" || cfg.DNS[1] != "1.1.1.1" {
		t.Fatalf("unexpected DNS: %+v", cfg.DNS)
	}
	if cfg.MTU != 1500 {
		t.Fatalf("unexpected MTU: %d", cfg.MTU)
	}
}

func TestParseMBIMIPConfigRequiresPrefix(t *testing.T) {
	_, err := ParseMBIMIPConfig("IPv4 address: '10.0.0.2'\nGateway: '10.0.0.1'")
	if err == nil {
		t.Fatal("expected missing-prefix error")
	}
}
