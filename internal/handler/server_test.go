package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

type stubService struct {
	status         domain.FullStatus
	lastRAT        domain.RATModePref
	apnErr         error
	inv            domain.ModemInventory
	lastDataReq    domain.DataConnectRequest
	dataConnectOut string
	dataConnectErr error
	dataDiscOut    string
	dataDiscErr    error
}

func (s *stubService) Status() domain.FullStatus       { return s.status }
func (s *stubService) CachedStatus() domain.FullStatus { return s.status }
func (s *stubService) History() []domain.SignalSample {
	return []domain.SignalSample{}
}
func (s *stubService) ListModems() domain.ModemInventory {
	if s.inv.Modems == nil {
		return domain.ModemInventory{Modems: []domain.ModemDevice{}}
	}
	return s.inv
}
func (s *stubService) SelectModem(req domain.ModemSelectRequest) (domain.ModemInventory, error) {
	s.inv.SelectedModemID = req.ModemID
	s.inv.SelectedATPort = req.ATPort
	s.inv.SelectedMBIM = req.MBIMDevice
	s.inv.SelectedNet = req.NetIface
	return s.inv, nil
}
func (s *stubService) SetAPN(domain.APNConfig) error { return s.apnErr }
func (s *stubService) SetRAT(p domain.RATModePref) error {
	s.lastRAT = p
	return nil
}
func (s *stubService) RawAT(string) (string, error) { return "OK", nil }
func (s *stubService) USBReset() (string, error)    { return "/sys/x", nil }
func (s *stubService) MBIMStatus() map[string]any {
	return map[string]any{"mbimcli_available": false, "devices": []string{}}
}
func (s *stubService) MBIMConnect(string, string) (string, error) { return "", nil }
func (s *stubService) MBIMDisconnect(string) (string, error)      { return "", nil }
func (s *stubService) DataConnect(req domain.DataConnectRequest) (string, error) {
	s.lastDataReq = req
	if s.dataConnectOut == "" && s.dataConnectErr == nil {
		return "ok", nil
	}
	return s.dataConnectOut, s.dataConnectErr
}
func (s *stubService) DataDisconnect(req domain.DataConnectRequest) (string, error) {
	s.lastDataReq = req
	if s.dataDiscOut == "" && s.dataDiscErr == nil {
		return "ok", nil
	}
	return s.dataDiscOut, s.dataDiscErr
}
func (s *stubService) USBMode() domain.USBModeInfo {
	return domain.USBModeInfo{Mode: domain.USBModeRNDIS41, Label: domain.USBModeLabel(domain.USBModeRNDIS41), Supported: domain.KnownUSBModes()}
}
func (s *stubService) SetUSBMode(mode int) (domain.USBModeInfo, error) {
	return domain.USBModeInfo{Mode: mode, Label: domain.USBModeLabel(mode), Supported: domain.KnownUSBModes()}, nil
}
func (s *stubService) HotspotStatus() domain.HotspotStatus {
	return domain.HotspotStatus{State: domain.HotspotStateStopped, Config: domain.HotspotConfig{SSID: "FM350-Hotspot"}}
}
func (s *stubService) HotspotSetConfig(domain.HotspotConfig) error { return nil }
func (s *stubService) HotspotStart(domain.HotspotStartRequest) (domain.HotspotStatus, error) {
	return domain.HotspotStatus{State: domain.HotspotStateRunning}, nil
}
func (s *stubService) HotspotStop() (domain.HotspotStatus, error) {
	return domain.HotspotStatus{State: domain.HotspotStateStopped}, nil
}
func (s *stubService) ListWiFi() []domain.WiFiDevice {
	return []domain.WiFiDevice{{Iface: "wlan0", SupportsAP: true, Label: "wlan0 (AP capable)"}}
}

func TestGetStatusEndpoint(t *testing.T) {
	srv := NewServer(&stubService{status: domain.FullStatus{RATMode: domain.RATModeAuto}}, "")
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body domain.FullStatus
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.RATMode != domain.RATModeAuto {
		t.Fatalf("got %+v", body)
	}
}

func TestGetStatusFreshQuery(t *testing.T) {
	// fresh=1 still returns OK with stub Status()
	srv := NewServer(&stubService{status: domain.FullStatus{RATMode: domain.RATMode5GOnly}}, "")
	req := httptest.NewRequest("GET", "/api/status?fresh=1", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body domain.FullStatus
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.RATMode != domain.RATMode5GOnly {
		t.Fatalf("got %+v", body)
	}
}

func TestStaticFilesEndpoint(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for embedded UI, got %d", w.Code)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMBIMStatusEndpoint(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	req := httptest.NewRequest("GET", "/api/mbim", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetRATValid(t *testing.T) {
	stub := &stubService{}
	srv := NewServer(stub, "")
	req := httptest.NewRequest("POST", "/api/rat", strings.NewReader(`{"mode":"5g"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if stub.lastRAT != domain.RATPref5G {
		t.Fatalf("got %q", stub.lastRAT)
	}
}

func TestSetRATInvalid(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	req := httptest.NewRequest("POST", "/api/rat", strings.NewReader(`{"mode":"nsa"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetAPNInvalidPDP(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	req := httptest.NewRequest("POST", "/api/apn", strings.NewReader(`{"apn":"x","pdp_type":"PPP"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListModemsEndpoint(t *testing.T) {
	srv := NewServer(&stubService{inv: domain.ModemInventory{
		Modems: []domain.ModemDevice{{ID: "serial:/dev/ttyUSB0", Name: "Serial"}},
	}}, "")
	req := httptest.NewRequest("GET", "/api/modems", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSelectModemEndpoint(t *testing.T) {
	stub := &stubService{inv: domain.ModemInventory{
		Modems: []domain.ModemDevice{{ID: "m1", Name: "M"}},
	}}
	srv := NewServer(stub, "")
	req := httptest.NewRequest("POST", "/api/modems/select",
		strings.NewReader(`{"modem_id":"m1","at_port":"/dev/ttyUSB0"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDataConnectEndpoint(t *testing.T) {
	stub := &stubService{dataConnectOut: "dhcp ok"}
	srv := NewServer(stub, "")
	req := httptest.NewRequest("POST", "/api/data/connect",
		strings.NewReader(`{"mode":"rndis","iface":"enxabc","apn":"internet"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if stub.lastDataReq.Mode != "rndis" || stub.lastDataReq.Iface != "enxabc" {
		t.Fatalf("req=%+v", stub.lastDataReq)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["output"] != "dhcp ok" {
		t.Fatalf("body=%v", body)
	}
}

func TestDataConnectEndpointError(t *testing.T) {
	stub := &stubService{
		dataConnectOut: "partial",
		dataConnectErr: errors.New("no dhcp"),
	}
	srv := NewServer(stub, "")
	req := httptest.NewRequest("POST", "/api/data/connect",
		strings.NewReader(`{"mode":"rndis","iface":"enx"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestDataDisconnectEndpoint(t *testing.T) {
	stub := &stubService{dataDiscOut: "link down"}
	srv := NewServer(stub, "")
	req := httptest.NewRequest("POST", "/api/data/disconnect",
		strings.NewReader(`{"mode":"mbim","iface":"/dev/cdc-wdm0"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if stub.lastDataReq.Mode != "mbim" || stub.lastDataReq.Iface != "/dev/cdc-wdm0" {
		t.Fatalf("req=%+v", stub.lastDataReq)
	}
}

func TestAuthRequired(t *testing.T) {
	srv := NewServer(&stubService{}, "secret")
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthBearerOK(t *testing.T) {
	srv := NewServer(&stubService{}, "secret")
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthQueryTokenSSE(t *testing.T) {
	srv := NewServer(&stubService{}, "secret")
	req := httptest.NewRequest("GET", "/api/status?token=secret", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSSEEventsStream(t *testing.T) {
	stub := &stubService{status: domain.FullStatus{RATMode: domain.RATMode5GOnly, Signal: domain.SignalInfo{Percentage: 80}}}
	srv := NewServer(stub, "")

	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go srv.Run(hubCtx)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, "GET", "/api/events", nil)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(w, req)
		close(done)
	}()

	// Wait until first SSE frame is written.
	deadline := time.Now().Add(2 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		body = w.Body.String()
		if strings.Contains(body, "data: ") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit after cancel")
	}

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type: %q", ct)
	}
	if !strings.Contains(body, "data: ") {
		t.Fatalf("expected data frame, body=%q", body)
	}
	// Parse first data line JSON.
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var st domain.FullStatus
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &st); err != nil {
			t.Fatalf("json: %v body line %q", err, line)
		}
		if st.RATMode != domain.RATMode5GOnly || st.Signal.Percentage != 80 {
			t.Fatalf("unexpected status: %+v", st)
		}
		return
	}
	t.Fatal("no data line found")
}

func TestSSEEventsAuth(t *testing.T) {
	srv := NewServer(&stubService{status: domain.FullStatus{RATMode: domain.RATModeAuto}}, "secret")

	// Missing token → 401
	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}

	// Query token → stream starts (200 + event-stream)
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go srv.Run(hubCtx)

	ctx, cancel := context.WithCancel(context.Background())
	req = httptest.NewRequestWithContext(ctx, "GET", "/api/events?token=secret", nil)
	w = httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(w, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.Body.String(), "data: ") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type: %q", ct)
	}
	if !strings.Contains(w.Body.String(), "data: ") {
		t.Fatalf("expected SSE data, body=%q", w.Body.String())
	}
}

func TestUSBModeEndpoint(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	req := httptest.NewRequest("GET", "/api/usbmode", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHotspotEndpoints(t *testing.T) {
	srv := NewServer(&stubService{}, "")
	for _, path := range []string{"/api/hotspot", "/api/hotspot/wifi"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		srv.Routes().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/hotspot/start", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest("POST", "/api/hotspot/stop", nil)
	w = httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stop: %d", w.Code)
	}
}
