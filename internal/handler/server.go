package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
	"fm350-monitor/internal/template"
	"fm350-monitor/internal/usecase"
)

// ModemUsecase is the application surface required by HTTP handlers.
type ModemUsecase interface {
	Status() domain.FullStatus
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
}

// Server is the HTTP/SSE adapter.
type Server struct {
	svc ModemUsecase
}

func NewServer(svc ModemUsecase) *Server {
	return &Server{svc: svc}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	fileServer := http.FileServer(http.FS(template.Files))
	mux.Handle("GET /", fileServer)

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

	return mux
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
	_ = json.NewEncoder(w).Encode(s.svc.Status())
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

	if bytes, err := json.Marshal(s.svc.Status()); err == nil {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", bytes)
		flusher.Flush()
	}

	ticker := time.NewTicker(appdefaults.SSEInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			current := s.svc.Status()
			bytes, err := json.Marshal(current)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", bytes)
				flusher.Flush()
			}
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
	var req struct {
		Mode string `json:"mode"`
	}
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
	var req struct {
		Command string `json:"command"`
	}
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
	var req struct {
		APN    string `json:"apn"`
		Device string `json:"device"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	out, err := s.svc.MBIMConnect(req.Device, req.APN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func (s *Server) handleMBIMDisconnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	out, err := s.svc.MBIMDisconnect(req.Device)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func writeUsecaseError(w http.ResponseWriter, err error) {
	if usecase.IsModemUnavailable(err) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
