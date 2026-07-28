package domain

// Known Fibocom FM350-GL USB composition modes (AT+GTUSBMODE).
// Stock USB firmware documents 40 and 41 — both RNDIS+serial, not MBIM.
const (
	USBModeUnknown = 0
	USBModeRNDIS40 = 40 // RNDIS + AT + GNSS + META + DEBUG + NPT + ADB
	USBModeRNDIS41 = 41 // default: mode 40 + AP(LOG)+AP(META)
)

// USBModeOption is one selectable composition profile for the UI.
type USBModeOption struct {
	Mode  int    `json:"mode"`
	Label string `json:"label"`
	Note  string `json:"note,omitempty"`
}

// USBModeInfo is the current / supported USB composition snapshot.
type USBModeInfo struct {
	Mode      int             `json:"mode"`
	Label     string          `json:"label"`
	Supported []USBModeOption `json:"supported"`
	Note      string          `json:"note,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// KnownUSBModes returns documented FM350 USB profiles.
func KnownUSBModes() []USBModeOption {
	return []USBModeOption{
		{Mode: USBModeRNDIS40, Label: "40 — RNDIS + AT (compact)", Note: "Fewer serial interfaces"},
		{Mode: USBModeRNDIS41, Label: "41 — RNDIS + AT (default)", Note: "Default composition; not MBIM"},
	}
}

// USBModeLabel returns a short label for a mode code.
func USBModeLabel(mode int) string {
	for _, o := range KnownUSBModes() {
		if o.Mode == mode {
			return o.Label
		}
	}
	if mode == 0 {
		return "unknown"
	}
	return "custom"
}
