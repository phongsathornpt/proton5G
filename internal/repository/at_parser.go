package repository

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

var digitsOnly = regexp.MustCompile(`^\d{5,}$`)

func ParseCSQ(response string) domain.SignalInfo {
	info := domain.SignalInfo{RawCSQ: response}
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CSQ:") {
			parts := strings.Split(strings.TrimPrefix(line, "+CSQ:"), ",")
			if len(parts) >= 1 {
				rssiRaw, err := strconv.Atoi(strings.TrimSpace(parts[0]))
				if err == nil && rssiRaw != CSQUnknown {
					// 0: CSQMinDBm, CSQMaxRaw: -51 dBm
					info.RSSI = CSQMinDBm + (rssiRaw * 2)
					info.Percentage = int((float64(rssiRaw) / float64(CSQMaxRaw)) * 100.0)
					if info.Percentage > 100 {
						info.Percentage = 100
					}
				}
			}
		}
	}
	return info
}

// ParseCESQ parses 3GPP +CESQ extended signal quality.
// Format: +CESQ: <rxlev>,<ber>,<rscp>,<ecno>,<rsrq>,<rsrp>
func ParseCESQ(response string) (rsrp, rsrq int, ok bool) {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CESQ:") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(line, "+CESQ:"), ",")
		if len(parts) < 6 {
			return 0, 0, false
		}
		rsrqRaw, err1 := strconv.Atoi(strings.TrimSpace(parts[4]))
		rsrpRaw, err2 := strconv.Atoi(strings.TrimSpace(parts[5]))
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		if rsrqRaw != CESQUnknown && rsrqRaw >= 0 && rsrqRaw <= RSRQMaxRaw {
			tenths := -195 + 5*rsrqRaw
			if tenths >= 0 {
				rsrq = (tenths + 5) / 10
			} else {
				rsrq = (tenths - 5) / 10
			}
			ok = true
		}
		if rsrpRaw != CESQUnknown && rsrpRaw >= 0 && rsrpRaw <= RSRPMaxRaw {
			rsrp = RSRPMinDBm + rsrpRaw
			ok = true
		}
		return rsrp, rsrq, ok
	}
	return 0, 0, false
}

// MergeExtendedSignal fills RSRP/RSRQ from CESQ, then Fibocom proprietary
// AT+GTCAINFO? / AT+GTCCINFO? when CESQ is missing or unknown.
func MergeExtendedSignal(base domain.SignalInfo, cesqResp string, proprietary ...string) domain.SignalInfo {
	if rsrp, rsrq, ok := ParseCESQ(cesqResp); ok {
		base = applyRSRPRSRQ(base, rsrp, rsrq)
	}
	if base.RSRP != 0 {
		return base
	}
	for _, raw := range proprietary {
		if raw == "" {
			continue
		}
		if rsrp, rsrq, ok := ParseGTCAINFO(raw); ok {
			base = applyRSRPRSRQ(base, rsrp, rsrq)
			if base.RSRP != 0 {
				return base
			}
		}
		if rsrp, rsrq, ok := ParseGTCCINFO(raw); ok {
			base = applyRSRPRSRQ(base, rsrp, rsrq)
			if base.RSRP != 0 {
				return base
			}
		}
	}
	return base
}

func applyRSRPRSRQ(base domain.SignalInfo, rsrp, rsrq int) domain.SignalInfo {
	if rsrp != 0 {
		base.RSRP = rsrp
		pct := int(float64(rsrp-(RSRPMinDBm)) / float64(-44-(RSRPMinDBm)) * 100.0)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		if base.Percentage == 0 {
			base.Percentage = pct
		}
		if base.RSSI == 0 {
			base.RSSI = rsrp
		}
	}
	if rsrq != 0 {
		base.RSRQ = rsrq
	}
	return base
}

// ParseGTCAINFO extracts RSRP/RSRQ from Fibocom AT+GTCAINFO? output.
// Examples:
//
//	PCC:5078,940,641760,450,2,1,1,3,19,-1,-80
//	SCC 3:…,13,-9,-81
//
// Trailing values in typical RSRP (-140..-44) / RSRQ (-20..0) ranges are used.
func ParseGTCAINFO(response string) (rsrp, rsrq int, ok bool) {
	bestRSRP := 0
	bestRSRQ := 0
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if !strings.Contains(upper, "PCC:") && !strings.Contains(upper, "SCC") {
			// also accept bare +GTCAINFO data lines without prefix after strip
			if !strings.Contains(line, ",") {
				continue
			}
		}
		// Normalize "PCC:a,b" → fields
		if i := strings.Index(line, ":"); i >= 0 && (strings.HasPrefix(upper, "PCC") || strings.HasPrefix(upper, "SCC") || strings.HasPrefix(upper, "+GTCAINFO")) {
			line = line[i+1:]
		}
		nums := parseSignedInts(line)
		if len(nums) == 0 {
			continue
		}
		// Prefer trailing candidates in valid ranges.
		lineRSRP, lineRSRQ := 0, 0
		for i := len(nums) - 1; i >= 0; i-- {
			n := nums[i]
			if lineRSRP == 0 && n >= -140 && n <= -44 {
				lineRSRP = n
				continue
			}
			if lineRSRQ == 0 && n >= -20 && n <= 0 && n != -1 {
				lineRSRQ = n
			}
		}
		if lineRSRP != 0 && (bestRSRP == 0 || lineRSRP > bestRSRP) {
			bestRSRP = lineRSRP
			if lineRSRQ != 0 {
				bestRSRQ = lineRSRQ
			}
		}
	}
	if bestRSRP == 0 {
		return 0, 0, false
	}
	return bestRSRP, bestRSRQ, true
}

// ParseGTCCINFO best-effort extracts RSRP from AT+GTCCINFO? cell lines.
// Example: 1,4,262,1,05D5,0019BF801,1300,358,103,100,13,60,60,22
// Trailing decimal fields in 0..97 are treated as 3GPP CESQ-style RSRP raw.
func ParseGTCCINFO(response string) (rsrp, rsrq int, ok bool) {
	best := 0
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR {
			continue
		}
		if strings.HasPrefix(line, "+GTCCINFO") {
			continue
		}
		// Skip hex-heavy tokens; only pure decimal fields at the end matter.
		parts := strings.Split(line, ",")
		var dec []int
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// pure decimal (optional leading minus)
			n, err := strconv.Atoi(p)
			if err != nil {
				dec = nil // reset run of trailing decimals after non-decimal
				continue
			}
			dec = append(dec, n)
		}
		// Among trailing decimals in CESQ RSRP raw range, pick strongest (max raw).
		// Example ends with …,13,60,60,22 → prefer 60 (-80 dBm) over 22.
		rawBest := -1
		for i := len(dec) - 1; i >= 0 && i >= len(dec)-4; i-- {
			raw := dec[i]
			if raw >= 0 && raw <= RSRPMaxRaw && raw != CESQUnknown {
				if raw > rawBest {
					rawBest = raw
				}
			}
		}
		if rawBest >= 0 {
			v := RSRPMinDBm + rawBest
			if best == 0 || v > best {
				best = v
			}
		}
	}
	if best == 0 {
		return 0, 0, false
	}
	return best, 0, true
}

func parseSignedInts(s string) []int {
	var out []int
	cur := ""
	flush := func() {
		if cur == "" || cur == "-" {
			cur = ""
			return
		}
		n, err := strconv.Atoi(cur)
		if err == nil {
			out = append(out, n)
		}
		cur = ""
	}
	for _, r := range s {
		if r == '-' || (r >= '0' && r <= '9') {
			if r == '-' && cur != "" {
				flush()
			}
			cur += string(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// ParseGTUSBMODE parses +GTUSBMODE: <n>
func ParseGTUSBMODE(response string) int {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+GTUSBMODE:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, "+GTUSBMODE:"))
		if i := strings.IndexAny(val, ",; "); i > 0 {
			val = val[:i]
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err == nil {
			return n
		}
	}
	return 0
}

const unknownOperator = "Unknown Operator"

// ParseCOPSFull extracts operator name and optional 3GPP AcT (access technology) code.
// Format: +COPS: <mode>,<format>,"<oper>",<act>
// AcT codes: 13/10 = E-UTRA-NR Dual Connectivity (EN-DC / 5G NSA), 11 = 5G SA, 7 = LTE, 2 = UMTS.
func ParseCOPSFull(response string) (oper string, act int, hasAct bool) {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+COPS:") {
			parts := strings.Split(line, ",")
			if len(parts) >= 3 {
				oper = strings.Trim(parts[2], "\" ")
			}
			if len(parts) >= 4 {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[3])); err == nil {
					act = n
					hasAct = true
				}
			}
			if oper != "" {
				return oper, act, hasAct
			}
		}
	}
	return unknownOperator, 0, false
}

func ParseRegistration(response string) domain.RegState {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valParts := strings.Split(parts[1], ",")
				stat := ""
				if len(valParts) >= 2 {
					stat = strings.TrimSpace(valParts[1])
				} else if len(valParts) == 1 {
					stat = strings.TrimSpace(valParts[0])
				}
				if stat != "" {
					return domain.ParseRegStatString(stat)
				}
			}
		}
	}
	return domain.RegNotRegistered
}

func ParseCPIN(response string) domain.SIMState {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CPIN:") {
			return domain.NormalizeSIMState(strings.TrimSpace(strings.TrimPrefix(line, "+CPIN:")))
		}
	}
	return domain.SIMMissing
}

// ParseCIMI extracts IMSI digits from AT+CIMI response.
func ParseCIMI(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR || strings.HasPrefix(line, "AT") {
			continue
		}
		line = strings.Trim(line, "\"")
		if strings.HasPrefix(strings.ToUpper(line), "+CIMI:") {
			line = strings.TrimSpace(line[6:])
			line = strings.Trim(line, "\"")
		}
		if digitsOnly.MatchString(line) {
			return line
		}
	}
	return ""
}

// ParseICCID extracts ICCID from AT+CCID / AT+ICCID / AT+QCCID style responses.
func ParseICCID(response string) string {
	prefixes := []string{"+CCID:", "+ICCID:", "+QCCID:"}
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR {
			continue
		}
		upper := strings.ToUpper(line)
		for _, p := range prefixes {
			if strings.HasPrefix(upper, p) {
				val := strings.TrimSpace(line[len(p):])
				val = strings.Trim(val, "\" ")
				if i := strings.IndexAny(val, ",; "); i > 0 {
					val = val[:i]
				}
				val = strings.TrimSpace(val)
				if len(val) >= 10 {
					return val
				}
			}
		}
		clean := strings.Trim(line, "\"")
		if digitsOnly.MatchString(clean) && len(clean) >= 15 {
			return clean
		}
	}
	return ""
}

func ParseCGDCONT(response string) domain.APNConfig {
	config := domain.APNConfig{CID: 1, PDPType: domain.DefaultPDPType(), APN: ""}
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CGDCONT:") {
			parts := strings.Split(strings.TrimPrefix(line, "+CGDCONT:"), ",")
			if len(parts) >= 3 {
				cid, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				pdp := domain.NormalizePDPType(strings.Trim(parts[1], "\" "))
				apn := strings.Trim(parts[2], "\" ")
				if cid == 1 || config.APN == "" {
					config.CID = cid
					config.PDPType = pdp
					config.APN = apn
				}
			}
		}
	}
	return config
}

// ParseGTSENRDTEMP parses AT+GTSENRDTEMP=1.
// Typical: +GTSENRDTEMP: 1,56736  (millidegrees C). Values under 200 are treated as °C already.
func ParseGTSENRDTEMP(response string) (float64, bool) {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+GTSENRDTEMP:") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "+GTSENRDTEMP:"))
		parts := strings.Split(rest, ",")
		raw := strings.TrimSpace(parts[len(parts)-1])
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil || n == 255 {
			return 0, false
		}
		if n > 200 {
			return n / 1000.0, true
		}
		if n == 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// ParseCGPADDR extracts the first non-zero IPv4 from AT+CGPADDR.
func ParseCGPADDR(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CGPADDR:") {
			continue
		}
		// +CGPADDR: <cid>,"a.b.c.d"[,"v6"]
		for _, tok := range quotedOrBareIPs(line) {
			if isIPv4Addr(tok) && tok != "0.0.0.0" {
				return tok
			}
		}
	}
	return ""
}

// ParseGTDNS extracts DNS1/DNS2 from +GTDNS: …
func ParseGTDNS(response string) (dns1, dns2 string) {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+GTDNS:") {
			continue
		}
		ips := quotedOrBareIPs(line)
		if len(ips) >= 1 && isIPv4Addr(ips[0]) {
			dns1 = ips[0]
		}
		if len(ips) >= 2 && isIPv4Addr(ips[1]) {
			dns2 = ips[1]
		}
		return dns1, dns2
	}
	return "", ""
}

func quotedOrBareIPs(line string) []string {
	var out []string
	rest := line
	if i := strings.Index(rest, ":"); i >= 0 {
		rest = rest[i+1:]
	}
	for _, p := range strings.Split(rest, ",") {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"")
		if p == "" || p == "0.0.0.0" {
			continue
		}
		// skip cid index
		if _, err := strconv.Atoi(p); err == nil && !strings.Contains(p, ".") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func isIPv4Addr(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// ParseInfoLine returns the first payload line from CGMI/CGMM/CGMR/CGSN-style replies.
func ParseInfoLine(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR {
			continue
		}
		if strings.HasPrefix(line, "AT") || strings.HasPrefix(line, "+CME") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			if i := strings.Index(line, ":"); i >= 0 {
				line = strings.TrimSpace(line[i+1:])
				line = strings.Trim(line, "\"")
			}
		}
		if line != "" && line != ATResultOK {
			return line
		}
	}
	return ""
}

// ParseGTCCINFOCells parses serving/neighbor rows from AT+GTCCINFO?.
// Fields: <svc>,<rat>,<mcc>,<mnc>,<tac>,<cell_id>,<arfcn>,<pci>,<band>,<bw>,<sinr>,<rsrp>,<rsrq>
func ParseGTCCINFOCells(response string) []domain.CellInfo {
	var cells []domain.CellInfo
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR {
			continue
		}
		if strings.HasPrefix(line, "+GTCCINFO") {
			continue
		}
		if !strings.Contains(line, ",") {
			continue
		}
		parts := splitCSV(line)
		if len(parts) < 11 {
			continue
		}
		cell := domain.CellInfo{
			Serving:   parts[0] == "1",
			RAT:       domain.RadioTechFromCellRAT(parts[1]),
			MCC:       dashEmpty(parts[2]),
			MNC:       dashEmpty(parts[3]),
			TAC:       hexOrEmpty(parts[4]),
			CellID:    hexOrEmpty(parts[5]),
			ARFCN:     dashEmpty(parts[6]),
			PCI:       hexOrEmpty(parts[7]),
			Band:      dashEmpty(parts[8]),
			Bandwidth: dashEmpty(parts[9]),
		}
		if sinr, ok := decodeGTCCSINR(atoiOr(parts[10], -1)); ok {
			cell.SINR = sinr
		}
		if len(parts) > 11 {
			if rsrp, ok := decodeRSRPField(atoiOr(parts[11], CESQUnknown)); ok {
				cell.RSRP = rsrp
			}
		}
		if len(parts) > 12 {
			if rsrq, ok := decodeRSRQField(atoiOr(parts[12], CESQUnknown)); ok {
				cell.RSRQ = rsrq
			} else if len(parts) > 13 {
				// leftover field sometimes holds CESQ-style RSRQ
				if rsrq, ok := decodeRSRQField(atoiOr(parts[len(parts)-1], CESQUnknown)); ok {
					cell.RSRQ = rsrq
				}
			}
		}
		cells = append(cells, cell)
	}
	return cells
}

// FormatBand normalizes raw band strings or vendor codes into B<n> / n<n> format.
func FormatBand(rawBand string, isNR bool) string {
	rawBand = strings.TrimSpace(rawBand)
	if rawBand == "" || rawBand == "-" || rawBand == "0" || strings.EqualFold(rawBand, "N/A") {
		return ""
	}
	u := strings.ToUpper(rawBand)
	if strings.HasPrefix(u, "B") || strings.HasPrefix(u, "N") {
		prefix := "B"
		if strings.HasPrefix(u, "N") {
			prefix = "n"
		}
		numStr := strings.TrimLeft(u[1:], "0")
		if numStr == "" {
			numStr = u[1:]
		}
		return prefix + numStr
	}
	n, err := strconv.Atoi(rawBand)
	if err == nil {
		// Fibocom vendor encoding: 101-199 often represents LTE Band (n-100), e.g. 103 -> B3
		if n > 100 && n < 200 {
			return fmt.Sprintf("B%d", n-100)
		}
		if isNR || n >= 77 || n == 41 || n == 78 || n == 79 {
			return fmt.Sprintf("n%d", n)
		}
		return fmt.Sprintf("B%d", n)
	}
	return rawBand
}

// DeriveBandFromARFCN translates EARFCN or NR-ARFCN to 3GPP Band.
func DeriveBandFromARFCN(arfcn int) string {
	if arfcn <= 0 {
		return ""
	}
	// 5G NR-ARFCN ranges. n77 and n78 overlap from 3300-3800 MHz, so
	// ARFCN alone cannot distinguish them there. Prefer the narrower/common n78
	// label in the shared range and report n77 only in its unambiguous upper span.
	switch {
	case arfcn >= 620000 && arfcn <= 653333:
		return "n78"
	case arfcn >= 653334 && arfcn <= 680000:
		return "n77"
	case arfcn >= 499200 && arfcn <= 537999:
		return "n41"
	case arfcn >= 123400 && arfcn <= 130400:
		return "n28"
	case arfcn >= 422000 && arfcn <= 434000:
		return "n1"
	case arfcn >= 361000 && arfcn <= 376000:
		return "n3"
	case arfcn >= 514000 && arfcn <= 524000:
		return "n7"
	case arfcn >= 158200 && arfcn <= 164200:
		return "n8"
	case arfcn >= 171800 && arfcn <= 178800:
		return "n20"
	}

	// LTE EARFCN ranges
	switch {
	case arfcn >= 0 && arfcn <= 599:
		return "B1"
	case arfcn >= 600 && arfcn <= 1199:
		return "B2"
	case arfcn >= 1200 && arfcn <= 1949:
		return "B3"
	case arfcn >= 1950 && arfcn <= 2399:
		return "B4"
	case arfcn >= 2400 && arfcn <= 2649:
		return "B5"
	case arfcn >= 2750 && arfcn <= 3449:
		return "B7"
	case arfcn >= 3450 && arfcn <= 3799:
		return "B8"
	case arfcn >= 5010 && arfcn <= 5179:
		return "B12"
	case arfcn >= 5180 && arfcn <= 5279:
		return "B13"
	case arfcn >= 5730 && arfcn <= 5849:
		return "B17"
	case arfcn >= 6000 && arfcn <= 6599:
		return "B20"
	case arfcn >= 9210 && arfcn <= 9659:
		return "B28"
	case arfcn >= 38650 && arfcn <= 39649:
		return "B40"
	case arfcn >= 39650 && arfcn <= 41589:
		return "B41"
	}
	return ""
}

// FormatBandwidth turns raw bandwidth representations into clean MHz strings (e.g. "20 MHz").
func FormatBandwidth(rawBW string, isNR bool) string {
	rawBW = strings.TrimSpace(rawBW)
	if rawBW == "" || rawBW == "0" || rawBW == "-" || strings.EqualFold(rawBW, "N/A") {
		return ""
	}
	u := strings.ToUpper(rawBW)
	if strings.HasSuffix(u, "MHZ") {
		val := strings.TrimSpace(strings.TrimSuffix(u, "MHZ"))
		return val + " MHz"
	}
	n, err := strconv.Atoi(rawBW)
	if err != nil {
		return rawBW
	}
	// LTE Resource blocks mapping
	if !isNR {
		switch n {
		case 6:
			return "1.4 MHz"
		case 15:
			return "3 MHz"
		case 25:
			return "5 MHz"
		case 50:
			return "10 MHz"
		case 75:
			return "15 MHz"
		case 100:
			return "20 MHz"
		}
	}
	// Standard values in kHz
	if n >= 1000 && n%1000 == 0 {
		return fmt.Sprintf("%d MHz", n/1000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1f MHz", float64(n)/1000.0)
	}
	// Direct MHz integer values
	if n > 0 && n <= 400 {
		return fmt.Sprintf("%d MHz", n)
	}
	return rawBW
}

// FormatModulation normalizes modulation identifiers (QPSK, 16QAM, 64QAM, 256QAM).
func FormatModulation(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" || raw == "255" || raw == "-1" || strings.EqualFold(raw, "N/A") {
		return ""
	}
	switch raw {
	case "1":
		return "QPSK"
	case "2":
		return "16QAM"
	case "3":
		return "64QAM"
	case "4":
		return "256QAM"
	case "5":
		return "1024QAM"
	}
	u := strings.ToUpper(raw)
	if strings.Contains(u, "QPSK") || strings.Contains(u, "QAM") || strings.Contains(u, "BPSK") {
		return u
	}
	return raw
}

// ParseGTCAINFOComponents parses PCC/SCC lines from AT+GTCAINFO?.
func ParseGTCAINFOComponents(response string) []domain.CAComponent {
	var out []domain.CAComponent
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ATResultOK || line == ATResultERROR {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "+GTCAINFO") {
			continue
		}
		comp := ""
		payload := line
		if i := strings.Index(line, ":"); i >= 0 {
			head := strings.TrimSpace(line[:i])
			hu := strings.ToUpper(strings.ReplaceAll(head, " ", ""))
			if strings.HasPrefix(hu, "PCC") {
				comp = "PCC"
			} else if strings.HasPrefix(hu, "SCC") {
				comp = hu
			}
			payload = line[i+1:]
		}
		if comp == "" {
			continue
		}
		parts := splitCSV(payload)
		if len(parts) == 0 {
			continue
		}

		ca := domain.CAComponent{Component: comp}
		isPCC := comp == "PCC"

		// Extract basic fields based on position and pattern
		if isPCC {
			ca.ULActive = true
			if len(parts) >= 8 {
				p1 := atoiOr(parts[1], 0)
				p2 := atoiOr(parts[2], 0)

				if p2 > 10000 {
					// Format seen in tests: PCC:5078,940,641760,450,2,1,1,3,19,-1,-80
					// parts[0]=5078 (ULARFCN/Band code), parts[1]=940 (UL ARFCN/PCI), parts[2]=641760 (NR-ARFCN), parts[3]=450 (BW or PCI)
					ca.ARFCN = parts[2]
					ca.ULARFCN = dashEmpty(parts[1])
					ca.PCI = dashEmpty(parts[3])
					ca.Band = DeriveBandFromARFCN(p2)
					if ca.Band == "" {
						ca.Band = FormatBand(parts[0], p2 > 100000)
					}
					if len(parts) > 6 {
						ca.DLMod = FormatModulation(parts[6])
					}
					if len(parts) > 7 {
						ca.ULMod = FormatModulation(parts[7])
					}
				} else if p1 > 100 {
					// parts[0] is band, parts[1] is DL ARFCN, parts[2] is UL ARFCN
					ca.Band = FormatBand(parts[0], p1 > 100000)
					ca.ARFCN = dashEmpty(parts[1])
					ca.ULARFCN = dashEmpty(parts[2])
					if len(parts) > 3 {
						ca.DLBW = FormatBandwidth(parts[3], p1 > 100000)
					}
					if len(parts) > 4 {
						ca.PCI = dashEmpty(parts[4])
					}
					if len(parts) > 6 {
						ca.DLMod = FormatModulation(parts[6])
					}
					if len(parts) > 7 {
						ca.ULMod = FormatModulation(parts[7])
					}
				} else {
					// Fallback positional
					ca.PCI = dashEmpty(parts[0])
					ca.ARFCN = dashEmpty(parts[1])
					ca.DLBW = FormatBandwidth(parts[2], false)
					if len(parts) > 3 {
						ca.ULBW = FormatBandwidth(parts[3], false)
					}
				}
			} else {
				ca.PCI = dashEmpty(parts[0])
				if len(parts) > 1 {
					ca.ARFCN = dashEmpty(parts[1])
				}
			}
		} else {
			// SCC component
			if len(parts) >= 9 {
				// Format: SCC<num>:<upload>,<p_cell_id>,<arfcn>,<dl_bandwidth>,<ul_bandwidth>,<dl_mimo>,<ul_mimo>,<dl_modulation>,<ul_modulation>
				// Or: SCC 3:2,0,103,216,1444,50,255,4,255,3,255,13,-9,-81
				p0 := atoiOr(parts[0], -1)
				p1 := atoiOr(parts[1], -1)
				p4 := atoiOr(parts[4], 0)

				if len(parts) >= 12 && p4 > 100 {
					// parts[0]=2, parts[1]=0 (ul_active), parts[2]=103 (Band 3), parts[3]=216 (PCI), parts[4]=1444 (ARFCN), parts[5]=50 (BW=10MHz)
					ca.ULActive = p1 > 0
					ca.Band = FormatBand(parts[2], false)
					ca.PCI = dashEmpty(parts[3])
					ca.ARFCN = dashEmpty(parts[4])
					ca.DLBW = FormatBandwidth(parts[5], false)
					if len(parts) > 7 {
						ca.DLMod = FormatModulation(parts[7])
					}
					if len(parts) > 9 {
						ca.ULMod = FormatModulation(parts[9])
					}
				} else if p0 == 0 || p0 == 1 {
					ca.ULActive = p0 == 1
					ca.PCI = dashEmpty(parts[1])
					ca.ARFCN = dashEmpty(parts[2])
					ca.DLBW = FormatBandwidth(parts[3], false)
					ca.ULBW = FormatBandwidth(parts[4], false)
					if len(parts) > 7 {
						ca.DLMod = FormatModulation(parts[7])
					}
					if len(parts) > 8 {
						ca.ULMod = FormatModulation(parts[8])
					}
				} else {
					ca.Band = FormatBand(parts[0], false)
					ca.ARFCN = dashEmpty(parts[1])
					ca.ULARFCN = dashEmpty(parts[2])
					ca.PCI = dashEmpty(parts[3])
					ca.DLBW = FormatBandwidth(parts[4], false)
				}
			} else {
				ca.PCI = dashEmpty(parts[0])
				if len(parts) > 1 {
					ca.ARFCN = dashEmpty(parts[1])
				}
			}
		}

		// Ensure band is derived if still empty but ARFCN is known
		if ca.Band == "" && ca.ARFCN != "" {
			if a, err := strconv.Atoi(ca.ARFCN); err == nil {
				ca.Band = DeriveBandFromARFCN(a)
			}
		}

		// Extract signal metrics (RSRP, RSRQ, SINR) from numbers
		nums := parseSignedInts(payload)
		for i := len(nums) - 1; i >= 0; i-- {
			n := nums[i]
			if ca.RSRP == 0 {
				if rsrp, ok := decodeRSRPField(n); ok {
					ca.RSRP = rsrp
					continue
				}
			}
			if ca.RSRQ == 0 {
				if rsrq, ok := decodeRSRQField(n); ok {
					ca.RSRQ = rsrq
					continue
				}
			}
			if ca.SINR == 0 && n > 0 && n <= 40 {
				ca.SINR = n
			}
		}
		out = append(out, ca)
	}
	return out
}

// CorrelateCAWithCells enriches CA components with missing band, bandwidth, and metrics from cell reports.
func CorrelateCAWithCells(ca []domain.CAComponent, cells []domain.CellInfo) []domain.CAComponent {
	if len(ca) == 0 || len(cells) == 0 {
		return ca
	}
	var servingCell *domain.CellInfo
	for i := range cells {
		if cells[i].Serving {
			servingCell = &cells[i]
			break
		}
	}

	for i := range ca {
		c := &ca[i]
		isPCC := c.Component == "PCC"
		if isPCC && servingCell != nil {
			if c.Band == "" && servingCell.Band != "" {
				c.Band = FormatBand(servingCell.Band, servingCell.RAT == domain.Tech5GNR)
			}
			if c.DLBW == "" && servingCell.Bandwidth != "" {
				c.DLBW = FormatBandwidth(servingCell.Bandwidth, servingCell.RAT == domain.Tech5GNR)
			}
			if c.PCI == "" && servingCell.PCI != "" {
				c.PCI = servingCell.PCI
			}
			if c.ARFCN == "" && servingCell.ARFCN != "" {
				c.ARFCN = servingCell.ARFCN
			}
			if c.RSRP == 0 && servingCell.RSRP != 0 {
				c.RSRP = servingCell.RSRP
			}
			if c.RSRQ == 0 && servingCell.RSRQ != 0 {
				c.RSRQ = servingCell.RSRQ
			}
			if c.SINR == 0 && servingCell.SINR != 0 {
				c.SINR = servingCell.SINR
			}
		} else {
			// Find cell matching PCI or ARFCN
			for _, cell := range cells {
				if (c.PCI != "" && cell.PCI == c.PCI) || (c.ARFCN != "" && cell.ARFCN == c.ARFCN) {
					if c.Band == "" && cell.Band != "" {
						c.Band = FormatBand(cell.Band, cell.RAT == domain.Tech5GNR)
					}
					if c.DLBW == "" && cell.Bandwidth != "" {
						c.DLBW = FormatBandwidth(cell.Bandwidth, cell.RAT == domain.Tech5GNR)
					}
					if c.SINR == 0 && cell.SINR != 0 {
						c.SINR = cell.SINR
					}
					break
				}
			}
		}
	}
	return ca
}

// DetectRadioTech determines active technology (5G NSA, 5G SA, LTE, UMTS) and EN-DC details.
func DetectRadioTech(copsAct int, hasCopsAct bool, regLTE, reg5g domain.RegState, cells []domain.CellInfo, ca []domain.CAComponent) (domain.RadioTech, domain.ENDCInfo) {
	var endcInfo domain.ENDCInfo
	var anchorBand, nrBand string
	hasNRCell := false
	hasLTEServing := false
	hasUMTSCell := false

	for _, c := range cells {
		if c.RAT == domain.Tech5GNR {
			hasNRCell = true
			if nrBand == "" && c.Band != "" {
				nrBand = FormatBand(c.Band, true)
			}
		}
		if c.RAT == domain.TechUMTS {
			hasUMTSCell = true
		}
		if c.Serving && (c.RAT == domain.TechLTE || c.RAT == domain.TechUnknown) {
			hasLTEServing = true
			if anchorBand == "" && c.Band != "" {
				anchorBand = FormatBand(c.Band, false)
			}
		}
	}

	for _, c := range ca {
		if strings.HasPrefix(strings.ToLower(c.Band), "n") || (c.ARFCN != "" && atoiOr(c.ARFCN, 0) >= 100000) {
			hasNRCell = true
			if nrBand == "" {
				nrBand = c.Band
			}
		} else if c.Component == "PCC" && anchorBand == "" && c.Band != "" {
			anchorBand = c.Band
		}
	}

	// EN-DC detection criteria:
	// 1. COPS AcT is 13 (E-UTRA-NR dual connectivity) or 10 (E-UTRA connected to 5GC)
	// 2. Or dual registered on both LTE and 5G
	// 3. Or both LTE anchor and active 5G NR carrier present
	isENDC := (hasCopsAct && (copsAct == 13 || copsAct == 10)) ||
		(regLTE.IsRegistered() && reg5g.IsRegistered()) ||
		(regLTE.IsRegistered() && hasNRCell && hasLTEServing)

	if isENDC {
		endcInfo = domain.ENDCInfo{
			Active:     true,
			AnchorBand: anchorBand,
			NRBand:     nrBand,
			State:      domain.ENDCStateActive,
		}
		return domain.Tech5GNSA, endcInfo
	}

	if (hasCopsAct && copsAct == 11) || reg5g.IsRegistered() || (hasNRCell && !hasLTEServing) {
		endcInfo = domain.ENDCInfo{
			Active: false,
			NRBand: nrBand,
			State:  domain.ENDCStateInactive,
		}
		return domain.Tech5GSA, endcInfo
	}

	if (hasCopsAct && (copsAct == 7 || copsAct == 4)) || regLTE.IsRegistered() || hasLTEServing {
		endcInfo = domain.ENDCInfo{
			Active:     false,
			AnchorBand: anchorBand,
			State:      domain.ENDCStateInactive,
		}
		return domain.TechLTE, endcInfo
	}

	if (hasCopsAct && copsAct == 2) || hasUMTSCell {
		return domain.TechUMTS, endcInfo
	}

	return domain.TechUnknown, endcInfo
}

// ApplyServingCell fills missing RSRP/RSRQ/SINR from the serving GTCCINFO row.
func ApplyServingCell(sig domain.SignalInfo, cells []domain.CellInfo) domain.SignalInfo {
	for _, c := range cells {
		if !c.Serving {
			continue
		}
		if sig.RSRP == 0 && c.RSRP != 0 {
			sig = applyRSRPRSRQ(sig, c.RSRP, c.RSRQ)
		} else if sig.RSRQ == 0 && c.RSRQ != 0 {
			sig.RSRQ = c.RSRQ
		}
		if sig.SINR == 0 && c.SINR != 0 {
			sig.SINR = c.SINR
		}
		return sig
	}
	return sig
}

func splitCSV(line string) []string {
	raw := strings.Split(line, ",")
	out := make([]string, len(raw))
	for i, p := range raw {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func dashEmpty(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return ""
	}
	return s
}

func hexOrEmpty(s string) string {
	s = strings.TrimSpace(s)
	u := strings.ToUpper(s)
	if s == "" || u == "0XFFFFFFF" || u == "00FFFFFFF" || u == "FFFFFFF" {
		return ""
	}
	return s
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func decodeRSRPField(raw int) (int, bool) {
	if raw >= -140 && raw <= -44 {
		return raw, true
	}
	if raw >= 0 && raw <= RSRPMaxRaw && raw != CESQUnknown {
		return RSRPMinDBm + raw, true
	}
	return 0, false
}

func decodeRSRQField(raw int) (int, bool) {
	if raw >= -20 && raw <= -3 {
		return raw, true
	}
	if raw >= 0 && raw <= RSRQMaxRaw && raw != CESQUnknown {
		tenths := -195 + 5*raw
		if tenths >= 0 {
			return (tenths + 5) / 10, true
		}
		return (tenths - 5) / 10, true
	}
	return 0, false
}

// decodeGTCCSINR treats Fibocom SINR as 0.5 dB units (vendor /2).
func decodeGTCCSINR(raw int) (int, bool) {
	if raw < 0 || raw == 255 || raw == 99 {
		return 0, false
	}
	return raw / 2, true
}

func ParseGTACT(response string) domain.RATMode {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+GTACT:") {
			parts := strings.Split(strings.TrimPrefix(line, "+GTACT:"), ",")
			if len(parts) >= 1 {
				return domain.ParseGTACTCodeString(parts[0])
			}
		}
	}
	return domain.RATModeUnspecified
}
