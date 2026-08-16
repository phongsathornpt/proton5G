package repository

import "fmt"

// NetRepo adapts modem host-network helpers to the usecase layer.
type NetRepo struct{}

func NewNetRepo() *NetRepo { return &NetRepo{} }

func (n *NetRepo) ConnectRNDIS(iface string) (string, error) {
	if err := ValidateFM350RNDISIface(iface); err != nil {
		return "", err
	}
	return ConnectRNDIS(iface)
}

func (n *NetRepo) ConnectRNDISStatic(iface, addrCIDR, gateway string) (string, error) {
	if err := ValidateFM350RNDISIface(iface); err != nil {
		return "", err
	}
	// AT+CGPADDR only reports the PDP address. It does not provide a trustworthy
	// subnet prefix or default gateway, so the service must never manufacture
	// /24 + .1 values and paint them onto the host interface.
	return "", fmt.Errorf("automatic static RNDIS configuration is disabled: modem PDP telemetry does not include a subnet prefix and gateway")
}

// ConfigureStatic applies explicit protocol-provided host parameters (for
// example, MBIM IP configuration) to any FM350-owned host net interface.
func (n *NetRepo) ConfigureStatic(iface, addrCIDR, gateway string) (string, error) {
	if err := ValidateFM350NetIface(iface); err != nil {
		return "", err
	}
	return ConnectRNDISStatic(iface, addrCIDR, gateway)
}

func (n *NetRepo) ConfigureDNS(iface string, servers []string) (string, error) {
	if err := ValidateFM350NetIface(iface); err != nil {
		return "", err
	}
	return ConfigureNetDNS(iface, servers)
}

func (n *NetRepo) ClearInterface(iface string) (string, error) {
	if err := ValidateFM350NetIface(iface); err != nil {
		return "", err
	}
	return ClearNetInterface(iface)
}

func (n *NetRepo) DisconnectRNDIS(iface string) (string, error) {
	if err := ValidateFM350RNDISIface(iface); err != nil {
		return "", err
	}
	return DisconnectRNDIS(iface)
}

func (n *NetRepo) IfaceAddrs(iface string) []string {
	return NetIfaceAddrsNative(iface)
}

func (n *NetRepo) IfaceCounters(iface string) (rx, tx uint64) {
	return NetIfaceCounters(iface)
}
