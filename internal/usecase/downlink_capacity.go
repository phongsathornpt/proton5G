package usecase

import (
	"math"
	"strconv"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

// FM350GLMaxDLMbps is Fibocom's published theoretical Sub-6 downlink ceiling.
const FM350GLMaxDLMbps = 4670

// downlinkEstimateEfficiency is a deliberately conservative heuristic factor used
// after modulation bits/Hz and MIMO layers. It absorbs OFDM/control overhead and a
// typical TDD/FDD scheduling allowance, but it is not a 3GPP peak-throughput formula.
const downlinkEstimateEfficiency = 0.70

// DeriveDownlinkCapacity summarizes the live CA report and estimates a useful
// radio-side peak. The estimate is intentionally marked partial whenever a carrier
// is missing bandwidth, MIMO, or modulation telemetry.
func DeriveDownlinkCapacity(ca []domain.CAComponent) domain.DownlinkCapacity {
	out := domain.DownlinkCapacity{
		DeviceCeilingMbps: FM350GLMaxDLMbps,
		EstimateMethod:    "sum(MHz × modulation bits × MIMO layers × 0.70), capped at FM350-GL 4.67 Gbps",
	}
	if len(ca) == 0 {
		return out
	}

	seenBands := make(map[string]struct{})
	bestLayers := 0
	bestBits := 0
	uncappedMbps := 0.0

	for _, c := range ca {
		if !isReportedDownlinkCarrier(c) {
			continue
		}
		out.ActiveCC++

		band := strings.TrimSpace(c.Band)
		if band != "" {
			if _, ok := seenBands[band]; !ok {
				seenBands[band] = struct{}{}
				out.Bands = append(out.Bands, band)
			}
		}

		bw := parseBandwidthMHz(c.DLBW)
		if bw > 0 {
			out.BandwidthKnownCC++
			out.TotalBandwidthMHz += bw
		}

		rat := carrierRAT(band)
		switch rat {
		case "NR":
			out.NRCC++
			out.NRBandwidthMHz += bw
		case "LTE":
			out.LTECC++
			out.LTEBandwidthMHz += bw
		}

		layers := parseMIMOLayers(c.DLMIMO)
		if layers > bestLayers {
			bestLayers = layers
			out.BestDLMIMO = c.DLMIMO
		}

		bits := modulationBits(c.DLMod)
		if bits > bestBits {
			bestBits = bits
			out.BestDLModulation = c.DLMod
		}

		if bw > 0 && layers > 0 && bits > 0 {
			out.EstimatedFromCC++
			uncappedMbps += bw * float64(bits) * float64(layers) * downlinkEstimateEfficiency
		}
	}

	out.TotalBandwidthMHz = round1(out.TotalBandwidthMHz)
	out.LTEBandwidthMHz = round1(out.LTEBandwidthMHz)
	out.NRBandwidthMHz = round1(out.NRBandwidthMHz)
	out.EstimateComplete = out.ActiveCC > 0 && out.EstimatedFromCC == out.ActiveCC

	if uncappedMbps > 0 {
		out.EstimatedPeakMbps = int(math.Round(math.Min(uncappedMbps, FM350GLMaxDLMbps)))
		if uncappedMbps > FM350GLMaxDLMbps {
			out.Limiter = "FM350-GL device ceiling"
		} else {
			out.Limiter = "reported CA / radio configuration"
		}
	}

	return out
}

func isReportedDownlinkCarrier(c domain.CAComponent) bool {
	return strings.TrimSpace(c.Band) != "" || strings.TrimSpace(c.ARFCN) != "" || strings.TrimSpace(c.DLBW) != ""
}

func carrierRAT(band string) string {
	b := strings.ToLower(strings.TrimSpace(band))
	if strings.HasPrefix(b, "n") {
		return "NR"
	}
	if strings.HasPrefix(b, "b") {
		return "LTE"
	}
	return ""
}

func parseBandwidthMHz(raw string) float64 {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "-" || raw == "n/a" {
		return 0
	}
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "mhz"))
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v > 400 {
		return 0
	}
	return v
}

func parseMIMOLayers(raw string) int {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "-" || raw == "n/a" {
		return 0
	}
	if i := strings.Index(raw, "x"); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 8 {
		return 0
	}
	return n
}

func modulationBits(raw string) int {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "BPSK":
		return 1
	case "QPSK":
		return 2
	case "16QAM":
		return 4
	case "64QAM":
		return 6
	case "256QAM":
		return 8
	case "1024QAM":
		return 10
	default:
		return 0
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
