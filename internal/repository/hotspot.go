package repository

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
)

// HotspotManager runs hostapd + dnsmasq + NAT for a software AP.
type HotspotManager struct {
	runtimeDir string

	mu         sync.Mutex
	hostapdCmd *exec.Cmd
	dnsmasqCmd *exec.Cmd
	wlan       string
	uplink     string
	lanCIDR    string
	running    bool
}

// NewHotspotManager builds a manager writing conf under runtimeDir.
func NewHotspotManager(runtimeDir string) *HotspotManager {
	if runtimeDir == "" {
		runtimeDir = appdefaults.HotspotRuntimeDir
	}
	return &HotspotManager{runtimeDir: runtimeDir}
}

// IsRunning reports whether a start session is active.
func (h *HotspotManager) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

// StatusExtras returns current LAN/uplink addresses and best-effort clients.
func (h *HotspotManager) StatusExtras() (lanAddrs, uplinkAddrs []string, clients []domain.HotspotClient) {
	h.mu.Lock()
	wlan, uplink, runtimeDir := h.wlan, h.uplink, h.runtimeDir
	running := h.running
	h.mu.Unlock()
	if wlan != "" {
		lanAddrs = NetIfaceAddrs(wlan)
	}
	if uplink != "" {
		uplinkAddrs = NetIfaceAddrs(uplink)
	}
	if running && wlan != "" {
		leaseFile := filepath.Join(runtimeDir, "dnsmasq.leases")
		clients = ListHotspotClients(wlan, leaseFile)
	}
	return lanAddrs, uplinkAddrs, clients
}

// Start brings up the AP stack. On failure it attempts cleanup.
func (h *HotspotManager) Start(cfg domain.HotspotConfig, uplink string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return "", fmt.Errorf("hotspot already running")
	}
	tools := HotspotToolsPresent()
	if !tools.Hostapd {
		return "", fmt.Errorf("hostapd not found (install hostapd)")
	}
	if !tools.Dnsmasq {
		return "", fmt.Errorf("dnsmasq not found (install dnsmasq)")
	}
	if !tools.IP {
		return "", fmt.Errorf("ip (iproute2) not found")
	}
	if !tools.Nftables && !tools.Iptables {
		return "", fmt.Errorf("need nft or iptables for NAT")
	}
	if err := domain.ValidateIfaceName(uplink); err != nil {
		return "", fmt.Errorf("uplink: %w", err)
	}
	if addrs := NetIfaceAddrs(uplink); len(addrs) == 0 {
		return "", fmt.Errorf("uplink %s has no IPv4 address — connect data bearer first", uplink)
	}

	if err := os.MkdirAll(h.runtimeDir, 0o700); err != nil {
		return "", fmt.Errorf("runtime dir: %w", err)
	}

	var logParts []string
	appendLog := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			logParts = append(logParts, s)
		}
	}

	// Best-effort unrfkill.
	if lookPath("rfkill") {
		if o, err := runCmd(3*time.Second, "rfkill", "unblock", "wifi"); err == nil {
			appendLog(o)
		}
	}

	if o, err := NetLinkUp(cfg.WlanIface); err != nil {
		return strings.Join(logParts, "\n"), fmt.Errorf("wlan up: %w", err)
	} else {
		appendLog(o)
	}

	// Replace addresses on wlan with LAN gateway.
	_, _ = runCmd(3*time.Second, "ip", "addr", "flush", "dev", cfg.WlanIface)
	if o, err := runCmd(3*time.Second, "ip", "addr", "add", cfg.LANCIDR, "dev", cfg.WlanIface); err != nil {
		return strings.Join(logParts, "\n"), fmt.Errorf("lan addr: %w", err)
	} else {
		appendLog(o)
	}

	hostapdPath := filepath.Join(h.runtimeDir, "hostapd.conf")
	dnsmasqPath := filepath.Join(h.runtimeDir, "dnsmasq.conf")
	leasePath := filepath.Join(h.runtimeDir, "dnsmasq.leases")
	hostapdConf := GenerateHostapdConf(cfg)
	if err := os.WriteFile(hostapdPath, []byte(hostapdConf), 0o600); err != nil {
		return strings.Join(logParts, "\n"), err
	}
	dnsConf, err := GenerateDnsmasqConf(cfg, appdefaults.HotspotDHCPStart, appdefaults.HotspotDHCPEnd, leasePath)
	if err != nil {
		return strings.Join(logParts, "\n"), err
	}
	if err := os.WriteFile(dnsmasqPath, []byte(dnsConf), 0o600); err != nil {
		return strings.Join(logParts, "\n"), err
	}

	// Enable forwarding
	if o, err := runCmd(2*time.Second, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		appendLog("sysctl forward: " + err.Error())
	} else {
		appendLog(o)
	}

	if err := h.applyNATLocked(cfg.WlanIface, uplink); err != nil {
		h.cleanupPartialLocked(cfg.WlanIface)
		return strings.Join(logParts, "\n"), fmt.Errorf("nat: %w", err)
	}

	// hostapd (supervised, own process group)
	hCmd := exec.Command("hostapd", hostapdPath)
	hCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	hCmd.Stdout = nil
	hCmd.Stderr = nil
	if err := hCmd.Start(); err != nil {
		_ = h.removeNATLocked(cfg.WlanIface, uplink)
		h.cleanupPartialLocked(cfg.WlanIface)
		return strings.Join(logParts, "\n"), fmt.Errorf("hostapd start: %w", err)
	}
	// Brief settle
	time.Sleep(300 * time.Millisecond)
	if hCmd.ProcessState != nil && hCmd.ProcessState.Exited() {
		_ = h.removeNATLocked(cfg.WlanIface, uplink)
		h.cleanupPartialLocked(cfg.WlanIface)
		return strings.Join(logParts, "\n"), fmt.Errorf("hostapd exited immediately")
	}

	// dnsmasq foreground-ish: --keep-in-foreground if supported, else normal with pid
	dCmd := exec.Command("dnsmasq", "--conf-file="+dnsmasqPath, "--keep-in-foreground", "--no-resolv")
	dCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := dCmd.Start(); err != nil {
		// try without keep-in-foreground
		dCmd = exec.Command("dnsmasq", "--conf-file="+dnsmasqPath, "-d")
		dCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err2 := dCmd.Start(); err2 != nil {
			_ = killProcessGroup(hCmd)
			_ = h.removeNATLocked(cfg.WlanIface, uplink)
			h.cleanupPartialLocked(cfg.WlanIface)
			return strings.Join(logParts, "\n"), fmt.Errorf("dnsmasq start: %v / %v", err, err2)
		}
	}

	// Reap in background so Wait is not required for Stop
	go func() { _ = hCmd.Wait() }()
	go func() { _ = dCmd.Wait() }()

	h.hostapdCmd = hCmd
	h.dnsmasqCmd = dCmd
	h.wlan = cfg.WlanIface
	h.uplink = uplink
	h.lanCIDR = cfg.LANCIDR
	h.running = true
	appendLog(fmt.Sprintf("hotspot up on %s uplink %s", cfg.WlanIface, uplink))
	log.Printf("[INFO] Hotspot started on %s (uplink %s)", cfg.WlanIface, uplink)
	return strings.Join(logParts, "\n"), nil
}

// Stop tears down AP processes and NAT.
func (h *HotspotManager) Stop() (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopLocked()
}

func (h *HotspotManager) stopLocked() (string, error) {
	if !h.running && h.hostapdCmd == nil && h.dnsmasqCmd == nil {
		return "already stopped", nil
	}
	var parts []string
	if h.dnsmasqCmd != nil {
		if err := killProcessGroup(h.dnsmasqCmd); err != nil {
			parts = append(parts, "dnsmasq: "+err.Error())
		}
		h.dnsmasqCmd = nil
	}
	if h.hostapdCmd != nil {
		if err := killProcessGroup(h.hostapdCmd); err != nil {
			parts = append(parts, "hostapd: "+err.Error())
		}
		h.hostapdCmd = nil
	}
	if h.wlan != "" && h.uplink != "" {
		if err := h.removeNATLocked(h.wlan, h.uplink); err != nil {
			parts = append(parts, "nat: "+err.Error())
		}
	}
	if h.wlan != "" {
		_, _ = runCmd(3*time.Second, "ip", "addr", "flush", "dev", h.wlan)
	}
	h.running = false
	wlan, up := h.wlan, h.uplink
	h.wlan, h.uplink, h.lanCIDR = "", "", ""
	log.Printf("[INFO] Hotspot stopped (was %s / %s)", wlan, up)
	if len(parts) > 0 {
		return strings.Join(parts, "\n"), fmt.Errorf("stop with warnings: %s", strings.Join(parts, "; "))
	}
	return "stopped", nil
}

func (h *HotspotManager) cleanupPartialLocked(wlan string) {
	if wlan != "" {
		_, _ = runCmd(3*time.Second, "ip", "addr", "flush", "dev", wlan)
	}
}

func (h *HotspotManager) applyNATLocked(wlan, uplink string) error {
	if lookPath("nft") {
		// Fresh table each start
		_, _ = runCmd(3*time.Second, "nft", "delete", "table", "ip", "fm350_hotspot")
		script := fmt.Sprintf(`
table ip fm350_hotspot {
  chain postrouting {
    type nat hook postrouting priority srcnat; policy accept;
    oifname "%s" masquerade
  }
  chain forward {
    type filter hook forward priority filter; policy accept;
    ct state established,related accept
    iifname "%s" oifname "%s" accept
  }
}
`, uplink, wlan, uplink)
		cmd := exec.Command("nft", "-f", "-")
		cmd.Stdin = strings.NewReader(script)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("nft: %w: %s", err, stderr.String())
		}
		return nil
	}
	// iptables fallback with marked rules
	rules := [][]string{
		{"-t", "nat", "-A", "POSTROUTING", "-o", uplink, "-m", "comment", "--comment", "fm350_hotspot", "-j", "MASQUERADE"},
		{"-A", "FORWARD", "-m", "comment", "--comment", "fm350_hotspot", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		{"-A", "FORWARD", "-i", wlan, "-o", uplink, "-m", "comment", "--comment", "fm350_hotspot", "-j", "ACCEPT"},
	}
	for _, r := range rules {
		if _, err := runCmd(3*time.Second, "iptables", r...); err != nil {
			_ = h.removeNATLocked(wlan, uplink)
			return err
		}
	}
	return nil
}

func (h *HotspotManager) removeNATLocked(wlan, uplink string) error {
	if lookPath("nft") {
		_, err := runCmd(3*time.Second, "nft", "delete", "table", "ip", "fm350_hotspot")
		// ignore missing table
		if err != nil && !strings.Contains(err.Error(), "No such file") && !strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return err
		}
		return nil
	}
	// Best-effort delete by comment is awkward; delete exact reverse of add.
	dels := [][]string{
		{"-t", "nat", "-D", "POSTROUTING", "-o", uplink, "-m", "comment", "--comment", "fm350_hotspot", "-j", "MASQUERADE"},
		{"-D", "FORWARD", "-m", "comment", "--comment", "fm350_hotspot", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		{"-D", "FORWARD", "-i", wlan, "-o", uplink, "-m", "comment", "--comment", "fm350_hotspot", "-j", "ACCEPT"},
	}
	for _, r := range dels {
		_, _ = runCmd(3*time.Second, "iptables", r...)
	}
	return nil
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		return nil
	}
}
