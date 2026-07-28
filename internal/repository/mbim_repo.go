package repository

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
