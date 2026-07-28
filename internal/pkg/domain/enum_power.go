package domain

import "strings"

// PowerControl is USB power/control sysfs value.
type PowerControl string

const (
	PowerOn      PowerControl = "on"
	PowerAuto    PowerControl = "auto"
	PowerUnknown PowerControl = "unknown"
)

// NormalizePowerControl maps sysfs power/control text.
func NormalizePowerControl(raw string) PowerControl {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(PowerOn):
		return PowerOn
	case string(PowerAuto):
		return PowerAuto
	case "":
		return PowerUnknown
	default:
		return PowerControl(strings.TrimSpace(raw))
	}
}
