package domain

// RadioTech is the active radio access technology label.
type RadioTech string

const (
	TechUnknown RadioTech = "Unknown"
	Tech5GNR    RadioTech = "5G NR"
	TechLTE     RadioTech = "LTE"
)
