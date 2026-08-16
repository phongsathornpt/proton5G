package repository

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func ConfigureNetDNS(iface string, servers []string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	if len(servers) == 0 {
		return "", nil
	}
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return "", fmt.Errorf("resolvectl not found; DNS servers were reported but not applied")
	}

	var parts []string
	args := []string{"dns", iface}
	args = append(args, servers...)
	out, err := runCmd(5*time.Second, "resolvectl", args...)
	parts = append(parts, out)
	if err != nil {
		return strings.TrimSpace(strings.Join(parts, "\n")), fmt.Errorf("resolvectl dns: %w", err)
	}

	// Make this WAN link eligible for default DNS routing. Merely attaching DNS
	// servers to a link is not sufficient on every systemd-resolved setup.
	out, err = runCmd(5*time.Second, "resolvectl", "default-route", iface, "yes")
	parts = append(parts, out)
	if err != nil {
		return strings.TrimSpace(strings.Join(parts, "\n")), fmt.Errorf("resolvectl default-route: %w", err)
	}
	out, err = runCmd(5*time.Second, "resolvectl", "domain", iface, "~.")
	parts = append(parts, out)
	if err != nil {
		return strings.TrimSpace(strings.Join(parts, "\n")), fmt.Errorf("resolvectl domain: %w", err)
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

func ClearNetInterface(iface string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name %q", iface)
	}
	var parts []string
	if _, err := exec.LookPath("ip"); err == nil {
		o, flushErr := runCmd(5*time.Second, "ip", "addr", "flush", "dev", iface)
		parts = append(parts, o)
		if flushErr != nil {
			return strings.TrimSpace(strings.Join(parts, "\n")), fmt.Errorf("ip addr flush: %w", flushErr)
		}
	}
	if _, err := exec.LookPath("resolvectl"); err == nil {
		o, _ := runCmd(5*time.Second, "resolvectl", "revert", iface)
		parts = append(parts, o)
	}
	o, err := NetLinkDown(iface)
	parts = append(parts, o)
	return strings.TrimSpace(strings.Join(parts, "\n")), err
}
