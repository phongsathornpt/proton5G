package repository

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"fm350-monitor/internal/pkg/domain"
)

type Client struct {
	portName string
	port     serial.Port
	mu       sync.Mutex
	ident    domain.ModemIdentity // cached after first successful identity poll
}

func NewClient(portName string) *Client {
	return &Client{portName: portName}
}

func (c *Client) PortName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.portName
}

func (c *Client) SetPortName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if name == "" || name == c.portName {
		return
	}
	if c.port != nil {
		_ = c.port.Close()
		c.port = nil
	}
	c.portName = name
	c.ident = domain.ModemIdentity{}
}

func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.port != nil
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *Client) connectLocked() error {
	if c.port != nil {
		_ = c.port.Close()
		c.port = nil
	}
	if c.portName == "" {
		return fmt.Errorf("serial port name is empty")
	}

	mode := &serial.Mode{
		BaudRate: SerialBaud,
		DataBits: SerialDataBits,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(c.portName, mode)
	if err != nil {
		return fmt.Errorf("open serial port %s: %w", c.portName, err)
	}
	_ = p.SetReadTimeout(2 * time.Second)
	c.port = p
	return nil
}

// EnsureConnected opens the port if needed and verifies it with a basic AT ping.
func (c *Client) EnsureConnected() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.port == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}

	resp, err := c.sendRawLocked(CmdAT)
	if err != nil || !strings.Contains(resp, ATResultOK) {
		_ = c.connectLocked()
		resp, err = c.sendRawLocked(CmdAT)
		if err != nil {
			return err
		}
		if !strings.Contains(resp, ATResultOK) {
			return fmt.Errorf("AT ping failed on %s: %s", c.portName, resp)
		}
	}
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port != nil {
		err := c.port.Close()
		c.port = nil
		return err
	}
	return nil
}

func (c *Client) SendRaw(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendRawLocked(cmd)
}

func (c *Client) sendRawLocked(cmd string) (string, error) {
	if c.port == nil {
		return "", fmt.Errorf("serial port not connected")
	}

	cleanCmd := strings.TrimSpace(cmd)
	if !strings.HasSuffix(cleanCmd, "\r") && !strings.HasSuffix(cleanCmd, "\r\n") {
		cleanCmd += "\r"
	}

	// Do not ResetInputBuffer here. The modem may have queued SMS URCs between
	// polling commands; resetting would silently lose +CMTI/+CMT/+CDS events.
	if _, err := c.port.Write([]byte(cleanCmd)); err != nil {
		_ = c.port.Close()
		c.port = nil
		return "", fmt.Errorf("write AT command: %w", err)
	}

	var resp []byte
	buf := make([]byte, 512)
	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		n, err := c.port.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			str := string(resp)
			if strings.Contains(str, ATResultOK) || strings.Contains(str, ATResultERROR) || strings.Contains(str, "+CMS ERROR:") {
				break
			}
		}
		if err != nil {
			if n == 0 && len(resp) == 0 {
				_ = c.port.Close()
				c.port = nil
				return "", fmt.Errorf("read AT response: %w", err)
			}
			break
		}
	}

	if len(resp) == 0 {
		return "", fmt.Errorf("empty AT response from %s", c.portName)
	}

	return c.storeAndStripSMSURCs(string(resp)), nil
}

func (c *Client) GetFullStatus() (domain.ATPoll, error) {
	var poll domain.ATPoll
	if err := c.EnsureConnected(); err != nil {
		return poll, err
	}

	csqResp, err := c.SendRaw(CmdCSQ)
	if err != nil {
		return poll, err
	}
	cesqResp, _ := c.SendRaw(CmdCESQ)
	copsResp, _ := c.SendRaw(CmdCOPS)
	c5gResp, _ := c.SendRaw(CmdC5GREG)
	clteResp, _ := c.SendRaw(CmdCEREG)
	cpinResp, _ := c.SendRaw(CmdCPIN)
	cimiResp, _ := c.SendRaw(CmdCIMI)
	ccidResp, _ := c.SendRaw(CmdCCID)
	cgdResp, _ := c.SendRaw(CmdCGDCONTQ)
	gtactResp, _ := c.SendRaw(CmdGTACTQ)
	tempResp, _ := c.SendRaw(CmdGTSENRDTEMP)
	gtca, _ := c.SendRaw(CmdGTCAINFO)
	gtcc, _ := c.SendRaw(CmdGTCCINFO)

	sig := MergeExtendedSignal(ParseCSQ(csqResp), cesqResp, gtca, gtcc)
	cells := ParseGTCCINFOCells(gtcc)
	ca := ParseGTCAINFOComponents(gtca)
	ca = EnrichGTCAINFOComponents(ca, gtca)
	ca = CorrelateCAWithCells(ca, cells)
	sig = ApplyServingCell(sig, cells)

	oper, copsAct, hasCopsAct := ParseCOPSFull(copsResp)
	reg5g := ParseRegistration(c5gResp)
	regLTE := ParseRegistration(clteResp)
	simState := ParseCPIN(cpinResp)
	imsi := ParseCIMI(cimiResp)
	iccid := ParseICCID(ccidResp)
	if iccid == "" {
		if alt, err := c.SendRaw(CmdICCID); err == nil {
			iccid = ParseICCID(alt)
		}
	}
	apn := ParseCGDCONT(cgdResp)
	rat := ParseGTACT(gtactResp)
	if tempC, ok := ParseGTSENRDTEMP(tempResp); ok {
		poll.TempC = tempC
	}

	cid := apn.CID
	if cid <= 0 {
		cid = 1
	}
	pdp, _ := c.QueryPDP(cid)
	if apn.IPAddr == "" {
		apn.IPAddr = pdp.IP
	}

	tech, endcInfo := DetectRadioTech(copsAct, hasCopsAct, regLTE, reg5g, cells, ca)
	regState := domain.RegNotRegistered
	if reg5g.IsRegistered() {
		regState = reg5g
	} else if regLTE.IsRegistered() {
		regState = regLTE
	}

	poll.Signal = sig
	poll.Network = domain.NetworkInfo{
		Operator: oper,
		RegState: regState,
		Tech:     tech,
		Reg5G:    reg5g,
		RegLTE:   regLTE,
		ENDC:     endcInfo,
	}
	poll.SIM = domain.SIMInfo{
		State: simState,
		IMSI:  imsi,
		ICCID: iccid,
	}
	poll.APN = apn
	poll.RATMode = rat
	poll.Cells = cells
	poll.CA = ca
	poll.PDP = pdp
	poll.Identity = c.pollIdentity()
	return poll, nil
}

func (c *Client) pollIdentity() domain.ModemIdentity {
	c.mu.Lock()
	ident := c.ident
	c.mu.Unlock()
	if ident.IMEI != "" {
		return ident
	}
	if resp, err := c.SendRaw(CmdCGMI); err == nil {
		ident.Manufacturer = ParseInfoLine(resp)
	}
	if resp, err := c.SendRaw(CmdCGMM); err == nil {
		ident.Model = ParseInfoLine(resp)
	}
	if resp, err := c.SendRaw(CmdCGMR); err == nil {
		ident.Firmware = ParseInfoLine(resp)
	}
	if resp, err := c.SendRaw(CmdCGSN); err == nil {
		ident.IMEI = ParseInfoLine(resp)
	}
	if ident.IMEI != "" || ident.Model != "" {
		c.mu.Lock()
		c.ident = ident
		c.mu.Unlock()
	}
	return ident
}

func (c *Client) ActivatePDP(cid int) error {
	if cid <= 0 {
		cid = 1
	}
	if err := c.EnsureConnected(); err != nil {
		return err
	}
	cmd := CmdCGACTSet(cid, true)
	resp, err := c.SendRaw(cmd)
	if err != nil {
		return err
	}
	if strings.Contains(resp, ATResultOK) {
		return nil
	}
	// Already-active contexts often return ERROR; accept if an address exists.
	sess, qerr := c.QueryPDP(cid)
	if qerr == nil && sess.IP != "" {
		return nil
	}
	return fmt.Errorf("%s failed: %s", cmd, resp)
}

func (c *Client) QueryPDP(cid int) (domain.PDPSession, error) {
	sess := domain.PDPSession{CID: cid}
	if cid <= 0 {
		sess.CID = 1
		cid = 1
	}
	if err := c.EnsureConnected(); err != nil {
		return sess, err
	}
	addrResp, err := c.SendRaw(CmdCGPADDR(cid))
	if err != nil {
		return sess, err
	}
	sess.IP = ParseCGPADDR(addrResp)
	if dnsResp, derr := c.SendRaw(CmdGTDNS(cid)); derr == nil {
		sess.DNS1, sess.DNS2 = ParseGTDNS(dnsResp)
	}
	return sess, nil
}

func (c *Client) SetAPN(cid int, pdpType domain.PDPType, apn string) error {
	if err := c.EnsureConnected(); err != nil {
		return err
	}
	cmd := CmdCGDCONTSet(cid, pdpType, apn)
	resp, err := c.SendRaw(cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, ATResultOK) {
		return fmt.Errorf("%s failed: %s", cmd, resp)
	}
	return nil
}

func (c *Client) SetRATMode(pref domain.RATModePref) error {
	if err := c.EnsureConnected(); err != nil {
		return err
	}
	cmd := CmdGTACTSetByPref(pref)
	resp, err := c.SendRaw(cmd)
	if err != nil || !strings.Contains(resp, ATResultOK) {
		// Fallback to simple AT+GTACT=<code> if parameterized multi-argument command is rejected
		fallback := CmdGTACTSet(pref.GTACTCode())
		if fallback != cmd {
			resp2, err2 := c.SendRaw(fallback)
			if err2 == nil && strings.Contains(resp2, ATResultOK) {
				return nil
			}
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("%s failed: %s", cmd, resp)
	}
	return nil
}

// GetUSBMode queries AT+GTUSBMODE? (current USB composition profile).
func (c *Client) GetUSBMode() (int, error) {
	if err := c.EnsureConnected(); err != nil {
		return 0, err
	}
	resp, err := c.SendRaw(CmdGTUSBMODEQ)
	if err != nil {
		return 0, err
	}
	mode := ParseGTUSBMODE(resp)
	if mode == 0 && !strings.Contains(resp, ATResultOK) {
		return 0, fmt.Errorf("GTUSBMODE query failed: %s", resp)
	}
	return mode, nil
}

// SetUSBMode issues AT+GTUSBMODE=<mode>. Modem typically re-enumerates USB.
func (c *Client) SetUSBMode(mode int) error {
	if mode <= 0 {
		return fmt.Errorf("invalid USB mode %d", mode)
	}
	if err := c.EnsureConnected(); err != nil {
		return err
	}
	cmd := CmdGTUSBMODESet(mode)
	resp, err := c.SendRaw(cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, ATResultOK) {
		return fmt.Errorf("%s failed: %s", cmd, resp)
	}
	// Port will disappear on re-enumeration.
	_ = c.Close()
	return nil
}
