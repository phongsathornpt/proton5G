package repository

import (
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

func ParseCOPS(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+COPS:") {
			parts := strings.Split(line, ",")
			if len(parts) >= 3 {
				return strings.Trim(parts[2], "\" ")
			}
		}
	}
	return unknownOperator
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
