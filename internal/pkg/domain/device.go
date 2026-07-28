package domain

// USBID identifies a USB device by vendor/product hex IDs (sysfs style, no 0x).
type USBID struct {
	Vendor  string
	Product string
}

// DefaultFM350 is the Fibocom FM350-GL identity used by this project.
var DefaultFM350 = USBID{
	Vendor:  "0e8d",
	Product: "7127",
}
