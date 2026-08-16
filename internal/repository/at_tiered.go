package repository

import (
	"strings"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

const (
	atMediumPollInterval = 10 * time.Second
	atSlowPollInterval   = 60 * time.Second
)

// TieredClient keeps high-rate radio telemetry fresh while moving slow/static
// AT queries off the 2-second hot path. It embeds Client so it satisfies the same
// usecase ATRepository interface without changing the serial implementation.
type TieredClient struct {
	*Client

	cacheMu sync.Mutex
	cache   tieredPollCache

	// Test hooks. Production leaves these nil and uses the embedded Client.
	nowFn    func() time.Time
	sendFn   func(string) (string, error)
	ensureFn func() error
}

type tieredPollCache struct {
	operator   string
	copsAct    int
	hasCopsAct bool
	tempC      float64
	simState   domain.SIMState
	imsi       string
	iccid      string
	apn        domain.APNConfig
	rat        domain.RATMode
	pdp        domain.PDPSession
	identity   domain.ModemIdentity
	mediumAt   time.Time
	slowAt     time.Time
}

func NewTieredClient(portName string) *TieredClient {
	return &TieredClient{Client: NewClient(portName)}
}

func (c *TieredClient) now() time.Time {
	if c.nowFn != nil {
		return c.nowFn()
	}
	return time.Now()
}

// ensureOpen deliberately avoids a standalone AT health ping. The first fast
// command (CSQ) validates the port and Client.SendRaw already closes it on I/O error.
func (c *TieredClient) ensureOpen() error {
	if c.ensureFn != nil {
		return c.ensureFn()
	}
	if c.Client.Connected() {
		return nil
	}
	return c.Client.Connect()
}

func (c *TieredClient) send(cmd string) (string, error) {
	if c.sendFn != nil {
		return c.sendFn(cmd)
	}
	return c.Client.SendRaw(cmd)
}

func (c *TieredClient) snapshotCache() tieredPollCache {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	return c.cache
}

func (c *TieredClient) storeCache(cache tieredPollCache) {
	c.cacheMu.Lock()
	c.cache = cache
	c.cacheMu.Unlock()
}

func (c *TieredClient) resetPollCache() {
	c.cacheMu.Lock()
	c.cache = tieredPollCache{}
	c.cacheMu.Unlock()
}

func (c *TieredClient) invalidateSlowCache() {
	c.cacheMu.Lock()
	c.cache.slowAt = time.Time{}
	c.cacheMu.Unlock()
}

// SetPortName also drops cached SIM/network metadata because a new tty may belong
// to a different modem or a re-enumerated device.
func (c *TieredClient) SetPortName(name string) {
	old := c.Client.PortName()
	c.Client.SetPortName(name)
	if name != "" && name != old {
		c.resetPollCache()
	}
}

func (c *TieredClient) SetAPN(cid int, pdpType domain.PDPType, apn string) error {
	if err := c.Client.SetAPN(cid, pdpType, apn); err != nil {
		return err
	}
	c.invalidateSlowCache()
	return nil
}

func (c *TieredClient) SetRATMode(pref domain.RATModePref) error {
	if err := c.Client.SetRATMode(pref); err != nil {
		return err
	}
	c.invalidateSlowCache()
	return nil
}

func (c *TieredClient) SetUSBMode(mode int) error {
	if err := c.Client.SetUSBMode(mode); err != nil {
		return err
	}
	c.resetPollCache()
	return nil
}

// GetFullStatus polls fast-changing radio values every call, medium values every
// 10 seconds, and SIM/config/identity values every 60 seconds. A normal 2-second
// cycle therefore issues only CSQ, CESQ, C5GREG, CEREG, GTCAINFO and GTCCINFO.
func (c *TieredClient) GetFullStatus() (domain.ATPoll, error) {
	var poll domain.ATPoll
	if err := c.ensureOpen(); err != nil {
		return poll, err
	}

	// Fast tier: values that affect the live radio dashboard.
	csqResp, err := c.send(CmdCSQ)
	if err != nil {
		return poll, err
	}
	cesqResp, _ := c.send(CmdCESQ)
	c5gResp, _ := c.send(CmdC5GREG)
	clteResp, _ := c.send(CmdCEREG)
	gtca, _ := c.send(CmdGTCAINFO)
	gtcc, _ := c.send(CmdGTCCINFO)

	now := c.now()
	cache := c.snapshotCache()

	// Slow tier first because medium PDP polling uses the configured CID.
	if cache.slowAt.IsZero() || now.Sub(cache.slowAt) >= atSlowPollInterval {
		c.refreshSlow(&cache)
		cache.slowAt = now
	}
	if cache.mediumAt.IsZero() || now.Sub(cache.mediumAt) >= atMediumPollInterval {
		c.refreshMedium(&cache)
		cache.mediumAt = now
	}
	c.storeCache(cache)

	sig := MergeExtendedSignal(ParseCSQ(csqResp), cesqResp, gtca, gtcc)
	cells := ParseGTCCINFOCells(gtcc)
	ca := ParseGTCAINFOComponents(gtca)
	ca = EnrichGTCAINFOComponents(ca, gtca)
	ca = CorrelateCAWithCells(ca, cells)
	sig = ApplyServingCell(sig, cells)

	reg5g := ParseRegistration(c5gResp)
	regLTE := ParseRegistration(clteResp)
	tech, endcInfo := DetectRadioTech(cache.copsAct, cache.hasCopsAct, regLTE, reg5g, cells, ca)
	regState := domain.RegNotRegistered
	if reg5g.IsRegistered() {
		regState = reg5g
	} else if regLTE.IsRegistered() {
		regState = regLTE
	}

	operator := cache.operator
	if strings.TrimSpace(operator) == "" {
		operator = unknownOperator
	}
	apn := cache.apn
	if apn.CID <= 0 {
		apn.CID = 1
	}
	if apn.PDPType == "" {
		apn.PDPType = domain.DefaultPDPType()
	}
	if apn.IPAddr == "" {
		apn.IPAddr = cache.pdp.IP
	}

	poll.Signal = sig
	poll.Network = domain.NetworkInfo{
		Operator: operator,
		RegState: regState,
		Tech:     tech,
		Reg5G:    reg5g,
		RegLTE:   regLTE,
		ENDC:     endcInfo,
	}
	poll.SIM = domain.SIMInfo{
		State: cache.simState,
		IMSI:  cache.imsi,
		ICCID: cache.iccid,
	}
	poll.APN = apn
	poll.RATMode = cache.rat
	poll.TempC = cache.tempC
	poll.Cells = cells
	poll.CA = ca
	poll.PDP = cache.pdp
	poll.Identity = cache.identity
	return poll, nil
}

func (c *TieredClient) refreshMedium(cache *tieredPollCache) {
	if resp, err := c.send(CmdCOPS); err == nil {
		oper, act, hasAct := ParseCOPSFull(resp)
		if oper != "" && (oper != unknownOperator || cache.operator == "") {
			cache.operator = oper
		}
		cache.copsAct = act
		cache.hasCopsAct = hasAct
	}
	if resp, err := c.send(CmdGTSENRDTEMP); err == nil {
		if tempC, ok := ParseGTSENRDTEMP(resp); ok {
			cache.tempC = tempC
		}
	}

	cid := cache.apn.CID
	if cid <= 0 {
		cid = 1
	}
	pdp := cache.pdp
	pdp.CID = cid
	if resp, err := c.send(CmdCGPADDR(cid)); err == nil {
		if ip := ParseCGPADDR(resp); ip != "" {
			pdp.IP = ip
			pdp.Gateway = GuessIPv4Gateway(ip)
		}
	}
	if resp, err := c.send(CmdGTDNS(cid)); err == nil {
		dns1, dns2 := ParseGTDNS(resp)
		if dns1 != "" || dns2 != "" {
			pdp.DNS1, pdp.DNS2 = dns1, dns2
		}
	}
	cache.pdp = pdp
}

func (c *TieredClient) refreshSlow(cache *tieredPollCache) {
	if resp, err := c.send(CmdCPIN); err == nil {
		cache.simState = ParseCPIN(resp)
	}
	if resp, err := c.send(CmdCIMI); err == nil {
		if v := ParseCIMI(resp); v != "" {
			cache.imsi = v
		}
	}
	if resp, err := c.send(CmdCCID); err == nil {
		if v := ParseICCID(resp); v != "" {
			cache.iccid = v
		} else if alt, altErr := c.send(CmdICCID); altErr == nil {
			if v = ParseICCID(alt); v != "" {
				cache.iccid = v
			}
		}
	}
	if resp, err := c.send(CmdCGDCONTQ); err == nil {
		cache.apn = ParseCGDCONT(resp)
	}
	if resp, err := c.send(CmdGTACTQ); err == nil {
		cache.rat = ParseGTACT(resp)
	}

	identity := cache.identity
	if resp, err := c.send(CmdCGMI); err == nil {
		if v := ParseInfoLine(resp); v != "" {
			identity.Manufacturer = v
		}
	}
	if resp, err := c.send(CmdCGMM); err == nil {
		if v := ParseInfoLine(resp); v != "" {
			identity.Model = v
		}
	}
	if resp, err := c.send(CmdCGMR); err == nil {
		if v := ParseInfoLine(resp); v != "" {
			identity.Firmware = v
		}
	}
	if resp, err := c.send(CmdCGSN); err == nil {
		if v := ParseInfoLine(resp); v != "" {
			identity.IMEI = v
		}
	}
	cache.identity = identity
}
