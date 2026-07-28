package repository

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// ListCandidatePorts returns serial device paths that may be the FM350 AT interface.
// USB VID/PID matches are listed first (sorted), then remaining /dev/ttyUSB* paths.
func ListCandidatePorts(vendor, product string) ([]string, error) {
	seen := make(map[string]struct{})
	var usbMatches []string
	var others []string

	targetVendor := strings.ToUpper(vendor)
	targetProduct := strings.ToUpper(product)

	ports, err := enumerator.GetDetailedPortsList()
	if err == nil {
		for _, p := range ports {
			if !p.IsUSB || p.Name == "" {
				continue
			}
			if strings.ToUpper(p.VID) == targetVendor && strings.ToUpper(p.PID) == targetProduct {
				if _, ok := seen[p.Name]; !ok {
					seen[p.Name] = struct{}{}
					usbMatches = append(usbMatches, p.Name)
				}
			}
		}
	}

	matches, err := filepath.Glob(TTYUSBGlob)
	if err == nil {
		for _, m := range matches {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			others = append(others, m)
		}
	}

	sort.Strings(usbMatches)
	sort.Strings(others)

	out := make([]string, 0, len(usbMatches)+len(others))
	out = append(out, usbMatches...)
	out = append(out, others...)
	return out, nil
}

// ProbeATPort opens name briefly and returns true if a basic AT command gets OK.
func ProbeATPort(name string) bool {
	if name == "" {
		return false
	}
	mode := &serial.Mode{
		BaudRate: SerialBaud,
		DataBits: SerialDataBits,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(name, mode)
	if err != nil {
		return false
	}
	defer p.Close()
	_ = p.SetReadTimeout(800 * time.Millisecond)
	_ = p.ResetInputBuffer()
	if _, err := p.Write([]byte(CmdAT + "\r")); err != nil {
		return false
	}

	var resp []byte
	buf := make([]byte, 256)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, err := p.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			if strings.Contains(string(resp), ATResultOK) {
				return true
			}
			if strings.Contains(string(resp), ATResultERROR) {
				return false
			}
		}
		if err != nil && n == 0 {
			break
		}
	}
	return false
}

// DiscoverATPort finds a working AT command port for the FM350-GL.
// Candidates are probed in parallel (bounded); the first ready path in list order wins
// (USB VID/PID matches stay preferred over generic ttyUSB*).
func DiscoverATPort(vendor, product string) (string, error) {
	candidates, err := ListCandidatePorts(vendor, product)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}

	ready := ProbeATPortsCached(candidates, "")
	for _, name := range candidates {
		if ready[name] {
			return name, nil
		}
	}

	// No port answered AT — return first candidate for later retry by status poller.
	return candidates[0], nil
}

// DiscoverATPortPrefer keeps preferred if it still answers AT; otherwise rediscovers.
func DiscoverATPortPrefer(vendor, product, preferred string) (string, error) {
	if preferred != "" {
		// Single-path check (uses cache when warm).
		if ProbeATPortCached(preferred, "") {
			return preferred, nil
		}
	}
	return DiscoverATPort(vendor, product)
}
