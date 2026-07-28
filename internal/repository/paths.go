package repository

// Linux path globs and sysfs nodes.
const (
	SysfsUSBGlob    = "/sys/bus/usb/devices/*"
	AutosuspendPath = "/sys/module/usbcore/parameters/autosuspend"
	AutosuspendOff  = "-1"
	TTYUSBGlob      = "/dev/ttyUSB*"
	DefaultATPort   = "/dev/ttyUSB0"
	USBFSNodeFmt    = "/dev/bus/usb/%03d/%03d"
	CDCWdmGlob      = "/dev/cdc-wdm*"

	// usbdevfsReset is USBDEVFS_RESET (_IO('U', 20)) on Linux.
	usbdevfsReset = 0x5514
)

// DefaultCDCWdm lists common MBIM control device nodes.
var DefaultCDCWdm = []string{
	"/dev/cdc-wdm0",
	"/dev/cdc-wdm1",
}
