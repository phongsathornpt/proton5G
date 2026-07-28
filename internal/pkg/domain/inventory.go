package domain

// Interface kinds for modem child nodes.
const (
	IfaceKindAT     = "at"
	IfaceKindMBIM   = "mbim"
	IfaceKindSerial = "serial"
)

// ModemInterface is one device node belonging to a modem (serial AT or MBIM).
type ModemInterface struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	ATReady bool   `json:"at_ready,omitempty"`
	Label   string `json:"label"`
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
	ATPorts      []ModemInterface `json:"at_ports"`
	MBIMNodes    []ModemInterface `json:"mbim_nodes"`
}

// ModemInventory is the full discovery snapshot + active selection.
type ModemInventory struct {
	Modems          []ModemDevice `json:"modems"`
	SelectedModemID string        `json:"selected_modem_id"`
	SelectedATPort  string        `json:"selected_at_port"`
	SelectedMBIM    string        `json:"selected_mbim"`
	MBIMCLI         bool          `json:"mbimcli_available"`
	InstallHint     string        `json:"install_hint,omitempty"`
	Note            string        `json:"note,omitempty"`
}

// ModemSelectRequest is the body for POST /api/modems/select.
type ModemSelectRequest struct {
	ModemID    string `json:"modem_id"`
	ATPort     string `json:"at_port,omitempty"`
	MBIMDevice string `json:"mbim_device,omitempty"`
}
