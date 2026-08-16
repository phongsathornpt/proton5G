package repository

import (
	"fmt"
	"strconv"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

// FormatMIMO normalizes the active MIMO layer count reported by GTCAINFO.
// 255/0/-1 are vendor sentinels for unavailable/unused values.
func FormatMIMO(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" || raw == "255" || raw == "-1" || strings.EqualFold(raw, "N/A") {
		return ""
	}
	if strings.Contains(strings.ToLower(raw), "x") {
		return strings.ToLower(raw)
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 16 {
		return raw
	}
	return fmt.Sprintf("%dx%d", n, n)
}

// EnrichGTCAINFOComponents corrects and enriches CA fields from the raw Fibocom
// AT+GTCAINFO response. Some FM350 firmware uses a long SCC row with explicit
// MIMO fields before modulation fields; treating those positions as modulation
// makes a 4-layer MIMO value look like 256QAM.
func EnrichGTCAINFOComponents(ca []domain.CAComponent, response string) []domain.CAComponent {
	if len(ca) == 0 || strings.TrimSpace(response) == "" {
		return ca
	}

	byComponent := make(map[string][]string)
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		head := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(line[:i]), " ", ""))
		if !strings.HasPrefix(head, "SCC") {
			continue
		}
		byComponent[head] = splitCSV(line[i+1:])
	}

	for i := range ca {
		c := &ca[i]
		parts := byComponent[strings.ToUpper(strings.ReplaceAll(c.Component, " ", ""))]
		if len(parts) < 9 {
			continue
		}

		p0 := atoiOr(parts[0], -1)
		p4 := atoiOr(parts[4], 0)

		if len(parts) >= 12 && p4 > 100 {
			// Long FM350 row, example:
			// SCC 3:2,0,103,216,1444,50,255,4,255,3,255,13,-9,-81
			//              band pci arfcn bw  ulbw dlM ulM dlQ ulQ
			c.DLBW = FormatBandwidth(parts[5], false)
			c.ULBW = FormatBandwidth(parts[6], false)
			c.DLMIMO = FormatMIMO(parts[7])
			c.ULMIMO = FormatMIMO(parts[8])
			c.DLMod = FormatModulation(parts[9])
			c.ULMod = FormatModulation(parts[10])
			continue
		}

		if p0 == 0 || p0 == 1 {
			// Compact documented row:
			// <upload>,<pci>,<arfcn>,<dl_bw>,<ul_bw>,<dl_mimo>,<ul_mimo>,<dl_mod>,<ul_mod>
			c.DLMIMO = FormatMIMO(parts[5])
			c.ULMIMO = FormatMIMO(parts[6])
			c.DLMod = FormatModulation(parts[7])
			c.ULMod = FormatModulation(parts[8])
		}
	}
	return ca
}
