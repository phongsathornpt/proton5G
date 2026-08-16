package domain

// DownlinkCapacity summarizes the live downlink carrier set and a heuristic peak
// derived from AT+GTCAINFO telemetry. EstimatedPeakMbps is not a guaranteed speed:
// operator policy, TDD slot pattern, scheduler load, RF quality, protocol overhead,
// backhaul, and the subscriber plan can all lower real throughput.
type DownlinkCapacity struct {
	ActiveCC             int      `json:"active_cc"`
	LTECC                int      `json:"lte_cc"`
	NRCC                 int      `json:"nr_cc"`
	BandwidthKnownCC     int      `json:"bandwidth_known_cc"`
	TotalBandwidthMHz    float64  `json:"total_bandwidth_mhz"`
	LTEBandwidthMHz      float64  `json:"lte_bandwidth_mhz"`
	NRBandwidthMHz       float64  `json:"nr_bandwidth_mhz"`
	Bands                []string `json:"bands,omitempty"`
	BestDLMIMO           string   `json:"best_dl_mimo,omitempty"`
	BestDLModulation     string   `json:"best_dl_modulation,omitempty"`
	EstimatedPeakMbps    int      `json:"estimated_peak_mbps,omitempty"`
	EstimatedFromCC      int      `json:"estimated_from_cc"`
	EstimateComplete     bool     `json:"estimate_complete"`
	DeviceCeilingMbps    int      `json:"device_ceiling_mbps"`
	Limiter              string   `json:"limiter,omitempty"`
	EstimateMethod       string   `json:"estimate_method,omitempty"`
}
