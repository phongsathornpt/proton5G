package domain

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// FM350GLMaxDLMbps is Fibocom's published theoretical Sub-6 downlink ceiling.
const FM350GLMaxDLMbps = 4670

// downlinkEstimateEfficiency is a deliberately conservative heuristic factor used
// after modulation bits/Hz and MIMO layers. It absorbs OFDM/control overhead and a
// typical TDD/FDD scheduling allowance, but it is not a 3GPP peak-throughput formula.
const downlinkEstimateEfficiency = 0.70

// DownlinkCapacity summarizes the live downlink carrier set and a heuristic peak
// derived from AT+GTCAINFO telemetry. EstimatedPeakMbps is not a guaranteed speed:
// operator policy, TDD slot pattern, scheduler load, RF quality, protocol overhead,
// backhaul, and the subscriber plan can all lower real throughput.
type DownlinkCapacity struct {
	ActiveCC          int      `json:"active_cc"`
	LTECC             int      `json:"lte_cc"`
	NRCC              int      `json:"nr_cc"`
	BandwidthKnownCC  int      `json:"bandwidth_known_cc"`
	TotalBandwidthMHz float64  `json:"total_bandwidth_mhz"`
	LTEBandwidthMHz   float64  `json:"lte_bandwidth_mhz"`
	NRBandwidthMHz    float64  `json:"nr_bandwidth_mhz"`
	Bands             []string `json:"bands,omitempty"`
	BestDLMIMO        string   `json:"best_dl_mimo,omitempty"`
	BestDLModulation  string   `json:"best_dl_modulation,omitempty"`
	EstimatedPeakMbps int      `json:"estimated_peak_mbps,omitempty"`
	EstimatedFromCC   int      `json:"estimated_from_cc"`
	EstimateComplete  bool     `json:"estimate_complete"`
	DeviceCeilingMbps int      `json:"device_ceiling_mbps"`
	Limiter           string   `json:"limiter,omitempty"`
	EstimateMethod    string   `json:"estimate_method,omitempty"`
}

// DeriveDownlinkCapacity summarizes the live CA report and estimates a useful
// radio-side peak. The estimate is intentionally marked partial whenever a carrier
// is missing bandwidth, MIMO, or modulation telemetry.
func DeriveDownlinkCapacity(ca []CAComponent) DownlinkCapacity {
	out := DownlinkCapacity{
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

		bw := parseCapacityBandwidthMHz(c.DLBW)
		if bw > 0 {
			out.BandwidthKnownCC++
			out.TotalBandwidthMHz += bw
		}

		switch capacityCarrierRAT(band) {
		case "NR":
			out.NRCC++
			out.NRBandwidthMHz += bw
		case "LTE":
			out.LTECC++
			out.LTEBandwidthMHz += bw
		}

		layers := parseCapacityMIMOLayers(c.DLMIMO)
		if layers > bestLayers {
			bestLayers = layers
			out.BestDLMIMO = c.DLMIMO
		}

		bits := capacityModulationBits(c.DLMod)
		if bits > bestBits {
			bestBits = bits
			out.BestDLModulation = c.DLMod
		}

		if bw > 0 && layers > 0 && bits > 0 {
			out.EstimatedFromCC++
			uncappedMbps += bw * float64(bits) * float64(layers) * downlinkEstimateEfficiency
		}
	}

	out.TotalBandwidthMHz = capacityRound1(out.TotalBandwidthMHz)
	out.LTEBandwidthMHz = capacityRound1(out.LTEBandwidthMHz)
	out.NRBandwidthMHz = capacityRound1(out.NRBandwidthMHz)
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

// MarshalJSON keeps downlink_capacity derived from the same CA snapshot that is
// serialized to REST/SSE clients. The stored field remains available for future
// callers that want to override it explicitly.
func (s FullStatus) MarshalJSON() ([]byte, error) {
	if s.DownlinkCapacity.DeviceCeilingMbps == 0 {
		s.DownlinkCapacity = DeriveDownlinkCapacity(s.CA)
	}
	type fullStatusAlias FullStatus
	return json.Marshal(fullStatusAlias(s))
}

func isReportedDownlinkCarrier(c CAComponent) bool {
	return strings.TrimSpace(c.Band) != "" || strings.TrimSpace(c.ARFCN) != "" || strings.TrimSpace(c.DLBW) != ""
}

func capacityCarrierRAT(band string) string {
	b := strings.ToLower(strings.TrimSpace(band))
	if strings.HasPrefix(b, "n") {
		return "NR"
	}
	if strings.HasPrefix(b, "b") {
		return "LTE"
	}
	return ""
}

func parseCapacityBandwidthMHz(raw string) float64 {
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

func parseCapacityMIMOLayers(raw string) int {
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

func capacityModulationBits(raw string) int {
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

func capacityRound1(v float64) float64 {
	return math.Round(v*10) / 10
}
