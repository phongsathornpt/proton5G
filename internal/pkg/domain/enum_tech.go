package domain

import "strings"

// RadioTech is the active radio access technology label.
type RadioTech string

const (
	TechUnknown RadioTech = "Unknown"
	Tech5GNR    RadioTech = "5G NR"
	Tech5GSA    RadioTech = "5G SA"
	Tech5GNSA   RadioTech = "5G NSA (EN-DC)"
	Tech5GENDC  RadioTech = "5G NSA (EN-DC)" // Synonym for Tech5GNSA
	TechLTE     RadioTech = "LTE"
	TechUMTS    RadioTech = "UMTS"
)

// RadioTechFromCellRAT maps Fibocom AT+GTCCINFO RAT codes.
// 2=UMTS, 4/7=LTE, 8/9/11=NR (vendor tools use 9 for NR).
func RadioTechFromCellRAT(code string) RadioTech {
	switch strings.TrimSpace(code) {
	case "2":
		return TechUMTS
	case "4", "7":
		return TechLTE
	case "8", "9", "11":
		return Tech5GNR
	default:
		return TechUnknown
	}
}
