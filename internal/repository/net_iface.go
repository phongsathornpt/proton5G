package repository

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// NetLinkUp brings a network interface administratively up.
func NetLinkUp(iface string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	return runCmd(5*time.Second, "ip", "link", "set", "dev", iface, "up")
}

// NetLinkDown brings a network interface down.
func NetLinkDown(iface string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	return runCmd(5*time.Second, "ip", "link", "set", "dev", iface, "down")
}

// NetDHCP tries dhclient then dhcpcd to obtain an address.
func NetDHCP(iface string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	// Prefer dhclient -v for readable output; fall back to dhcpcd.
	if _, err := exec.LookPath("dhclient"); err == nil {
		// Release first to avoid stuck leases, ignore errors.
		_, _ = runCmd(8*time.Second, "dhclient", "-r", iface)
		out, err := runCmd(30*time.Second, "dhclient", "-v", iface)
		return out, err
	}
	if _, err := exec.LookPath("dhcpcd"); err == nil {
		return runCmd(30*time.Second, "dhcpcd", "-n", iface)
	}
	return "", fmt.Errorf("no DHCP client found (install isc-dhcp-client or dhcpcd)")
}

// ConnectRNDIS brings the RNDIS/net iface up and runs DHCP.
func ConnectRNDIS(iface string) (string, error) {
	upOut, err := NetLinkUp(iface)
	if err != nil {
		return upOut, fmt.Errorf("ip link up: %w", err)
	}
	dhcpOut, err := NetDHCP(iface)
	combined := strings.TrimSpace(upOut + "\n" + dhcpOut)
	if err != nil {
		return combined, fmt.Errorf("dhcp: %w", err)
	}
	return combined, nil
}

// ConnectRNDISStatic applies explicit host network parameters. The address must
// include its CIDR prefix; this helper never guesses a prefix or gateway.
func ConnectRNDISStatic(iface, addrCIDR, gateway string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	addrCIDR = strings.TrimSpace(addrCIDR)
	slash := strings.Index(addrCIDR, "/")
	if slash <= 0 || slash == len(addrCIDR)-1 {
		return "", fmt.Errorf("static address must include an explicit CIDR prefix")
	}
	ip := addrCIDR[:slash]
	if !isIPv4Addr(ip) || ip == "0.0.0.0" {
		return "", fmt.Errorf("invalid IPv4 address %q", ip)
	}
	prefix, err := strconv.Atoi(addrCIDR[slash+1:])
	if err != nil || prefix < 0 || prefix > 32 {
		return "", fmt.Errorf("invalid IPv4 prefix in %q", addrCIDR)
	}
	gateway = strings.TrimSpace(gateway)
	if gateway != "" && !isIPv4Addr(gateway) {
		return "", fmt.Errorf("invalid IPv4 gateway %q", gateway)
	}

	upOut, err := NetLinkUp(iface)
	if err != nil {
		return upOut, fmt.Errorf("ip link up: %w", err)
	}
	parts := []string{upOut}

	o, addErr := runCmd(5*time.Second, "ip", "addr", "replace", addrCIDR, "dev", iface)
	parts = append(parts, o)
	if addErr != nil {
		return strings.TrimSpace(strings.Join(parts, "\n")), fmt.Errorf("ip addr replace: %w", addErr)
	}

	if gateway != "" {
		o, routeErr := runCmd(5*time.Second, "ip", "route", "replace", "default", "via", gateway, "dev", iface, "metric", "100")
		parts = append(parts, o)
		if routeErr != nil {
			return strings.TrimSpace(strings.Join(parts, "\n")), fmt.Errorf("default route: %w", routeErr)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

// DisconnectRNDIS releases DHCP if possible, flushes addresses, and sets link down.
func DisconnectRNDIS(iface string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	var parts []string
	if _, err := exec.LookPath("dhclient"); err == nil {
		o, _ := runCmd(8*time.Second, "dhclient", "-r", iface)
		parts = append(parts, o)
	}
	if _, err := exec.LookPath("ip"); err == nil {
		o, _ := runCmd(5*time.Second, "ip", "addr", "flush", "dev", iface)
		parts = append(parts, o)
	}
	o, err := NetLinkDown(iface)
	parts = append(parts, o)
	return strings.TrimSpace(strings.Join(parts, "\n")), err
}

// NetIfaceCounters reads rx/tx byte counters from sysfs. Zero if unknown.
func NetIfaceCounters(iface string) (rx, tx uint64) {
	if !validIfaceName(iface) {
		return 0, 0
	}
	rx = readSysUint("/sys/class/net/" + iface + "/statistics/rx_bytes")
	tx = readSysUint("/sys/class/net/" + iface + "/statistics/tx_bytes")
	return rx, tx
}

func readSysUint(path string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func validIfaceName(s string) bool {
	if s == "" || len(s) > 15 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// NetIfaceAddrs returns IPv4 addresses currently configured on iface (via `ip`).
// Empty slice if unknown / down / no tool.
func NetIfaceAddrs(iface string) []string {
	if !validIfaceName(iface) {
		return nil
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return nil
	}
	out, err := runCmd(3*time.Second, "ip", "-4", "-o", "addr", "show", "dev", iface)
	if err != nil || out == "" {
		return nil
	}
	// Example: "2: enx…    inet 10.0.0.2/24 brd … scope global …"
	var addrs []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "inet" {
				a := fields[i+1]
				if slash := strings.Index(a, "/"); slash > 0 {
					a = a[:slash]
				}
				if a != "" {
					addrs = append(addrs, a)
				}
			}
		}
	}
	return addrs
}

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		out := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if err != nil {
			return out, fmt.Errorf("%s: %w: %s", name, err, out)
		}
		return out, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("%s timed out after %s", name, timeout)
	}
}
