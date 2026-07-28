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

// MergeExtendedSignal fills RSRP/RSRQ from CESQ and optionally improves percentage from RSRP.
func MergeExtendedSignal(base domain.SignalInfo, cesqResp string) domain.SignalInfo {
	rsrp, rsrq, ok := ParseCESQ(cesqResp)
	if !ok {
		return base
	}
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
