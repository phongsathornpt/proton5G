package domain

import (
	"fmt"
	"strings"
)

// InterfaceKind is a stable API token for modem child-node kinds.
type InterfaceKind string

const (
	IfaceKindAT     InterfaceKind = "at"
	IfaceKindMBIM   InterfaceKind = "mbim"
	IfaceKindSerial InterfaceKind = "serial"
	IfaceKindNet    InterfaceKind = "net" // RNDIS/ECM network interface
)

// DataMode is a stable API token for modem data paths and USB composition modes.
type DataMode string

const (
	DataModeNone   DataMode = "none"
	DataModeAuto   DataMode = "auto"
	DataModeRNDIS  DataMode = "rndis"
	DataModeMBIM   DataMode = "mbim"
	DataModeMixed  DataMode = "mixed"
	DataModeATOnly DataMode = "at_only"
)

// ParseDataMode validates request-facing data modes. Empty input means auto;
// the legacy "net" alias is normalized to RNDIS.
func ParseDataMode(raw string) (DataMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(DataModeAuto):
		return DataModeAuto, nil
	case string(DataModeRNDIS), "net":
		return DataModeRNDIS, nil
	case string(DataModeMBIM):
		return DataModeMBIM, nil
	default:
		return "", fmt.Errorf("invalid data mode %q (want auto|rndis|mbim)", raw)
	}
}

// DataMethod controls host-side RNDIS address configuration.
type DataMethod string

const (
	DataMethodAuto   DataMethod = "auto"   // modem-reported PDP config, then DHCP fallback
	DataMethodDHCP   DataMethod = "dhcp"   // DHCP only
	DataMethodStatic DataMethod = "static" // require modem-reported address/prefix/gateway
)

// ParseDataMethod validates and normalizes RNDIS address methods.
func ParseDataMethod(raw string) (DataMethod, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(DataMethodAuto):
		return DataMethodAuto, nil
	case string(DataMethodDHCP):
		return DataMethodDHCP, nil
	case string(DataMethodStatic):
		return DataMethodStatic, nil
	default:
		return "", fmt.Errorf("invalid data method %q (want auto|dhcp|static)", raw)
	}
}

// ModemInterface is one device node belonging to a modem (serial AT, MBIM, or net).
type ModemInterface struct {
	Path    string        `json:"path"`
	Kind    InterfaceKind `json:"kind"`
	ATReady bool          `json:"at_ready,omitempty"`
	Label   string        `json:"label"`
	State   string        `json:"state,omitempty"` // for net: UP/DOWN
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
	DataMode     DataMode         `json:"data_mode"`
	ATPorts      []ModemInterface `json:"at_ports"`
	MBIMNodes    []ModemInterface `json:"mbim_nodes"`
	NetIfaces    []ModemInterface `json:"net_ifaces"`
}

// ModemInventory is the full discovery snapshot + active selection.
type ModemInventory struct {
	Modems          []ModemDevice `json:"modems"`
	SelectedModemID string        `json:"selected_modem_id"`
	SelectedATPort  string        `json:"selected_at_port"`
	SelectedMBIM    string        `json:"selected_mbim"`
	SelectedNet     string        `json:"selected_net"`
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
	Mode   DataMode   `json:"mode"`
	Iface  string     `json:"iface"`
	APN    string     `json:"apn,omitempty"`
	Method DataMethod `json:"method,omitempty"`
}
