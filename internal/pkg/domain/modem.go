package domain

import "time"

// ModemStatus represents the current physical USB state of the FM350-GL.
type ModemStatus struct {
	Connected    bool         `json:"connected"`
	SysPath      string       `json:"sys_path,omitempty"`
	PortPath     string       `json:"port_path,omitempty"`
	PowerControl PowerControl `json:"power_control,omitempty"`
}

// SignalInfo holds current RSSI, RSRP, RSRQ, SINR and mapped percentage.
type SignalInfo struct {
	RSSI       int    `json:"rssi"`           // -113 to -51 dBm
	RSRP       int    `json:"rsrp"`           // -140 to -44 dBm
	RSRQ       int    `json:"rsrq"`           // -20 to -3 dB
	SINR       int    `json:"sinr,omitempty"` // dB; 0 = unknown
	Percentage int    `json:"percentage"`     // 0-100%
	RawCSQ     string `json:"raw_csq"`
}

// NetworkInfo holds registration state, operator, and technology.
type NetworkInfo struct {
	Operator string    `json:"operator"`
	RegState RegState  `json:"reg_state"`
	Tech     RadioTech `json:"tech"`
	Reg5G    RegState  `json:"reg_5g"`
	RegLTE   RegState  `json:"reg_lte"`
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
	SINR       int       `json:"sinr,omitempty"`
	Percentage int       `json:"percentage"`
	Tech       RadioTech `json:"tech,omitempty"`
}

// ModemIdentity is AT+CGMI/CGMM/CGMR/CGSN (cached after first success).
type ModemIdentity struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Firmware     string `json:"firmware,omitempty"`
	IMEI         string `json:"imei,omitempty"`
}

// CellInfo is one AT+GTCCINFO? neighbor or serving cell.
type CellInfo struct {
	Serving   bool      `json:"serving"`
	RAT       RadioTech `json:"rat"`
	MCC       string    `json:"mcc,omitempty"`
	MNC       string    `json:"mnc,omitempty"`
	TAC       string    `json:"tac,omitempty"`
	CellID    string    `json:"cell_id,omitempty"`
	PCI       string    `json:"pci,omitempty"`
	ARFCN     string    `json:"arfcn,omitempty"`
	Band      string    `json:"band,omitempty"`
	Bandwidth string    `json:"bandwidth,omitempty"`
	RSRP      int       `json:"rsrp,omitempty"`
	RSRQ      int       `json:"rsrq,omitempty"`
	SINR      int       `json:"sinr,omitempty"`
}

// CAComponent is one AT+GTCAINFO? primary or secondary carrier.
type CAComponent struct {
	Component string `json:"component"`            // PCC, SCC1, …
	Band      string `json:"band,omitempty"`       // B3, B1, n78, etc.
	PCI       string `json:"pci,omitempty"`
	ARFCN     string `json:"arfcn,omitempty"`      // DL EARFCN / NR-ARFCN
	ULARFCN   string `json:"ul_arfcn,omitempty"`   // UL EARFCN / NR-ARFCN
	DLBW      string `json:"dl_bandwidth,omitempty"`
	ULBW      string `json:"ul_bandwidth,omitempty"`
	DLMod     string `json:"dl_modulation,omitempty"`
	ULMod     string `json:"ul_modulation,omitempty"`
	ULActive  bool   `json:"ul_active,omitempty"`  // true when UL transmission / UL CA is active
	RSRP      int    `json:"rsrp,omitempty"`
	RSRQ      int    `json:"rsrq,omitempty"`
	SINR      int    `json:"sinr,omitempty"`
}

// PDPSession is the active PDP address/DNS from AT+CGPADDR / AT+GTDNS.
type PDPSession struct {
	CID     int    `json:"cid,omitempty"`
	IP      string `json:"ip,omitempty"`
	DNS1    string `json:"dns1,omitempty"`
	DNS2    string `json:"dns2,omitempty"`
	Gateway string `json:"gateway,omitempty"` // guessed .1; not from AT
}

// WANInfo is host-side RNDIS iface addresses and byte counters.
type WANInfo struct {
	Iface     string   `json:"iface,omitempty"`
	Method    string   `json:"method,omitempty"`  // dhcp | static
	Session   string   `json:"session,omitempty"` // connected | disconnected
	Addrs     []string `json:"addrs,omitempty"`
	RxBytes   uint64   `json:"rx_bytes,omitempty"`
	TxBytes   uint64   `json:"tx_bytes,omitempty"`
	RxRateBps int64    `json:"rx_rate_bps,omitempty"`
	TxRateBps int64    `json:"tx_rate_bps,omitempty"`
}

// ATPoll is one AT repository status sample (no USB/WAN host state).
type ATPoll struct {
	Signal   SignalInfo
	Network  NetworkInfo
	SIM      SIMInfo
	APN      APNConfig
	RATMode  RATMode
	Identity ModemIdentity
	TempC    float64
	Cells    []CellInfo
	CA       []CAComponent
	PDP      PDPSession
}

// FullStatus aggregates all modem metrics for WebUI and SSE.
type FullStatus struct {
	Modem        ModemStatus   `json:"modem"`
	Signal       SignalInfo    `json:"signal"`
	Network      NetworkInfo   `json:"network"`
	SIM          SIMInfo       `json:"sim"`
	APN          APNConfig     `json:"apn"`
	RATMode      RATMode       `json:"rat_mode"`
	Identity     ModemIdentity `json:"identity,omitempty"`
	TemperatureC float64       `json:"temperature_c,omitempty"`
	Cells        []CellInfo    `json:"cells,omitempty"`
	CA           []CAComponent `json:"ca,omitempty"`
	PDP          PDPSession    `json:"pdp,omitempty"`
	WAN          WANInfo       `json:"wan,omitempty"`
	Error        string        `json:"error,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at,omitempty"` // last successful poll (or last error update)
}
