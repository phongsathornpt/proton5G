package template

import (
	"strings"
	"testing"
)

func TestRenderIndex_ShellAndPanels(t *testing.T) {
	html, err := RenderIndex()
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, want := range []string{
		"layout-base",
		"layout-topbar",
		"layout-sidebar",
		"layout-content",
		"layout-footer",
		`id="panel-overview"`,
		`id="panel-cellular"`,
		`id="panel-wan"`,
		`id="panel-lan"`,
		`id="panel-advanced"`,
		`id="footer-updated"`,
		`/assets/layout/layout.css`,
		`/assets/app.js`,
		`/assets/boot.js`,
		`id="modem-select"`,
		`id="hotspot-ssid"`,
		`id="sig-sinr"`,
		`id="cells-table"`,
		`id="ca-table"`,
		"UL Bandwidth",
		"DL Bandwidth",
		"Uplink",
		`id="pdp-ip"`,
		`id="wan-method-select"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in rendered index", want)
		}
	}
}

func TestAssetsEmbedded(t *testing.T) {
	for _, path := range []string{
		"assets/layout/layout.css",
		"assets/shared/components.css",
		"assets/app.js",
		"assets/features/overview/panel.html",
	} {
		if _, err := Assets.ReadFile(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
