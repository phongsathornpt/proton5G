package repository

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"fm350-monitor/internal/pkg/domain"
)

const (
	CmdCMGFText = "AT+CMGF=1"
	CmdCSCSGSM  = "AT+CSCS=\"GSM\""
	CmdCSCSUCS2 = "AT+CSCS=\"UCS2\""
)

var smsNumberRE = regexp.MustCompile(`^\+?[0-9]{5,20}$`)

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

func ucs2Hex(s string) string {
	u16 := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(u16)*2)
	for _, v := range u16 {
		b = append(b, byte(v>>8), byte(v))
	}
	return strings.ToUpper(hex.EncodeToString(b))
}

func (c *Client) ListSMS() ([]domain.SMSMessage, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}
	if resp, err := c.SendRaw(CmdCMGFText); err != nil || !strings.Contains(resp, ATResultOK) {
		if err != nil { return nil, err }
		return nil, fmt.Errorf("CMGF failed: %s", resp)
	}
	_, _ = c.SendRaw(CmdCSCSGSM)
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
	parts := splitRunes(body, 160, 153)
	charset := CmdCSCSGSM
	if enc == domain.SMSEncodingUCS2 {
		parts = splitRunes(body, 70, 67)
		charset = CmdCSCSUCS2
	}

	if err := c.EnsureConnected(); err != nil {
		return result, err
	}
	if resp, err := c.SendRaw(CmdCMGFText); err != nil || !strings.Contains(resp, ATResultOK) {
		if err != nil { return result, err }
		return result, fmt.Errorf("CMGF failed: %s", resp)
	}
	if resp, err := c.SendRaw(charset); err != nil || !strings.Contains(resp, ATResultOK) {
		if err != nil { return result, err }
		return result, fmt.Errorf("CSCS failed: %s", resp)
	}

	for _, part := range parts {
		addr := to
		payload := part
		if enc == domain.SMSEncodingUCS2 {
			addr = ucs2Hex(to)
			payload = ucs2Hex(part)
		}
		mr, err := c.sendSMSPart(addr, payload)
		if err != nil {
			return result, err
		}
		result.MessageRefs = append(result.MessageRefs, mr)
	}
	result.Encoding = enc
	result.Segments = len(parts)
	result.SubmittedAt = time.Now().UTC()
	return result, nil
}

func (c *Client) sendSMSPart(address, payload string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port == nil {
		return 0, fmt.Errorf("serial port not connected")
	}

	cmd := fmt.Sprintf("AT+CMGS=\"%s\"\r", address)
	if _, err := c.port.Write([]byte(cmd)); err != nil {
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
		if !ok { continue }
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
		if !strings.HasPrefix(line, "+CMGR:") { continue }
		msg, ok := parseSMSHeader(strings.TrimPrefix(line, "+CMGR:"), false)
		if !ok { return domain.SMSMessage{}, false }
		msg.Index = index
		if i+1 < len(lines) { msg.Body = strings.TrimSpace(lines[i+1]) }
		return msg, true
	}
	return domain.SMSMessage{}, false
}

func parseSMSHeader(raw string, hasIndex bool) (domain.SMSMessage, bool) {
	fields := splitCSVQuoted(raw)
	min := 3
	if hasIndex { min = 4 }
	if len(fields) < min { return domain.SMSMessage{}, false }
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
			if quoted { b.WriteRune(r) } else { out = append(out, strings.TrimSpace(b.String())); b.Reset() }
		default:
			b.WriteRune(r)
		}
	}
	out = append(out, strings.TrimSpace(b.String()))
	return out
}
