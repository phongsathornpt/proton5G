package repository

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// NetLinkUp brings a network interface administratively up.
func NetLinkUp(iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("network interface name is empty")
	}
	return runCmd(5*time.Second, "ip", "link", "set", "dev", iface, "up")
}

// NetLinkDown brings a network interface down.
func NetLinkDown(iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("network interface name is empty")
	}
	return runCmd(5*time.Second, "ip", "link", "set", "dev", iface, "down")
}

// NetDHCP tries dhclient then dhcpcd to obtain an address.
func NetDHCP(iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("network interface name is empty")
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

// DisconnectRNDIS releases DHCP if possible and sets link down.
func DisconnectRNDIS(iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("network interface name is empty")
	}
	var parts []string
	if _, err := exec.LookPath("dhclient"); err == nil {
		o, _ := runCmd(8*time.Second, "dhclient", "-r", iface)
		parts = append(parts, o)
	}
	o, err := NetLinkDown(iface)
	parts = append(parts, o)
	return strings.TrimSpace(strings.Join(parts, "\n")), err
}

// NetIfaceAddrs returns IPv4 addresses currently configured on iface (via `ip`).
// Empty slice if unknown / down / no tool.
func NetIfaceAddrs(iface string) []string {
	if iface == "" {
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
