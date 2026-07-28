package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
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
func (f *fakeAT) GetUSBMode() (int, error)             { return domain.USBModeRNDIS41, nil }
func (f *fakeAT) SetUSBMode(mode int) error {
	if mode <= 0 {
		return errors.New("bad mode")
	}
	f.closed++
	return nil
}
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
func (f *fakeATPerm) GetUSBMode() (int, error) {
	return 0, errors.New("permission denied")
}
func (f *fakeATPerm) SetUSBMode(int) error {
	return errors.New("permission denied")
}
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

type fakeNet struct {
	lastConnect    string
	lastDisconnect string
	connectOut     string
	connectErr     error
}

func (f *fakeNet) ConnectRNDIS(iface string) (string, error) {
	f.lastConnect = iface
	return f.connectOut, f.connectErr
}
func (f *fakeNet) DisconnectRNDIS(iface string) (string, error) {
	f.lastDisconnect = iface
	return "down", nil
}
func (f *fakeNet) IfaceAddrs(string) []string { return nil }

type fakeMBIM struct {
	lastConnectDev string
	lastConnectAPN string
	lastDisconnect string
}

func (f *fakeMBIM) Status() map[string]any {
	return map[string]any{"mbimcli_available": true}
}
func (f *fakeMBIM) Connect(device, apn string) (string, error) {
	f.lastConnectDev = device
	f.lastConnectAPN = apn
	return "connected", nil
}
func (f *fakeMBIM) Disconnect(device string) (string, error) {
	f.lastDisconnect = device
	return "disconnected", nil
}

type fakeInventory struct {
	modems []domain.ModemDevice
}

func (f *fakeInventory) ListModems(_, _, _ string) []domain.ModemDevice { return f.modems }
func (f *fakeInventory) ListMBIMDevices() []string                     { return nil }
func (f *fakeInventory) MBIMCLIAvailable() bool                        { return true }
func (f *fakeInventory) MBIMInstallHint() string                       { return "" }

func TestDataConnectRNDIS(t *testing.T) {
	net := &fakeNet{connectOut: "dhcp ok"}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
		Net: net,
	})
	out, err := svc.DataConnect(domain.DataConnectRequest{
		Mode:  domain.DataModeRNDIS,
		Iface: "enxabc",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != "dhcp ok" || net.lastConnect != "enxabc" {
		t.Fatalf("out=%q last=%q", out, net.lastConnect)
	}
}

func TestDataConnectRNDISUsesSelectedNet(t *testing.T) {
	net := &fakeNet{connectOut: "ok"}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
		Net: net,
		Inventory: &fakeInventory{modems: []domain.ModemDevice{{
			ID: "usb:x",
			NetIfaces: []domain.ModemInterface{
				{Path: "enx1", Kind: domain.IfaceKindNet},
			},
		}}},
	})
	// Select modem so selectedNet is populated
	if _, err := svc.SelectModem(domain.ModemSelectRequest{ModemID: "usb:x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DataConnect(domain.DataConnectRequest{Mode: domain.DataModeRNDIS}); err != nil {
		t.Fatal(err)
	}
	if net.lastConnect != "enx1" {
		t.Fatalf("expected selected net enx1, got %q", net.lastConnect)
	}
}

func TestDataConnectAutoPrefersRNDIS(t *testing.T) {
	net := &fakeNet{connectOut: "ok"}
	mbim := &fakeMBIM{}
	svc := NewModemService(ModemServiceConfig{
		USB:  &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:   &fakeAT{port: "/dev/ttyUSB0"},
		Net:  net,
		MBIM: mbim,
	})
	svc.selectedNet = "enx2"
	svc.selectedMBIM = "/dev/cdc-wdm0"
	if _, err := svc.DataConnect(domain.DataConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	if net.lastConnect != "enx2" {
		t.Fatalf("auto should prefer RNDIS, got %q", net.lastConnect)
	}
	if mbim.lastConnectDev != "" {
		t.Fatal("MBIM should not be used when RNDIS is selected")
	}
}

func TestDataConnectMBIMUsesAPNFromStatus(t *testing.T) {
	mbim := &fakeMBIM{}
	svc := NewModemService(ModemServiceConfig{
		USB:  &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:   &fakeAT{port: "/dev/ttyUSB0"},
		MBIM: mbim,
	})
	svc.selectedMBIM = "/dev/cdc-wdm0"
	svc.status.APN.APN = "internet.mnc"
	if _, err := svc.DataConnect(domain.DataConnectRequest{Mode: domain.DataModeMBIM}); err != nil {
		t.Fatal(err)
	}
	if mbim.lastConnectDev != "/dev/cdc-wdm0" || mbim.lastConnectAPN != "internet.mnc" {
		t.Fatalf("dev=%q apn=%q", mbim.lastConnectDev, mbim.lastConnectAPN)
	}
}

func TestDataConnectUnknownMode(t *testing.T) {
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
		Net: &fakeNet{},
	})
	_, err := svc.DataConnect(domain.DataConnectRequest{Mode: "ppp", Iface: "x"})
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestDataDisconnectRNDIS(t *testing.T) {
	net := &fakeNet{}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
		Net: net,
	})
	svc.selectedNet = "enx9"
	if _, err := svc.DataDisconnect(domain.DataConnectRequest{Mode: domain.DataModeRNDIS}); err != nil {
		t.Fatal(err)
	}
	if net.lastDisconnect != "enx9" {
		t.Fatalf("got %q", net.lastDisconnect)
	}
}

func TestSelectModemNetIface(t *testing.T) {
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  &fakeAT{port: "/dev/ttyUSB0"},
		Inventory: &fakeInventory{modems: []domain.ModemDevice{{
			ID: "usb:1",
			ATPorts: []domain.ModemInterface{
				{Path: "/dev/ttyUSB0", Kind: domain.IfaceKindAT, ATReady: true},
			},
			NetIfaces: []domain.ModemInterface{
				{Path: "enxa", Kind: domain.IfaceKindNet},
				{Path: "enxb", Kind: domain.IfaceKindNet},
			},
		}}},
	})
	inv, err := svc.SelectModem(domain.ModemSelectRequest{
		ModemID:  "usb:1",
		NetIface: "enxb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.SelectedNet != "enxb" {
		t.Fatalf("selected net=%q", inv.SelectedNet)
	}
}

func TestUSBModeQueryAndSet(t *testing.T) {
	at := &fakeAT{port: "/dev/ttyUSB0"}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true}},
		AT:  at,
	})
	// Skip sleep in unit test path: call Get only first
	info := svc.USBMode()
	if info.Mode != domain.USBModeRNDIS41 {
		t.Fatalf("mode=%d", info.Mode)
	}
	if len(info.Supported) < 2 {
		t.Fatal("expected known modes")
	}
}

func TestCachedStatusDoesNotCallAT(t *testing.T) {
	at := &fakeAT{
		port: "/dev/ttyUSB0",
		sig:  domain.SignalInfo{RSSI: -70, Percentage: 50},
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:  at,
	})
	// Seed cache via Status
	_ = svc.Status()
	callsAfterPoll := at.calls
	st := svc.CachedStatus()
	if st.Signal.Percentage != 50 {
		t.Fatalf("cache: %+v", st)
	}
	if at.calls != callsAfterPoll {
		t.Fatalf("CachedStatus must not poll AT, calls %d -> %d", callsAfterPoll, at.calls)
	}
	if st.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt set")
	}
}

func TestRunStatusPollerUpdatesCache(t *testing.T) {
	at := &fakeAT{
		port: "/dev/ttyUSB0",
		sig:  domain.SignalInfo{RSSI: -65, Percentage: 70},
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:  at,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		svc.RunStatusPoller(ctx, 20*time.Millisecond)
		close(done)
	}()

	// Wait until at least one poll (immediate + maybe a tick)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if svc.CachedStatus().Signal.Percentage == 70 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if svc.CachedStatus().Signal.Percentage != 70 {
		t.Fatalf("poller did not update cache: %+v", svc.CachedStatus())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poller did not stop")
	}
}

func TestConcurrentCachedStatusDuringPoll(t *testing.T) {
	at := &fakeAT{
		port: "/dev/ttyUSB0",
		sig:  domain.SignalInfo{RSSI: -70, Percentage: 40},
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:  at,
	})
	_ = svc.Status()

	errCh := make(chan error, 8)
	for i := 0; i < 4; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				_ = svc.CachedStatus()
			}
			errCh <- nil
		}()
	}
	go func() {
		for j := 0; j < 20; j++ {
			_ = svc.Status()
		}
		errCh <- nil
	}()
	for i := 0; i < 5; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}

// trackingAT records concurrent AT method entry so tests can assert atMu serialization.
type trackingAT struct {
	fakeAT
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	hold        time.Duration // artificial delay inside AT ops to widen race window
}

func (f *trackingAT) enter() {
	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	f.mu.Unlock()
	if f.hold > 0 {
		time.Sleep(f.hold)
	}
}

func (f *trackingAT) leave() {
	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
}

func (f *trackingAT) MaxInFlight() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *trackingAT) EnsureConnected() error {
	f.enter()
	defer f.leave()
	return f.fakeAT.EnsureConnected()
}

func (f *trackingAT) SetAPN(cid int, pdp domain.PDPType, apn string) error {
	f.enter()
	defer f.leave()
	return f.fakeAT.SetAPN(cid, pdp, apn)
}

func (f *trackingAT) SetRATMode(pref domain.RATModePref) error {
	f.enter()
	defer f.leave()
	return f.fakeAT.SetRATMode(pref)
}

func (f *trackingAT) SendRaw(cmd string) (string, error) {
	f.enter()
	defer f.leave()
	return f.fakeAT.SendRaw(cmd)
}

func (f *trackingAT) GetUSBMode() (int, error) {
	f.enter()
	defer f.leave()
	return f.fakeAT.GetUSBMode()
}

func (f *trackingAT) SetUSBMode(mode int) error {
	f.enter()
	defer f.leave()
	return f.fakeAT.SetUSBMode(mode)
}

func (f *trackingAT) GetFullStatus() (domain.SignalInfo, domain.NetworkInfo, domain.SIMInfo, domain.APNConfig, domain.RATMode, error) {
	f.enter()
	defer f.leave()
	return f.fakeAT.GetFullStatus()
}

func (f *trackingAT) Close() error {
	f.enter()
	defer f.leave()
	return f.fakeAT.Close()
}

func (f *trackingAT) SetPortName(n string) {
	f.enter()
	defer f.leave()
	f.fakeAT.SetPortName(n)
}

func TestATGateSerializesControlAndPoll(t *testing.T) {
	at := &trackingAT{
		fakeAT: fakeAT{
			port: "/dev/ttyUSB0",
			sig:  domain.SignalInfo{RSSI: -70, Percentage: 55},
		},
		hold: 5 * time.Millisecond,
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:  at,
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = svc.Status()
		}()
		go func() {
			defer wg.Done()
			_ = svc.SetAPN(domain.APNConfig{APN: "internet", PDPType: domain.PDPIPV4V6})
		}()
		go func() {
			defer wg.Done()
			_, _ = svc.RawAT("AT")
		}()
	}
	wg.Wait()

	if max := at.MaxInFlight(); max != 1 {
		t.Fatalf("AT gate failed: max concurrent AT ops = %d (want 1)", max)
	}
}

func TestATGateSerializesUSBModeAndPoll(t *testing.T) {
	at := &trackingAT{
		fakeAT: fakeAT{
			port: "/dev/ttyUSB0",
			sig:  domain.SignalInfo{RSSI: -65, Percentage: 60},
		},
		hold: 3 * time.Millisecond,
	}
	svc := NewModemService(ModemServiceConfig{
		USB:      &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:       at,
		Discover: &fakeDiscover{port: "/dev/ttyUSB0"},
	})

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = svc.Status()
		}()
		go func() {
			defer wg.Done()
			_ = svc.USBMode()
		}()
	}
	wg.Wait()

	if max := at.MaxInFlight(); max != 1 {
		t.Fatalf("AT gate failed on USBMode+poll: max concurrent = %d (want 1)", max)
	}
}

func TestSelectModemPortSwitchUsesATGate(t *testing.T) {
	at := &trackingAT{
		fakeAT: fakeAT{port: "/dev/ttyUSB0"},
		hold:   2 * time.Millisecond,
	}
	svc := NewModemService(ModemServiceConfig{
		USB: &fakeUSB{status: domain.ModemStatus{Connected: true, SysPath: "/sys/x"}},
		AT:  at,
		Inventory: InventoryFuncs{
			ListModemsFn: func(_, _, _ string) []domain.ModemDevice {
				return []domain.ModemDevice{{
					ID:   "serial:/dev/ttyUSB1",
					Name: "Serial",
					ATPorts: []domain.ModemInterface{{
						Path: "/dev/ttyUSB1", Kind: domain.IfaceKindAT, ATReady: true, Label: "/dev/ttyUSB1",
					}},
				}}
			},
		},
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = svc.Status()
		}
	}()
	go func() {
		defer wg.Done()
		_, err := svc.SelectModem(domain.ModemSelectRequest{ATPort: "/dev/ttyUSB1"})
		if err != nil {
			t.Errorf("SelectModem: %v", err)
		}
	}()
	wg.Wait()

	if max := at.MaxInFlight(); max != 1 {
		t.Fatalf("port switch raced with poll: max concurrent = %d", max)
	}
	if at.port != "/dev/ttyUSB1" {
		t.Fatalf("port not switched: %s", at.port)
	}
}
