package repository

import (
	"fmt"
	"net"
	"strings"
)

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
	if err := validateFM350RNDISStaticConfig(addrCIDR, gateway); err != nil {
		return "", err
	}
	return ConnectRNDISStatic(iface, addrCIDR, gateway)
}

// validateFM350RNDISStaticConfig deliberately accepts only the FM350 RNDIS
// convention used by the modem's rndis_host composition: PDP IPv4 on /24 with
// the first address of that /24 as the peer/default gateway. This prevents the
// generic API path from painting arbitrary static routes onto host interfaces.
func validateFM350RNDISStaticConfig(addrCIDR, gateway string) error {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(addrCIDR))
	if err != nil || ip.To4() == nil {
		return fmt.Errorf("invalid FM350 RNDIS IPv4 CIDR %q", addrCIDR)
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones != 24 {
		return fmt.Errorf("FM350 RNDIS address must use /24, got %q", addrCIDR)
	}
	v4 := ip.To4()
	wantGateway := net.IPv4(v4[0], v4[1], v4[2], 1).To4()
	gotGateway := net.ParseIP(strings.TrimSpace(gateway)).To4()
	if gotGateway == nil || !gotGateway.Equal(wantGateway) {
		return fmt.Errorf("FM350 RNDIS gateway must be %s for %s", wantGateway.String(), addrCIDR)
	}
	if v4.Equal(gotGateway) {
		return fmt.Errorf("FM350 RNDIS host address cannot equal gateway %s", gotGateway.String())
	}
	return nil
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
