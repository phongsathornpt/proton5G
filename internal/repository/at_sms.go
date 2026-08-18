package repository

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"fm350-monitor/internal/pkg/domain"
)

const (
	CmdCMGFText = "AT+CMGF=1"
	CmdCMGFPDU  = "AT+CMGF=0"
	CmdCSCSGSM  = "AT+CSCS=\"GSM\""
	CmdCSCSUCS2 = "AT+CSCS=\"UCS2\""
	CmdCNMISMS  = "AT+CNMI=2,1,0,1,0"
)

var (
	smsNumberRE   = regexp.MustCompile(`^\+?[0-9]{5,20}$`)
	smsConcatSeed atomic.Uint32
)

type CMSError struct {
	Code int
	Raw  string
}

func (e *CMSError) Error() string {
	if e.Code > 0 {
		return fmt.Sprintf("SMS modem error +CMS ERROR: %d", e.Code)
	}
	return "SMS modem error: " + e.Raw
}

func parseCMSError(resp string) error {
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CMS ERROR:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "+CMS ERROR:"))
		code, _ := strconv.Atoi(raw)
		return &CMSError{Code: code, Raw: raw}
	}
	return nil
}

func smsEncoding(body string) domain.SMSEncoding {
	for _, r := range body {
		if r < 0x20 || r > 0x7e {
			return domain.SMSEncodingUCS2
		}
	}
	return domain.SMSEncodingGSM7
}

func splitRunes(s string, single, multi int) []string {
	r := []rune(s)
	if len(r) <= single {
		return []string{s}
	}
	out := make([]string, 0, (len(r)+multi-1)/multi)
	for len(r) > 0 {
		n := multi
		if len(r) < n {
			n = len(r)
		}
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return out
}

// splitUCS2Units keeps each multipart segment within max UTF-16 code units.
// This matters for supplementary characters, which use two UTF-16 code units.
func splitUCS2Units(s string, max int) []string {
	if max <= 0 {
		return nil
	}
	var out []string
	var part []rune
	units := 0
	for _, r := range s {
		n := len(utf16.Encode([]rune{r}))
		if units+n > max && len(part) > 0 {
			out = append(out, string(part))
			part = nil
			units = 0
		}
		part = append(part, r)
		units += n
	}
	if len(part) > 0 {
		out = append(out, string(part))
	}
	return out
}

func ucs2Bytes(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(u16)*2)
	for _, v := range u16 {
		b = append(b, byte(v>>8), byte(v))
	}
	return b
}

func ucs2Hex(s string) string {
	return strings.ToUpper(hex.EncodeToString(ucs2Bytes(s)))
}

func semiOctetAddress(number string) (digits string, toa byte, encoded []byte, err error) {
	number = strings.TrimSpace(number)
	toa = 0x81
	if strings.HasPrefix(number, "+") {
		toa = 0x91
		number = strings.TrimPrefix(number, "+")
	}
	if number == "" || !regexp.MustCompile(`^[0-9]+$`).MatchString(number) {
		return "", 0, nil, fmt.Errorf("invalid SMS destination")
	}
	digits = number
	padded := number
	if len(padded)%2 != 0 {
		padded += "F"
	}
	encoded = make([]byte, 0, len(padded)/2)
	for i := 0; i < len(padded); i += 2 {
		hi, err1 := strconv.ParseUint(string(padded[i]), 16, 4)
		lo, err2 := strconv.ParseUint(string(padded[i+1]), 16, 4)
		if err1 != nil || err2 != nil {
			return "", 0, nil, fmt.Errorf("invalid SMS destination")
		}
		encoded = append(encoded, byte(lo<<4|hi))
	}
	return digits, toa, encoded, nil
}

// buildConcatUCS2PDU builds one SMS-SUBMIT TPDU with an 8-bit concatenation UDH.
// The returned length excludes the initial SMSC-length octet, as required by CMGS.
func buildConcatUCS2PDU(to, text string, ref, total, seq byte) (string, int, error) {
	digits, toa, addr, err := semiOctetAddress(to)
	if err != nil {
		return "", 0, err
	}
	if total < 2 || seq == 0 || seq > total {
		return "", 0, fmt.Errorf("invalid multipart SMS sequence")
	}
	udh := []byte{0x05, 0x00, 0x03, ref, total, seq}
	payload := append(udh, ucs2Bytes(text)...)
	if len(payload) > 140 {
		return "", 0, fmt.Errorf("multipart SMS segment exceeds 140 bytes")
	}

	pdu := make([]byte, 0, 1+6+len(addr)+len(payload))
	pdu = append(pdu, 0x00) // use modem-configured SMSC
	pdu = append(pdu, 0x41) // SMS-SUBMIT + UDHI
	pdu = append(pdu, 0x00) // TP-MR
	pdu = append(pdu, byte(len(digits)))
	pdu = append(pdu, toa)
	pdu = append(pdu, addr...)
	pdu = append(pdu, 0x00) // PID
	pdu = append(pdu, 0x08) // UCS2 DCS
	pdu = append(pdu, byte(len(payload)))
	pdu = append(pdu, payload...)
	return strings.ToUpper(hex.EncodeToString(pdu)), len(pdu) - 1, nil
}

func (c *Client) enableSMSURCs() {
	_, _ = c.SendRaw(CmdCNMISMS)
}

func (c *Client) ListSMS() ([]domain.SMSMessage, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}
	if resp, err := c.SendRaw(CmdCMGFText); err != nil || !strings.Contains(resp, ATResultOK) {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("CMGF failed: %s", resp)
	}
	_, _ = c.SendRaw(CmdCSCSGSM)
	c.enableSMSURCs()
	resp, err := c.SendRaw(`AT+CMGL="ALL"`)
	if err != nil {
		return nil, err
	}
	if cms := parseCMSError(resp); cms != nil {
		return nil, cms
	}
	return ParseCMGL(resp), nil
}

func (c *Client) ReadSMS(index int) (domain.SMSMessage, error) {
	if index < 0 {
		return domain.SMSMessage{}, fmt.Errorf("invalid SMS index %d", index)
	}
	if err := c.EnsureConnected(); err != nil {
		return domain.SMSMessage{}, err
	}
	_, _ = c.SendRaw(CmdCMGFText)
	_, _ = c.SendRaw(CmdCSCSGSM)
	c.enableSMSURCs()
	resp, err := c.SendRaw(fmt.Sprintf("AT+CMGR=%d", index))
	if err != nil {
		return domain.SMSMessage{}, err
	}
	if cms := parseCMSError(resp); cms != nil {
		return domain.SMSMessage{}, cms
	}
	msg, ok := ParseCMGR(index, resp)
	if !ok {
		return domain.SMSMessage{}, fmt.Errorf("SMS %d not found", index)
	}
	return msg, nil
}

func (c *Client) DeleteSMS(index int) error {
	if index < 0 {
		return fmt.Errorf("invalid SMS index %d", index)
	}
	if err := c.EnsureConnected(); err != nil {
		return err
	}
	resp, err := c.SendRaw(fmt.Sprintf("AT+CMGD=%d", index))
	if err != nil {
		return err
	}
	if cms := parseCMSError(resp); cms != nil {
		return cms
	}
	if !strings.Contains(resp, ATResultOK) {
		return fmt.Errorf("CMGD failed: %s", resp)
	}
	return nil
}

func (c *Client) SendSMS(req domain.SMSSendRequest) (domain.SMSSendResult, error) {
	var result domain.SMSSendResult
	to := strings.TrimSpace(req.To)
	body := req.Body
	if !smsNumberRE.MatchString(to) {
		return result, fmt.Errorf("invalid SMS destination")
	}
	if strings.TrimSpace(body) == "" {
		return result, fmt.Errorf("SMS body is empty")
	}

	enc := smsEncoding(body)
	runeCount := len([]rune(body))
	needsMultipart := (enc == domain.SMSEncodingGSM7 && runeCount > 160) ||
		(enc == domain.SMSEncodingUCS2 && len(utf16.Encode([]rune(body))) > 70)

	if err := c.EnsureConnected(); err != nil {
		return result, err
	}
	c.enableSMSURCs()

	if needsMultipart {
		parts := splitUCS2Units(body, 67)
		if len(parts) > 255 {
			return result, fmt.Errorf("SMS requires too many segments")
		}
		if resp, err := c.SendRaw(CmdCMGFPDU); err != nil || !strings.Contains(resp, ATResultOK) {
			if err != nil {
				return result, err
			}
			return result, fmt.Errorf("CMGF PDU failed: %s", resp)
		}
		ref := byte(smsConcatSeed.Add(1))
		if ref == 0 {
			ref = 1
		}
		for i, part := range parts {
			pdu, tpduLen, err := buildConcatUCS2PDU(to, part, ref, byte(len(parts)), byte(i+1))
			if err != nil {
				return result, err
			}
			mr, err := c.submitCMGS(fmt.Sprintf("AT+CMGS=%d", tpduLen), pdu)
			if err != nil {
				return result, err
			}
			result.MessageRefs = append(result.MessageRefs, mr)
		}
		result.Encoding = domain.SMSEncodingUCS2
		result.Segments = len(parts)
		result.SubmittedAt = time.Now().UTC()
		return result, nil
	}

	charset := CmdCSCSGSM
	payload := body
	address := to
	if enc == domain.SMSEncodingUCS2 {
		charset = CmdCSCSUCS2
		address = ucs2Hex(to)
		payload = ucs2Hex(body)
	}
	if resp, err := c.SendRaw(CmdCMGFText); err != nil || !strings.Contains(resp, ATResultOK) {
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("CMGF failed: %s", resp)
	}
	if resp, err := c.SendRaw(charset); err != nil || !strings.Contains(resp, ATResultOK) {
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("CSCS failed: %s", resp)
	}
	mr, err := c.submitCMGS(fmt.Sprintf("AT+CMGS=\"%s\"", address), payload)
	if err != nil {
		return result, err
	}
	result.MessageRefs = []int{mr}
	result.Encoding = enc
	result.Segments = 1
	result.SubmittedAt = time.Now().UTC()
	return result, nil
}

func (c *Client) submitCMGS(command, payload string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port == nil {
		return 0, fmt.Errorf("serial port not connected")
	}
	if _, err := c.port.Write([]byte(strings.TrimSpace(command) + "\r")); err != nil {
		return 0, fmt.Errorf("write CMGS: %w", err)
	}

	buf := make([]byte, 256)
	var resp []byte
	promptDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(promptDeadline) {
		n, err := c.port.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			if strings.Contains(string(resp), ">") {
				break
			}
			if cms := parseCMSError(string(resp)); cms != nil {
				return 0, cms
			}
		}
		if err != nil && n == 0 {
			continue
		}
	}
	if !strings.Contains(string(resp), ">") {
		return 0, fmt.Errorf("SMS submit prompt timeout: %s", strings.TrimSpace(string(resp)))
	}

	if _, err := c.port.Write(append([]byte(payload), 0x1a)); err != nil {
		return 0, fmt.Errorf("write SMS payload: %w", err)
	}
	resp = resp[:0]
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		n, err := c.port.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			s := string(resp)
			if cms := parseCMSError(s); cms != nil {
				return 0, cms
			}
			if strings.Contains(s, "\r\nOK") || strings.HasSuffix(strings.TrimSpace(s), "OK") {
				return parseCMGSRef(s), nil
			}
			if strings.Contains(s, ATResultERROR) {
				return 0, fmt.Errorf("CMGS failed: %s", strings.TrimSpace(s))
			}
		}
		if err != nil && n == 0 {
			continue
		}
	}
	return 0, fmt.Errorf("SMS submit timeout: %s", strings.TrimSpace(string(resp)))
}

func parseCMGSRef(resp string) int {
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CMGS:") {
			v, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "+CMGS:")))
			return v
		}
	}
	return 0
}

func ParseCMGL(resp string) []domain.SMSMessage {
	lines := strings.Split(strings.ReplaceAll(resp, "\r", ""), "\n")
	out := make([]domain.SMSMessage, 0)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "+CMGL:") {
			continue
		}
		msg, ok := parseSMSHeader(strings.TrimPrefix(line, "+CMGL:"), true)
		if !ok {
			continue
		}
		if i+1 < len(lines) {
			msg.Body = strings.TrimSpace(lines[i+1])
			i++
		}
		out = append(out, msg)
	}
	return out
}

func ParseCMGR(index int, resp string) (domain.SMSMessage, bool) {
	lines := strings.Split(strings.ReplaceAll(resp, "\r", ""), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CMGR:") {
			continue
		}
		msg, ok := parseSMSHeader(strings.TrimPrefix(line, "+CMGR:"), false)
		if !ok {
			return domain.SMSMessage{}, false
		}
		msg.Index = index
		if i+1 < len(lines) {
			msg.Body = strings.TrimSpace(lines[i+1])
		}
		return msg, true
	}
	return domain.SMSMessage{}, false
}

func parseSMSHeader(raw string, hasIndex bool) (domain.SMSMessage, bool) {
	fields := splitCSVQuoted(raw)
	min := 3
	if hasIndex {
		min = 4
	}
	if len(fields) < min {
		return domain.SMSMessage{}, false
	}
	msg := domain.SMSMessage{Encoding: domain.SMSEncodingGSM7}
	off := 0
	if hasIndex {
		msg.Index, _ = strconv.Atoi(strings.TrimSpace(fields[0]))
		off = 1
	}
	msg.Status = domain.SMSStatus(strings.Trim(fields[off], "\" "))
	msg.Address = strings.Trim(fields[off+1], "\" ")
	if len(fields) > off+3 {
		msg.Timestamp = strings.Trim(fields[off+3], "\" ")
	}
	return msg, true
}

func splitCSVQuoted(s string) []string {
	var out []string
	var b strings.Builder
	quoted := false
	for _, r := range s {
		switch r {
		case '"':
			quoted = !quoted
			b.WriteRune(r)
		case ',':
			if quoted {
				b.WriteRune(r)
			} else {
				out = append(out, strings.TrimSpace(b.String()))
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	out = append(out, strings.TrimSpace(b.String()))
	return out
}
