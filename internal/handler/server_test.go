package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

type stubService struct {
	status  domain.FullStatus
	lastRAT domain.RATModePref
	apnErr  error
	inv     domain.ModemInventory
}

func (s *stubService) Status() domain.FullStatus { return s.status }
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

func TestGetStatusEndpoint(t *testing.T) {
	srv := NewServer(&stubService{status: domain.FullStatus{RATMode: domain.RATModeAuto}})
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

func TestStaticFilesEndpoint(t *testing.T) {
	srv := NewServer(&stubService{})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for embedded UI, got %d", w.Code)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	srv := NewServer(&stubService{})
	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMBIMStatusEndpoint(t *testing.T) {
	srv := NewServer(&stubService{})
	req := httptest.NewRequest("GET", "/api/mbim", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetRATValid(t *testing.T) {
	stub := &stubService{}
	srv := NewServer(stub)
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
	srv := NewServer(&stubService{})
	req := httptest.NewRequest("POST", "/api/rat", strings.NewReader(`{"mode":"nsa"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetAPNInvalidPDP(t *testing.T) {
	srv := NewServer(&stubService{})
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
	}})
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
	srv := NewServer(stub)
	req := httptest.NewRequest("POST", "/api/modems/select",
		strings.NewReader(`{"modem_id":"m1","at_port":"/dev/ttyUSB0"}`))
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}
