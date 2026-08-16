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
	args := []string{"dns", iface}
	args = append(args, servers...)
	return runCmd(5*time.Second, "resolvectl", args...)
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
