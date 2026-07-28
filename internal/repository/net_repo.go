package repository

// NetRepo adapts RNDIS link helpers to the usecase NetRepository port.
type NetRepo struct{}

func NewNetRepo() *NetRepo { return &NetRepo{} }

func (n *NetRepo) ConnectRNDIS(iface string) (string, error) {
	return ConnectRNDIS(iface)
}

func (n *NetRepo) DisconnectRNDIS(iface string) (string, error) {
	return DisconnectRNDIS(iface)
}

func (n *NetRepo) IfaceAddrs(iface string) []string {
	return NetIfaceAddrs(iface)
}
