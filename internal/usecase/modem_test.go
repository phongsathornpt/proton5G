package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

type fakeUSB struct {
	status domain.ModemStatus
	resets int
}

func (f *fakeUSB) Check(string) domain.ModemStatus { return f.status }
func (f *fakeUSB) DisableAutosuspend() error       { return nil }
func (f *fakeUSB) HardReset(string) error {
	f.resets++
	return nil
}

type fakeAT struct {
	port  string
	failN int
	calls int
	sig   domain.SignalInfo
	net   domain.NetworkInfo
	closed int
}

func (f *fakeAT) PortName() string                     { return f.port }
func (f *fakeAT) SetPortName(n string)                 { f.port = n }
func (f *fakeAT) Close() error                         { f.closed++; return nil }
func (f *fakeAT) Connect() error                       { return nil }
func (f *fakeAT) EnsureConnected() error               { return nil }
func (f *fakeAT) SetAPN(int, domain.PDPType, string) error { return nil }
func (f *fakeAT) SetRATMode(domain.RATModePref) error  { return nil }
func (f *fakeAT) SendRaw(string) (string, error)       { return "OK", nil }
func (f *fakeAT) GetFullStatus() (domain.SignalInfo, domain.NetworkInfo, domain.SIMInfo, domain.APNConfig, domain.RATMode, error) {
	f.calls++
	if f.calls <= f.failN {
		return domain.SignalInfo{}, domain.NetworkInfo{}, domain.SIMInfo{}, domain.APNConfig{}, "", errors.New("at fail")
	}
	return f.sig, f.net, domain.SIMInfo{State: domain.SIMReady}, domain.APNConfig{APN: "internet", PDPType: domain.PDPIPV4V6}, domain.RATModeAuto, nil
}

type fakeHist struct {
	n int
}

func (f *fakeHist) Add(domain.SignalSample)         { f.n++ }
func (f *fakeHist) Snapshot() []domain.SignalSample { return nil }
func (f *fakeHist) LoadFile(string) error           { return nil }
func (f *fakeHist) SaveFile(string) error           { return nil }

type fakeDiscover struct {
	port string
}

func (f *fakeDiscover) DiscoverATPort(string, string) (string, error) {
	return f.port, nil
}

func TestStatusDisconnected(t *testing.T) {
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: false}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
	})
	st := svc.Status()
	if st.Error != "modem disconnected" {
		t.Fatalf("got %q", st.Error)
	}
}

func TestStatusSuccessSamplesHistory(t *testing.T) {
	hist := &fakeHist{}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT: &fakeAT{
			port: "/dev/ttyUSB2",
			sig:  domain.SignalInfo{RSSI: -70, Percentage: 50},
			net:  domain.NetworkInfo{Tech: domain.TechLTE},
		},
		History: hist,
	})
	st := svc.Status()
	if st.Signal.Percentage != 50 || st.Modem.PortPath != "/dev/ttyUSB2" {
		t.Fatalf("unexpected status: %+v", st)
	}
	if hist.n != 1 {
		t.Fatalf("expected history sample, got %d", hist.n)
	}
}

func TestStatusRediscoverThenOK(t *testing.T) {
	at := &fakeAT{
		port:  "/dev/ttyUSB0",
		failN: 1,
		sig:   domain.SignalInfo{RSSI: -80, Percentage: 30},
	}
	svc := NewModemService(ModemServiceConfig{
		USB:      &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:       at,
		Discover: &fakeDiscover{port: "/dev/ttyUSB2"},
		Vendor:   domain.DefaultFM350.Vendor,
		Product:  domain.DefaultFM350.Product,
	})
	st := svc.Status()
	if st.Error != "" {
		t.Fatalf("expected recovery, got error %q", st.Error)
	}
	if at.port != "/dev/ttyUSB2" {
		t.Fatalf("expected rediscover switch, port=%s", at.port)
	}
	if at.closed < 1 {
		t.Fatal("expected Close before rediscover")
	}
}

func TestHardResetAfterStreak(t *testing.T) {
	usb := &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}}
	at := &fakeAT{port: "/dev/ttyUSB0", failN: 100}
	svc := NewModemService(ModemServiceConfig{
		USB:      usb,
		AT:       at,
		Discover: &fakeDiscover{port: "/dev/ttyUSB0"},
	})
	svc.resetCooldown = time.Millisecond
	svc.rediscoverEvery = time.Hour // force streak without extra rediscover noise
	for i := 0; i < 3; i++ {
		_ = svc.Status()
	}
	if usb.resets < 1 {
		t.Fatalf("expected hard reset after fail streak, resets=%d", usb.resets)
	}
}

func TestPermissionDeniedSkipsResetAndRediscoverSpam(t *testing.T) {
	usb := &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}}
	permAT := &fakeATPerm{port: "/dev/ttyUSB0"}
	svc := NewModemService(ModemServiceConfig{
		USB:      usb,
		AT:       permAT,
		Discover: &fakeDiscover{port: "/dev/ttyUSB0"},
	})
	for i := 0; i < 5; i++ {
		st := svc.Status()
		if !strings.Contains(strings.ToLower(st.Error), "permission") {
			t.Fatalf("expected permission guidance, got %q", st.Error)
		}
		if !strings.Contains(st.Error, "dialout") {
			t.Fatalf("expected dialout hint, got %q", st.Error)
		}
	}
	if usb.resets != 0 {
		t.Fatalf("hard reset must not run on permission denied, resets=%d", usb.resets)
	}
	if permAT.closed != 0 {
		t.Fatalf("should not close/rediscover on permission denied, closed=%d", permAT.closed)
	}
}

type fakeATPerm struct {
	port   string
	closed int
}

func (f *fakeATPerm) PortName() string                         { return f.port }
func (f *fakeATPerm) SetPortName(n string)                     { f.port = n }
func (f *fakeATPerm) Close() error                             { f.closed++; return nil }
func (f *fakeATPerm) Connect() error                           { return nil }
func (f *fakeATPerm) EnsureConnected() error                   { return nil }
func (f *fakeATPerm) SetAPN(int, domain.PDPType, string) error { return nil }
func (f *fakeATPerm) SetRATMode(domain.RATModePref) error      { return nil }
func (f *fakeATPerm) SendRaw(string) (string, error) {
	return "", errors.New("permission denied")
}
func (f *fakeATPerm) GetFullStatus() (domain.SignalInfo, domain.NetworkInfo, domain.SIMInfo, domain.APNConfig, domain.RATMode, error) {
	return domain.SignalInfo{}, domain.NetworkInfo{}, domain.SIMInfo{}, domain.APNConfig{}, "",
		errors.New("open serial port /dev/ttyUSB0: Permission denied")
}

func TestSetAPNInvalidPDP(t *testing.T) {
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
	})
	err := svc.SetAPN(domain.APNConfig{APN: "x", PDPType: domain.PDPType("PPP")})
	if err == nil {
		t.Fatal("expected invalid PDP error")
	}
}
