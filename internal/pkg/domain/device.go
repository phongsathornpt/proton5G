package domain

import "strings"

// USBID identifies a USB device by vendor/product hex IDs (sysfs style, no 0x).
type USBID struct {
	Vendor  string
	Product string
}

const FM350Vendor = "0e8d"

// Documented / observed FM350-GL USB product IDs.
// 7127 is the usual RNDIS composition; 7126 is the same module on many
// USB 3 / OEM sticks (this host: Bus 002 Dev, sysfs 2-1.4).
const (
	FM350Product7127 = "7127"
	FM350Product7126 = "7126"
)

// FM350Products is every PID treated as an FM350-GL.
var FM350Products = []string{FM350Product7127, FM350Product7126}

// DefaultFM350 is the primary identity (7127). Matching accepts all FM350Products.
var DefaultFM350 = USBID{
	Vendor:  FM350Vendor,
	Product: FM350Product7127,
}

// NormalizeUSBHex lowercases a sysfs/lsusb hex id and strips 0x.
func NormalizeUSBHex(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "0x")
	return s
}

// IsFM350 reports whether vendor:product is a known FM350-GL USB id.
func IsFM350(vendor, product string) bool {
	if NormalizeUSBHex(vendor) != FM350Vendor {
		return false
	}
	p := NormalizeUSBHex(product)
	for _, k := range FM350Products {
		if p == k {
			return true
		}
	}
	return false
}

// MatchFM350Filter matches a sysfs device against a requested vendor/product.
// Empty or "*" product, or any known FM350 PID, accepts every FM350Products sibling.
func MatchFM350Filter(filterVendor, filterProduct, gotVendor, gotProduct string) bool {
	fv := NormalizeUSBHex(filterVendor)
	fp := NormalizeUSBHex(filterProduct)
	gv := NormalizeUSBHex(gotVendor)
	gp := NormalizeUSBHex(gotProduct)
	if fv == "" {
		fv = FM350Vendor
	}
	if gv != fv {
		return false
	}
	if fp == "" || fp == "*" {
		return IsFM350(gv, gp)
	}
	if fp == gp {
		return true
	}
	return IsFM350(fv, fp) && IsFM350(gv, gp)
}
