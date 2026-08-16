package domain

import "testing"

func TestValidateHotspotConfigOK(t *testing.T) {
	err := ValidateHotspotConfig(HotspotConfig{
		SSID:      "FM350-Hotspot",
		Password:  "password1",
		WlanIface: "wlan0",
		Channel:   6,
		Band:      HotspotBand24,
		LANCIDR:   "192.168.50.1/24",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateHotspotConfigErrors(t *testing.T) {
	base := HotspotConfig{
		SSID:      "ok",
		Password:  "password1",
		WlanIface: "wlan0",
		Band:      HotspotBand24,
		LANCIDR:   "192.168.50.1/24",
	}
	cases := []struct {
		name string
		mut  func(*HotspotConfig)
	}{
		{"empty ssid", func(c *HotspotConfig) { c.SSID = "" }},
		{"short pass", func(c *HotspotConfig) { c.Password = "short" }},
		{"bad iface", func(c *HotspotConfig) { c.WlanIface = "wlan;rm" }},
		{"bad cidr", func(c *HotspotConfig) { c.LANCIDR = "not-a-cidr" }},
		{"bad band", func(c *HotspotConfig) { c.Band = "6" }},
		{"channel 2.4", func(c *HotspotConfig) { c.Channel = 40 }},
	}
	for _, tc := range cases {
		c := base
		tc.mut(&c)
		if err := ValidateHotspotConfig(c); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestValidateIfaceName(t *testing.T) {
	if err := ValidateIfaceName("wlan0"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIfaceName("enx00aabb"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIfaceName(""); err == nil {
		t.Fatal("expected empty error")
	}
	if err := ValidateIfaceName("a/b"); err == nil {
		t.Fatal("expected slash error")
	}
}

func TestRedactedConfig(t *testing.T) {
	c := HotspotConfig{Password: "secretpass"}.RedactedConfig()
	if c.Password != "********" {
		t.Fatalf("got %q", c.Password)
	}
}

func TestValidateHotspotConfigRejectsHostapdInjection(t *testing.T) {
	base := HotspotConfig{
		SSID: "ok", Password: "password1", WlanIface: "wlan0",
		Band: HotspotBand24, Channel: 6, LANCIDR: "192.168.50.1/24",
	}
	for _, mutate := range []func(*HotspotConfig){
		func(c *HotspotConfig) { c.SSID = "safe\ncountry_code=ZZ" },
		func(c *HotspotConfig) { c.Password = "password1\nwpa=0" },
	} {
		cfg := base
		mutate(&cfg)
		if err := ValidateHotspotConfig(cfg); err == nil {
			t.Fatal("expected control-character rejection")
		}
	}
}

func TestValidateHotspotConfigSSIDUsesByteLimit(t *testing.T) {
	cfg := HotspotConfig{
		SSID: "กกกกกกกกกกก", Password: "password1", WlanIface: "wlan0",
		Band: HotspotBand24, Channel: 6, LANCIDR: "192.168.50.1/24",
	}
	if err := ValidateHotspotConfig(cfg); err == nil {
		t.Fatal("expected >32-byte UTF-8 SSID rejection")
	}
}

func TestValidateHotspotConfigRejectsUnusableLANGateway(t *testing.T) {
	base := HotspotConfig{
		SSID: "ok", Password: "password1", WlanIface: "wlan0",
		Band: HotspotBand24, Channel: 6,
	}
	for _, cidr := range []string{"192.168.50.0/24", "192.168.50.255/24", "192.168.50.1/31", "192.168.50.1/32"} {
		cfg := base
		cfg.LANCIDR = cidr
		if err := ValidateHotspotConfig(cfg); err == nil {
			t.Fatalf("expected invalid LAN CIDR %s", cidr)
		}
	}
}
