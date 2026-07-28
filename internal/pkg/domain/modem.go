package domain

import "time"

// ModemStatus represents the current physical USB state of the FM350-GL.
type ModemStatus struct {
	Connected    bool         `json:"connected"`
	SysPath      string       `json:"sys_path,omitempty"`
	PortPath     string       `json:"port_path,omitempty"`
	PowerControl PowerControl `json:"power_control,omitempty"`
}

// SignalInfo holds current RSSI, RSRP, RSRQ and mapped percentage.
type SignalInfo struct {
	RSSI       int    `json:"rssi"`       // -113 to -51 dBm
	RSRP       int    `json:"rsrp"`       // -140 to -44 dBm
	RSRQ       int    `json:"rsrq"`       // -20 to -3 dBm
	Percentage int    `json:"percentage"` // 0-100%
	RawCSQ     string `json:"raw_csq"`
}

// NetworkInfo holds registration state, operator, and technology.
type NetworkInfo struct {
	Operator string   `json:"operator"`
	RegState RegState `json:"reg_state"`
	Tech     RadioTech `json:"tech"`
	Reg5G    RegState `json:"reg_5g"`
	RegLTE   RegState `json:"reg_lte"`
}

// SIMInfo holds SIM readiness, IMSI, and ICCID.
type SIMInfo struct {
	State SIMState `json:"state"`
	IMSI  string   `json:"imsi,omitempty"`
	ICCID string   `json:"iccid,omitempty"`
}

// APNConfig represents PDP context APN configuration.
type APNConfig struct {
	CID     int     `json:"cid"`
	PDPType PDPType `json:"pdp_type"`
	APN     string  `json:"apn"`
	IPAddr  string  `json:"ip_addr,omitempty"`
}

// SignalSample is one historical datapoint for charts / logging.
type SignalSample struct {
	Timestamp  time.Time `json:"timestamp"`
	RSSI       int       `json:"rssi"`
	RSRP       int       `json:"rsrp,omitempty"`
	RSRQ       int       `json:"rsrq,omitempty"`
	Percentage int       `json:"percentage"`
	Tech       RadioTech `json:"tech,omitempty"`
}

// FullStatus aggregates all modem metrics for WebUI and SSE.
type FullStatus struct {
	Modem   ModemStatus `json:"modem"`
	Signal  SignalInfo  `json:"signal"`
	Network NetworkInfo `json:"network"`
	SIM     SIMInfo     `json:"sim"`
	APN     APNConfig   `json:"apn"`
	RATMode RATMode     `json:"rat_mode"`
	Error   string      `json:"error,omitempty"`
}
