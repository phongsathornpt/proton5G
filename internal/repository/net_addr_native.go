package repository

import "net"

// NetIfaceAddrsNative returns IPv4 addresses using the Go net package only.
// It is intended for hot status-poll paths where spawning `ip` every sample is
// unnecessarily expensive and can block the service state lock.
func NetIfaceAddrsNative(iface string) []string {
	if !validIfaceName(iface) {
		return nil
	}
	dev, err := net.InterfaceByName(iface)
	if err != nil {
		return nil
	}
	addrs, err := dev.Addrs()
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{}, len(addrs))
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			host, _, splitErr := net.SplitHostPort(addr.String())
			if splitErr == nil {
				ip = net.ParseIP(host)
			} else {
				ip = net.ParseIP(addr.String())
			}
		}
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		s := ip4.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
