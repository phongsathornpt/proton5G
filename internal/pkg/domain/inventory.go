package domain

// Interface kinds for modem child nodes.
const (
	IfaceKindAT     = "at"
	IfaceKindMBIM   = "mbim"
	IfaceKindSerial = "serial"
	IfaceKindNet    = "net" // RNDIS/ECM network interface
)

// Data path modes detected on the USB composition.
const (
	DataModeNone   = "none"
	DataModeRNDIS  = "rndis"
	DataModeMBIM   = "mbim"
	DataModeMixed  = "mixed"
	DataModeATOnly = "at_only"
)

// RNDIS address methods for POST /api/data/connect.
const (
	DataMethodAuto   = "auto" // DHCP, then PDP static from CGPADDR
	DataMethodDHCP   = "dhcp"
	DataMethodStatic = "static"
)

// ModemInterface is one device node belonging to a modem (serial AT, MBIM, or net).
type ModemInterface struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	ATReady bool   `json:"at_ready,omitempty"`
	Label   string `json:"label"`
	State   string `json:"state,omitempty"` // for net: UP/DOWN
}

// ModemDevice is one logical USB modem (or synthetic serial-only entry).
type ModemDevice struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	VendorID     string           `json:"vendor_id,omitempty"`
	ProductID    string           `json:"product_id,omitempty"`
	SysPath      string           `json:"sys_path,omitempty"`
	Connected    bool             `json:"connected"`
	PowerControl PowerControl     `json:"power_control,omitempty"`
	DataMode     string           `json:"data_mode"` // rndis | mbim | at_only | mixed | none
	ATPorts      []ModemInterface `json:"at_ports"`
	MBIMNodes    []ModemInterface `json:"mbim_nodes"`
	NetIfaces    []ModemInterface `json:"net_ifaces"` // RNDIS etc.
}

// ModemInventory is the full discovery snapshot + active selection.
type ModemInventory struct {
	Modems          []ModemDevice `json:"modems"`
	SelectedModemID string        `json:"selected_modem_id"`
	SelectedATPort  string        `json:"selected_at_port"`
	SelectedMBIM    string        `json:"selected_mbim"`
	SelectedNet     string        `json:"selected_net"` // RNDIS iface name
	MBIMCLI         bool          `json:"mbimcli_available"`
	InstallHint     string        `json:"install_hint,omitempty"`
	Note            string        `json:"note,omitempty"`
}

// ModemSelectRequest is the body for POST /api/modems/select.
type ModemSelectRequest struct {
	ModemID    string `json:"modem_id"`
	ATPort     string `json:"at_port,omitempty"`
	MBIMDevice string `json:"mbim_device,omitempty"`
	NetIface   string `json:"net_iface,omitempty"`
}

// DataConnectRequest brings up a data path (RNDIS iface or MBIM device).
type DataConnectRequest struct {
	Mode   string `json:"mode"`  // "rndis" | "mbim"
	Iface  string `json:"iface"` // net name or /dev/cdc-wdm*
	APN    string `json:"apn,omitempty"`
	Method string `json:"method,omitempty"` // auto | dhcp | static (RNDIS only)
}
