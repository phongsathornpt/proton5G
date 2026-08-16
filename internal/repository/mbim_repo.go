package repository

import "fm350-monitor/internal/pkg/domain"

// MBIM is a thin struct adapter so usecase can depend on a repository value.
type MBIM struct{}

func NewMBIM() *MBIM { return &MBIM{} }

func (m *MBIM) Status() map[string]any {
	return Status()
}

func (m *MBIM) Connect(device, apn string) (string, error) {
	return Connect(device, apn)
}

func (m *MBIM) Disconnect(device string) (string, error) {
	return Disconnect(device)
}

// QueryIPConfig is an optional capability consumed by WANService. It is kept
// outside the base MBIMRepository interface so existing adapters remain compatible.
func (m *MBIM) QueryIPConfig(device string) (domain.WANIPConfig, error) {
	return QueryIPConfig(device)
}
