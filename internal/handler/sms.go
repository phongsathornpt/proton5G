package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

type smsUsecase interface {
	ListSMS() ([]domain.SMSMessage, error)
	ReadSMS(index int) (domain.SMSMessage, error)
	SendSMS(req domain.SMSSendRequest) (domain.SMSSendResult, error)
	DeleteSMS(index int) error
}

type smsNotificationUsecase interface {
	SMSNotifications() []domain.SMSNotification
}

func (s *Server) registerSMSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sms", s.handleSMSList)
	mux.HandleFunc("GET /api/sms/notifications", s.handleSMSNotifications)
	mux.HandleFunc("GET /api/sms/{index}", s.handleSMSRead)
	mux.HandleFunc("POST /api/sms/send", s.handleSMSSend)
	mux.HandleFunc("DELETE /api/sms/{index}", s.handleSMSDelete)
}

func (s *Server) smsService(w http.ResponseWriter) (smsUsecase, bool) {
	svc, ok := s.svc.(smsUsecase)
	if !ok {
		http.Error(w, "SMS service unavailable", http.StatusNotImplemented)
	}
	return svc, ok
}

func (s *Server) handleSMSList(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.smsService(w)
	if !ok {
		return
	}
	msgs, err := svc.ListSMS()
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"messages": msgs})
}

func (s *Server) handleSMSNotifications(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.svc.(smsNotificationUsecase)
	if !ok {
		http.Error(w, "SMS notifications unavailable", http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"notifications": svc.SMSNotifications()})
}

func smsIndex(r *http.Request) (int, error) {
	return strconv.Atoi(strings.TrimSpace(r.PathValue("index")))
}

func (s *Server) handleSMSRead(w http.ResponseWriter, r *http.Request) {
	index, err := smsIndex(r)
	if err != nil || index < 0 {
		http.Error(w, "invalid SMS index", http.StatusBadRequest)
		return
	}
	svc, ok := s.smsService(w)
	if !ok {
		return
	}
	msg, err := svc.ReadSMS(index)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msg)
}

func (s *Server) handleSMSSend(w http.ResponseWriter, r *http.Request) {
	// Sending SMS is billable. Refuse to expose it unless the daemon has an
	// explicit shared token configured; global middleware validates the token.
	if s.token == "" {
		http.Error(w, "SMS send requires -token or FM350_API_TOKEN", http.StatusForbidden)
		return
	}
	var req domain.SMSSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	svc, ok := s.smsService(w)
	if !ok {
		return
	}
	result, err := svc.SendSMS(req)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") || strings.Contains(msg, "empty") || strings.Contains(msg, "exceeds") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.Contains(msg, "rate limit") {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		writeUsecaseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleSMSDelete(w http.ResponseWriter, r *http.Request) {
	if s.token == "" {
		http.Error(w, "SMS delete requires -token or FM350_API_TOKEN", http.StatusForbidden)
		return
	}
	index, err := smsIndex(r)
	if err != nil || index < 0 {
		http.Error(w, "invalid SMS index", http.StatusBadRequest)
		return
	}
	svc, ok := s.smsService(w)
	if !ok {
		return
	}
	if err := svc.DeleteSMS(index); err != nil {
		writeUsecaseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
