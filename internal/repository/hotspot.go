package repository

import (
	"fmt"
	"log"
	"net"
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

	mu                sync.Mutex
	hostapdCmd        *exec.Cmd
	dnsmasqCmd        *exec.Cmd
	wlan              string
	uplink            string
	lanCIDR           string
	running           bool
	forwardingChanged bool
	forwardingPrev    string
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
	if !h.running {
		return false
	}
	if !processAlive(h.hostapdCmd) || !processAlive(h.dnsmasqCmd) {
		log.Printf("[WARN] Hotspot daemon exited; cleaning up stale AP state")
		_, _ = h.stopLocked()
		return false
	}
	return true
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
	if err := h.preflight(cfg, uplink); err != nil {
		return "", err
	}
	hostapdBin := resolveTool("hostapd")
	dnsmasqBin := resolveTool("dnsmasq")

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

	// Enable forwarding and remember the previous host value so Stop/rollback can restore it.
	if o, err := h.enableForwardingLocked(); err != nil {
		h.cleanupPartialLocked(cfg.WlanIface)
		return strings.Join(logParts, "\n"), fmt.Errorf("enable forwarding: %w", err)
	} else {
		appendLog(o)
	}

	if err := h.applyNATLocked(cfg.WlanIface, uplink, cfg.LANCIDR); err != nil {
		h.cleanupPartialLocked(cfg.WlanIface)
		return strings.Join(logParts, "\n"), fmt.Errorf("nat: %w", err)
	}

	// hostapd (supervised, own process group); capture stderr for diagnostics
	hostapdLog := filepath.Join(h.runtimeDir, "hostapd.log")
	hLogFile, _ := os.OpenFile(hostapdLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	hCmd := exec.Command(hostapdBin, hostapdPath)
	hCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if hLogFile != nil {
		hCmd.Stdout = hLogFile
		hCmd.Stderr = hLogFile
	}
	if err := hCmd.Start(); err != nil {
		if hLogFile != nil {
			_ = hLogFile.Close()
		}
		_ = h.removeNATLocked(cfg.WlanIface, uplink, cfg.LANCIDR)
		h.cleanupPartialLocked(cfg.WlanIface)
		return strings.Join(logParts, "\n"), fmt.Errorf("hostapd start: %w", err)
	}
	// Brief settle — ProcessState is only set after Wait; probe with signal 0.
	time.Sleep(400 * time.Millisecond)
	if hCmd.Process != nil {
		if err := hCmd.Process.Signal(syscall.Signal(0)); err != nil {
			if hLogFile != nil {
				_ = hLogFile.Close()
			}
			_ = h.removeNATLocked(cfg.WlanIface, uplink, cfg.LANCIDR)
			h.cleanupPartialLocked(cfg.WlanIface)
			return strings.Join(logParts, "\n"), fmt.Errorf("hostapd exited immediately: %s", tailFile(hostapdLog, 1200))
		}
	}
	if hLogFile != nil {
		_ = hLogFile.Close()
	}
	go func() { _ = hCmd.Wait() }()

	// dnsmasq foreground-ish: --keep-in-foreground if supported, else -d
	dnsmasqLog := filepath.Join(h.runtimeDir, "dnsmasq.log")
	dLogFile, _ := os.OpenFile(dnsmasqLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	dCmd := exec.Command(dnsmasqBin, "--conf-file="+dnsmasqPath, "--keep-in-foreground", "--no-resolv")
	dCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if dLogFile != nil {
		dCmd.Stdout = dLogFile
		dCmd.Stderr = dLogFile
	}
	if err := dCmd.Start(); err != nil {
		// try without keep-in-foreground
		if dLogFile != nil {
			_ = dLogFile.Close()
		}
		dLogFile, _ = os.OpenFile(dnsmasqLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		dCmd = exec.Command(dnsmasqBin, "--conf-file="+dnsmasqPath, "-d")
		dCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if dLogFile != nil {
			dCmd.Stdout = dLogFile
			dCmd.Stderr = dLogFile
		}
		if err2 := dCmd.Start(); err2 != nil {
			if dLogFile != nil {
				_ = dLogFile.Close()
			}
			_ = killProcessGroup(hCmd)
			_ = h.removeNATLocked(cfg.WlanIface, uplink, cfg.LANCIDR)
			h.cleanupPartialLocked(cfg.WlanIface)
			return strings.Join(logParts, "\n"), fmt.Errorf("dnsmasq start: %v / %v; %s", err, err2, tailFile(dnsmasqLog, 800))
		}
	}

	if dLogFile != nil {
		_ = dLogFile.Close()
	}
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
		if err := h.removeNATLocked(h.wlan, h.uplink, h.lanCIDR); err != nil {
			parts = append(parts, "nat: "+err.Error())
		}
	}
	if h.wlan != "" {
		_, _ = runCmd(3*time.Second, "ip", "addr", "flush", "dev", h.wlan)
	}
	if err := h.restoreForwardingLocked(); err != nil {
		parts = append(parts, "forwarding: "+err.Error())
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
	_ = h.restoreForwardingLocked()
}

func (h *HotspotManager) enableForwardingLocked() (string, error) {
	prevRaw, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return "", err
	}
	prev := strings.TrimSpace(string(prevRaw))
	h.forwardingPrev = prev
	h.forwardingChanged = false
	if prev == "1" {
		return "", nil
	}
	out, err := runCmd(2*time.Second, "sysctl", "-w", "net.ipv4.ip_forward=1")
	if err != nil {
		return out, err
	}
	h.forwardingChanged = true
	return out, nil
}

func (h *HotspotManager) restoreForwardingLocked() error {
	if !h.forwardingChanged {
		h.forwardingPrev = ""
		return nil
	}
	prev := h.forwardingPrev
	h.forwardingChanged = false
	h.forwardingPrev = ""
	if prev == "" {
		prev = "0"
	}
	_, err := runCmd(2*time.Second, "sysctl", "-w", "net.ipv4.ip_forward="+prev)
	return err
}

func hotspotLANNetwork(lanCIDR string) (string, error) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(lanCIDR))
	if err != nil || network == nil || network.IP.To4() == nil {
		return "", fmt.Errorf("invalid hotspot LAN CIDR %q", lanCIDR)
	}
	return network.String(), nil
}

func (h *HotspotManager) applyNATLocked(wlan, uplink, lanCIDR string) error {
	lanNetwork, err := hotspotLANNetwork(lanCIDR)
	if err != nil {
		return err
	}
	if lookPath("nft") {
		// Fresh table each start. Rules are scoped to this AP subnet and interfaces.
		_, _ = runCmd(3*time.Second, "nft", "delete", "table", "ip", "fm350_hotspot")
		script := fmt.Sprintf(`
table ip fm350_hotspot {
  chain postrouting {
    type nat hook postrouting priority srcnat; policy accept;
    ip saddr %s oifname "%s" masquerade
  }
  chain forward {
    type filter hook forward priority filter; policy accept;
    iifname "%s" oifname "%s" ip saddr %s accept
    iifname "%s" oifname "%s" ip daddr %s ct state established,related accept
  }
}
`, lanNetwork, uplink, wlan, uplink, lanNetwork, uplink, wlan, lanNetwork)
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
		{"-t", "nat", "-A", "POSTROUTING", "-s", lanNetwork, "-o", uplink, "-m", "comment", "--comment", "fm350_hotspot", "-j", "MASQUERADE"},
		{"-A", "FORWARD", "-i", wlan, "-o", uplink, "-s", lanNetwork, "-m", "comment", "--comment", "fm350_hotspot", "-j", "ACCEPT"},
		{"-A", "FORWARD", "-i", uplink, "-o", wlan, "-d", lanNetwork, "-m", "comment", "--comment", "fm350_hotspot", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}
	for _, r := range rules {
		if _, err := runCmd(3*time.Second, "iptables", r...); err != nil {
			_ = h.removeNATLocked(wlan, uplink, lanCIDR)
			return err
		}
	}
	return nil
}

func (h *HotspotManager) removeNATLocked(wlan, uplink, lanCIDR string) error {
	if lookPath("nft") {
		_, err := runCmd(3*time.Second, "nft", "delete", "table", "ip", "fm350_hotspot")
		// ignore missing table
		if err != nil && !strings.Contains(err.Error(), "No such file") && !strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return err
		}
		return nil
	}
	lanNetwork, err := hotspotLANNetwork(lanCIDR)
	if err != nil {
		return err
	}
	// Delete the exact scoped rules added by applyNATLocked.
	dels := [][]string{
		{"-t", "nat", "-D", "POSTROUTING", "-s", lanNetwork, "-o", uplink, "-m", "comment", "--comment", "fm350_hotspot", "-j", "MASQUERADE"},
		{"-D", "FORWARD", "-i", wlan, "-o", uplink, "-s", lanNetwork, "-m", "comment", "--comment", "fm350_hotspot", "-j", "ACCEPT"},
		{"-D", "FORWARD", "-i", uplink, "-o", wlan, "-d", lanNetwork, "-m", "comment", "--comment", "fm350_hotspot", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}
	for _, r := range dels {
		_, _ = runCmd(3*time.Second, "iptables", r...)
	}
	return nil
}

// preflight validates tools, wlan, and uplink before mutating state.
func (h *HotspotManager) preflight(cfg domain.HotspotConfig, uplink string) error {
	tools := HotspotToolsPresent()
	if !tools.Hostapd {
		hint := HotspotInstallHint(tools)
		return fmt.Errorf("hostapd not found (%s)", hint)
	}
	if !tools.Dnsmasq {
		return fmt.Errorf("dnsmasq not found (%s)", HotspotInstallHint(tools))
	}
	if !tools.IP {
		return fmt.Errorf("ip (iproute2) not found")
	}
	if !tools.Nftables && !tools.Iptables {
		return fmt.Errorf("need nft or iptables for NAT (%s)", HotspotInstallHint(tools))
	}
	if err := domain.ValidateIfaceName(cfg.WlanIface); err != nil {
		return fmt.Errorf("wlan_iface: %w", err)
	}
	if !isWirelessIface(cfg.WlanIface) {
		return fmt.Errorf("wlan %s is not a wireless interface", cfg.WlanIface)
	}
	if addrs := NetIfaceAddrs(cfg.WlanIface); len(addrs) > 0 {
		return fmt.Errorf("wlan %s already has IPv4 address %s; disconnect/unmanage it before starting hotspot", cfg.WlanIface, strings.Join(addrs, ", "))
	}
	if tools.Iw {
		phy := wirelessPhy(cfg.WlanIface)
		_, ap := wifiAPModes(phy, cfg.WlanIface)
		if !ap {
			return fmt.Errorf("wlan %s does not advertise AP mode (iw); cannot start software AP", cfg.WlanIface)
		}
	}
	if err := domain.ValidateIfaceName(uplink); err != nil {
		return fmt.Errorf("uplink: %w", err)
	}
	if addrs := NetIfaceAddrs(uplink); len(addrs) == 0 {
		return fmt.Errorf("uplink %s has no IPv4 address — connect data bearer (RNDIS) first", uplink)
	}
	return nil
}

func tailFile(path string, max int) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return "(no log)"
	}
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		s = s[len(s)-max:]
	}
	return s
}

func processAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return false
	}
	return cmd.Process.Signal(syscall.Signal(0)) == nil
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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(cmd) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Kill()
	}
	return nil
}
