package usecase

import "fm350-monitor/internal/pkg/domain"

// USBRepository abstracts sysfs USB power management and hard reset.
type USBRepository interface {
	Check(globPattern string) domain.ModemStatus
	DisableAutosuspend() error
	HardReset(sysPath string) error
}

// ATRepository abstracts serial AT I/O.
type ATRepository interface {
	PortName() string
	SetPortName(name string)
	Close() error
	Connect() error
	EnsureConnected() error
	GetFullStatus() (domain.SignalInfo, domain.NetworkInfo, domain.SIMInfo, domain.APNConfig, domain.RATMode, error)
	SetAPN(cid int, pdpType domain.PDPType, apn string) error
	SetRATMode(pref domain.RATModePref) error
	SendRaw(cmd string) (string, error)
	GetUSBMode() (int, error)
	SetUSBMode(mode int) error
}

// HistoryRepository abstracts signal sample storage.
type HistoryRepository interface {
	Add(s domain.SignalSample)
	Snapshot() []domain.SignalSample
	LoadFile(path string) error
	SaveFile(path string) error
}

// MBIMRepository abstracts optional mbimcli bearer control.
type MBIMRepository interface {
	Status() map[string]any
	Connect(device, apn string) (string, error)
	Disconnect(device string) (string, error)
}

// NetRepository abstracts RNDIS/ECM network iface bring-up and address query.
type NetRepository interface {
	ConnectRNDIS(iface string) (string, error)
	DisconnectRNDIS(iface string) (string, error)
	IfaceAddrs(iface string) []string
}

// ATDiscoverer finds a working AT serial port.
type ATDiscoverer interface {
	DiscoverATPort(vendor, product string) (string, error)
}

// DeviceInventory lists USB/serial/MBIM modem candidates for the UI.
type DeviceInventory interface {
	ListModems(vendor, product, openATPort string) []domain.ModemDevice
	ListMBIMDevices() []string
	MBIMCLIAvailable() bool
	MBIMInstallHint() string
}

// discoverFunc adapts a plain function to ATDiscoverer.
type discoverFunc func(vendor, product string) (string, error)

func (f discoverFunc) DiscoverATPort(vendor, product string) (string, error) {
	return f(vendor, product)
}

// InventoryFunc adapts package functions to DeviceInventory.
type InventoryFuncs struct {
	ListModemsFn      func(vendor, product, openATPort string) []domain.ModemDevice
	ListMBIMFn        func() []string
	MBIMAvailableFn   func() bool
	MBIMInstallHintFn func() string
}

func (i InventoryFuncs) ListModems(vendor, product, openATPort string) []domain.ModemDevice {
	if i.ListModemsFn == nil {
		return nil
	}
	return i.ListModemsFn(vendor, product, openATPort)
}
func (i InventoryFuncs) ListMBIMDevices() []string {
	if i.ListMBIMFn == nil {
		return nil
	}
	return i.ListMBIMFn()
}
func (i InventoryFuncs) MBIMCLIAvailable() bool {
	if i.MBIMAvailableFn == nil {
		return false
	}
	return i.MBIMAvailableFn()
}
func (i InventoryFuncs) MBIMInstallHint() string {
	if i.MBIMInstallHintFn == nil {
		return ""
	}
	return i.MBIMInstallHintFn()
}
