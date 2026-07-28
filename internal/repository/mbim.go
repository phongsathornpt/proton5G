package repository

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Available reports whether mbimcli is on PATH.
func Available() bool {
	_, err := exec.LookPath("mbimcli")
	return err == nil
}

// InstallHint returns a distro-oriented install command when mbimcli is missing.
func InstallHint() string {
	if Available() {
		return ""
	}
	// Prefer apt on Debian/Ubuntu; still useful as generic guidance elsewhere.
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "sudo apt-get install -y libmbim-utils"
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return "sudo dnf install -y libmbim"
	}
	if _, err := exec.LookPath("pacman"); err == nil {
		return "sudo pacman -S libmbim"
	}
	return "install package providing mbimcli (e.g. libmbim-utils)"
}

// FindDevice returns the first existing cdc-wdm node, or empty string.
func FindDevice() string {
	for _, d := range DefaultCDCWdm {
		if st, err := os.Stat(d); err == nil && !st.IsDir() {
			return d
		}
	}
	matches, _ := filepath.Glob(CDCWdmGlob)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// Connect attempts an MBIM connect via mbimcli when available.
// apn may be empty (operator default). device may be empty (auto-detect).
func errMbimcliMissing() error {
	return fmt.Errorf("mbimcli not found in PATH — %s", InstallHint())
}

func Connect(device, apn string) (string, error) {
	if !Available() {
		return "", errMbimcliMissing()
	}
	if device == "" {
		device = FindDevice()
	}
	if device == "" {
		return "", ConnectErrNoDevice()
	}

	args := []string{"-d", device, "--no-open=no", "--no-close=no"}
	if strings.TrimSpace(apn) == "" {
		args = append(args, "--simple-connect")
	} else {
		args = append(args, fmt.Sprintf("--simple-connect=apn='%s'", apn))
	}

	return runMbimcli(8*time.Second, args...)
}

// Disconnect tears down the MBIM session.
func Disconnect(device string) (string, error) {
	if !Available() {
		return "", errMbimcliMissing()
	}
	if device == "" {
		device = FindDevice()
	}
	if device == "" {
		return "", ConnectErrNoDevice()
	}
	return runMbimcli(5*time.Second, "-d", device, "--no-open=no", "--no-close=no", "--disconnect")
}

// QueryIP returns IP configuration output from mbimcli when possible.
func QueryIP(device string) (string, error) {
	if !Available() {
		return "", errMbimcliMissing()
	}
	if device == "" {
		device = FindDevice()
	}
	if device == "" {
		return "", ConnectErrNoDevice()
	}
	return runMbimcli(5*time.Second, "-d", device, "--no-open=no", "--no-close=no", "--query-ip-configuration")
}

// Status summarizes helper readiness without requiring a live modem.
func Status() map[string]any {
	devs := ListMBIMDevices()
	dev := ""
	if len(devs) > 0 {
		dev = devs[0]
	}
	st := map[string]any{
		"mbimcli_available": Available(),
		"device":            dev,
		"devices":           devs,
		"device_present":    len(devs) > 0,
	}
	if !Available() {
		st["install_hint"] = InstallHint()
	}
	if Available() && len(devs) == 0 {
		st["note"] = "No /dev/cdc-wdm* — modem may not be in MBIM mode; AT control can still work."
	}
	return st
}

// ConnectErrNoDevice is the user-facing error when no MBIM node exists.
func ConnectErrNoDevice() error {
	return fmt.Errorf("no /dev/cdc-wdm* device found — modem may be AT-only; select an AT port for monitoring, or switch USB composition to MBIM")
}

func runMbimcli(timeout time.Duration, args ...string) (string, error) {
	cmd := exec.Command("mbimcli", args...)
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
			return out, fmt.Errorf("mbimcli: %w: %s", err, out)
		}
		return out, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("mbimcli timed out after %s", timeout)
	}
}
