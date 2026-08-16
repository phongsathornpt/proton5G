package usecase

import (
	"errors"
	"strings"
)

var errModemUnavailable = errors.New("modem not connected")

func IsModemUnavailable(err error) bool {
	return errors.Is(err, errModemUnavailable)
}

// isPermissionError reports OS-level access denied (serial, usbfs, sysfs).
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "access is denied")
}

// formatDeviceError adds operator guidance for common failure modes.
func formatDeviceError(err error) string {
	if err == nil {
		return ""
	}
	if isPermissionError(err) {
		return err.Error() + " — run as root, or: sudo usermod -aG dialout $USER && re-login " +
			"(serial); hard reset also needs root for /dev/bus/usb"
	}
	return err.Error()
}
