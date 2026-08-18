package usecase

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

const (
	smsRateWindow = time.Minute
	smsRateMax    = 10
	smsIdemTTL    = 10 * time.Minute
)

type smsATRepository interface {
	ListSMS() ([]domain.SMSMessage, error)
	ReadSMS(index int) (domain.SMSMessage, error)
	SendSMS(req domain.SMSSendRequest) (domain.SMSSendResult, error)
	DeleteSMS(index int) error
}

type smsNotificationRepository interface {
	DrainSMSNotifications() []domain.SMSNotification
}

type smsGuardState struct {
	mu          sync.Mutex
	windowStart time.Time
	windowCount int
	idem        map[string]smsIdemEntry
}

type smsIdemEntry struct {
	result domain.SMSSendResult
	at     time.Time
}

var smsGuards sync.Map // map[*ModemService]*smsGuardState; one state per daemon service

func (s *ModemService) smsAT() (smsATRepository, error) {
	if s.at == nil {
		return nil, errModemUnavailable
	}
	at, ok := s.at.(smsATRepository)
	if !ok {
		return nil, fmt.Errorf("SMS is not supported by the active AT repository")
	}
	return at, nil
}

func (s *ModemService) smsGuard() *smsGuardState {
	if v, ok := smsGuards.Load(s); ok {
		return v.(*smsGuardState)
	}
	g := &smsGuardState{idem: make(map[string]smsIdemEntry)}
	actual, _ := smsGuards.LoadOrStore(s, g)
	return actual.(*smsGuardState)
}

func (s *ModemService) ListSMS() ([]domain.SMSMessage, error) {
	var out []domain.SMSMessage
	err := s.withAT(func() error {
		at, err := s.smsAT()
		if err != nil {
			return err
		}
		out, err = at.ListSMS()
		return err
	})
	return out, err
}

func (s *ModemService) ReadSMS(index int) (domain.SMSMessage, error) {
	var out domain.SMSMessage
	if index < 0 {
		return out, fmt.Errorf("invalid SMS index")
	}
	err := s.withAT(func() error {
		at, err := s.smsAT()
		if err != nil {
			return err
		}
		out, err = at.ReadSMS(index)
		return err
	})
	return out, err
}

func (s *ModemService) DeleteSMS(index int) error {
	if index < 0 {
		return fmt.Errorf("invalid SMS index")
	}
	return s.withAT(func() error {
		at, err := s.smsAT()
		if err != nil {
			return err
		}
		return at.DeleteSMS(index)
	})
}

func (s *ModemService) SMSNotifications() []domain.SMSNotification {
	if s.at == nil {
		return nil
	}
	at, ok := s.at.(smsNotificationRepository)
	if !ok {
		return nil
	}
	// Queue operations are internally synchronized and perform no serial I/O.
	return at.DrainSMSNotifications()
}

func (s *ModemService) SendSMS(req domain.SMSSendRequest) (domain.SMSSendResult, error) {
	var zero domain.SMSSendResult
	req.To = strings.TrimSpace(req.To)
	req.Body = strings.TrimSpace(req.Body)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.To == "" {
		return zero, fmt.Errorf("SMS destination is required")
	}
	if req.Body == "" {
		return zero, fmt.Errorf("SMS body is required")
	}
	if len([]rune(req.Body)) > 1000 {
		return zero, fmt.Errorf("SMS body exceeds 1000 characters")
	}

	g := s.smsGuard()
	now := time.Now()
	g.mu.Lock()
	for key, entry := range g.idem {
		if now.Sub(entry.at) > smsIdemTTL {
			delete(g.idem, key)
		}
	}
	if req.IdempotencyKey != "" {
		if entry, ok := g.idem[req.IdempotencyKey]; ok {
			g.mu.Unlock()
			return entry.result, nil
		}
	}
	if g.windowStart.IsZero() || now.Sub(g.windowStart) >= smsRateWindow {
		g.windowStart = now
		g.windowCount = 0
	}
	if g.windowCount >= smsRateMax {
		g.mu.Unlock()
		return zero, fmt.Errorf("SMS send rate limit exceeded; retry later")
	}
	g.windowCount++
	g.mu.Unlock()

	var result domain.SMSSendResult
	err := s.withAT(func() error {
		at, err := s.smsAT()
		if err != nil {
			return err
		}
		result, err = at.SendSMS(req)
		return err
	})
	if err != nil {
		return zero, err
	}

	if req.IdempotencyKey != "" {
		g.mu.Lock()
		g.idem[req.IdempotencyKey] = smsIdemEntry{result: result, at: time.Now()}
		g.mu.Unlock()
	}
	return result, nil
}
