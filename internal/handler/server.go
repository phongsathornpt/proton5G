package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
	"fm350-monitor/internal/template"
	"fm350-monitor/internal/usecase"
)

// ModemUsecase is the application surface required by HTTP handlers.
type ModemUsecase interface {
	Status() domain.FullStatus       // force AT/USB poll
	CachedStatus() domain.FullStatus // last poll snapshot (SSE hot path)
	History() []domain.SignalSample
	ListModems() domain.ModemInventory
	SelectModem(req domain.ModemSelectRequest) (domain.ModemInventory, error)
	SetAPN(cfg domain.APNConfig) error
	SetRAT(pref domain.RATModePref) error
	RawAT(cmd string) (string, error)
	USBReset() (sysPath string, err error)
	MBIMStatus() map[string]any
	MBIMConnect(device, apn string) (string, error)
	MBIMDisconnect(device string) (string, error)
	DataConnect(req domain.DataConnectRequest) (string, error)
	DataDisconnect(req domain.DataConnectRequest) (string, error)
	USBMode() domain.USBModeInfo
	SetUSBMode(mode int) (domain.USBModeInfo, error)
	HotspotStatus() domain.HotspotStatus
	HotspotSetConfig(cfg domain.HotspotConfig) error
	HotspotStart(req domain.HotspotStartRequest) (domain.HotspotStatus, error)
	HotspotStop() (domain.HotspotStatus, error)
	ListWiFi() []domain.WiFiDevice
}

// Server is the HTTP/SSE adapter.
type Server struct {
	svc   ModemUsecase
	token string // optional shared API token; empty = open access
	hub   *SSEHub
}

// NewServer builds the HTTP adapter. token protects all routes when non-empty
// (Authorization Bearer, X-API-Token, or ?token= for SSE).
// Call Run(ctx) once from main so the SSE hub ticks.
func NewServer(svc ModemUsecase, token string) *Server {
	s := &Server{svc: svc, token: strings.TrimSpace(token)}
	s.hub = NewSSEHub(svc.CachedStatus, appdefaults.SSEInterval)
	return s
}

// Run starts the SSE broadcast hub until ctx is cancelled. Start once from main.
func (s *Server) Run(ctx context.Context) {
	if s.hub != nil {
		s.hub.Run(ctx)
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Feature-based UI: composed HTML index + static assets under /assets/
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /index.html", s.handleIndex)
	assetFS, err := fs.Sub(template.Assets, "assets")
	if err != nil {
		// Should not happen if embed is correct; fall back to empty handler.
		mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "assets unavailable", http.StatusInternalServerError)
		})
	} else {
		mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetFS))))
	}

	mux.HandleFunc("GET /api/status", s.handleGetStatus)
	mux.HandleFunc("GET /api/events", s.handleSSE)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/modems", s.handleListModems)
	mux.HandleFunc("POST /api/modems/select", s.handleSelectModem)
	mux.HandleFunc("POST /api/apn", s.handleSetAPN)
	mux.HandleFunc("POST /api/rat", s.handleSetRAT)
	mux.HandleFunc("POST /api/raw", s.handleRawAT)
	mux.HandleFunc("POST /api/reset", s.handleReset)
	mux.HandleFunc("GET /api/mbim", s.handleMBIMStatus)
	mux.HandleFunc("POST /api/mbim/connect", s.handleMBIMConnect)
	mux.HandleFunc("POST /api/mbim/disconnect", s.handleMBIMDisconnect)
	mux.HandleFunc("POST /api/data/connect", s.handleDataConnect)
	mux.HandleFunc("POST /api/data/disconnect", s.handleDataDisconnect)
	mux.HandleFunc("GET /api/usbmode", s.handleGetUSBMode)
	mux.HandleFunc("POST /api/usbmode", s.handleSetUSBMode)
	mux.HandleFunc("GET /api/hotspot", s.handleHotspotStatus)
	mux.HandleFunc("GET /api/hotspot/wifi", s.handleHotspotWiFi)
	mux.HandleFunc("POST /api/hotspot/config", s.handleHotspotConfig)
	mux.HandleFunc("POST /api/hotspot/start", s.handleHotspotStart)
	mux.HandleFunc("POST /api/hotspot/stop", s.handleHotspotStop)
	s.registerSMSRoutes(mux)

	var h http.Handler = mux
	if s.token != "" {
		h = s.authMiddleware(mux)
	}
	return h
}

func (s *Server) handleListModems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.svc.ListModems())
}

func (s *Server) handleSelectModem(w http.ResponseWriter, r *http.Request) {
	var req domain.ModemSelectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	inv, err := s.svc.SelectModem(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(inv)
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Default: cached snapshot from background poller. ?fresh=1 forces AT/USB poll.
	if r.URL.Query().Get("fresh") == "1" || strings.EqualFold(r.URL.Query().Get("fresh"), "true") {
		_ = json.NewEncoder(w).Encode(s.svc.Status())
		return
	}
	_ = json.NewEncoder(w).Encode(s.svc.CachedStatus())
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.svc.History())
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch, unsub := s.hub.Subscribe()
	defer unsub()

	writeFrame := func(payload []byte) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	select {
	case payload, ok := <-ch:
		if !ok { return }
		if !writeFrame(payload) { return }
	default:
		if raw, err := json.Marshal(s.svc.CachedStatus()); err == nil {
			if !writeFrame(raw) { return }
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-ch:
			if !ok { return }
			if !writeFrame(payload) { return }
		}
	}
}

func (s *Server) handleSetAPN(w http.ResponseWriter, r *http.Request) {
	var req domain.APNConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.PDPType != "" {
		if _, err := domain.ParsePDPType(string(req.PDPType)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := s.svc.SetAPN(req); err != nil {
		writeUsecaseError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleSetRAT(w http.ResponseWriter, r *http.Request) {
	var req struct { Mode string `json:"mode"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pref, err := domain.ParseRATModePref(req.Mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.svc.SetRAT(pref); err != nil {
		writeUsecaseError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleRawAT(w http.ResponseWriter, r *http.Request) {
	var req struct { Command string `json:"command"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := s.svc.RawAT(req.Command)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"response": resp})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	sysPath, err := s.svc.USBReset()
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "sys_path": sysPath})
}

func (s *Server) handleMBIMStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.svc.MBIMStatus())
}

func (s *Server) handleMBIMConnect(w http.ResponseWriter, r *http.Request) {
	var req struct { APN string `json:"apn"`; Device string `json:"device"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := s.svc.MBIMConnect(req.Device, req.APN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func (s *Server) handleMBIMDisconnect(w http.ResponseWriter, r *http.Request) {
	var req struct { Device string `json:"device"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := s.svc.MBIMDisconnect(req.Device)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func (s *Server) handleDataConnect(w http.ResponseWriter, r *http.Request) {
	var req domain.DataConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := s.svc.DataConnect(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func (s *Server) handleDataDisconnect(w http.ResponseWriter, r *http.Request) {
	var req domain.DataConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := s.svc.DataDisconnect(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func (s *Server) handleGetUSBMode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.svc.USBMode())
}

func (s *Server) handleSetUSBMode(w http.ResponseWriter, r *http.Request) {
	var req struct { Mode int `json:"mode"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := s.svc.SetUSBMode(req.Mode)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "usb_mode": info})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "usb_mode": info})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	html, err := template.RenderIndex()
	if err != nil {
		http.Error(w, "UI render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}

func (s *Server) handleHotspotStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.svc.HotspotStatus())
}

func (s *Server) handleHotspotWiFi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"devices": s.svc.ListWiFi()})
}

func (s *Server) handleHotspotConfig(w http.ResponseWriter, r *http.Request) {
	var cfg domain.HotspotConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.svc.HotspotSetConfig(cfg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleHotspotStart(w http.ResponseWriter, r *http.Request) {
	var req domain.HotspotStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	st, err := s.svc.HotspotStart(req)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "hotspot": st})
		return
	}
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) handleHotspotStop(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.HotspotStop()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "hotspot": st})
		return
	}
	_ = json.NewEncoder(w).Encode(st)
}

func writeUsecaseError(w http.ResponseWriter, err error) {
	if usecase.IsModemUnavailable(err) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
