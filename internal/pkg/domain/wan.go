package domain

// WANIPConfig is host network configuration reported by a data protocol such as
// MBIM. AddressCIDR must include the network prefix; callers must not infer one.
type WANIPConfig struct {
	AddressCIDR string   `json:"address_cidr,omitempty"`
	Gateway     string   `json:"gateway,omitempty"`
	DNS         []string `json:"dns,omitempty"`
	MTU         int      `json:"mtu,omitempty"`
}

func (c WANIPConfig) Valid() bool {
	return c.AddressCIDR != ""
}
