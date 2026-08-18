package repository

import (
	"strings"
	"testing"

	"fm350-monitor/internal/pkg/domain"
)

func TestParseCMGL(t *testing.T) {
	resp := "+CMGL: 3,\"REC UNREAD\",\"+66812345678\",\"\",\"26/08/19,03:00:00+28\"\r\nhello\r\n+CMGL: 4,\"REC READ\",\"+66999999999\",\"\",\"26/08/19,03:01:00+28\"\r\nworld\r\nOK\r\n"
	msgs := ParseCMGL(resp)
	if len(msgs) != 2 { t.Fatalf("got %d messages", len(msgs)) }
	if msgs[0].Index != 3 || msgs[0].Address != "+66812345678" || msgs[0].Body != "hello" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
}

func TestParseCMGR(t *testing.T) {
	msg, ok := ParseCMGR(7, "+CMGR: \"REC READ\",\"+66812345678\",\"\",\"26/08/19,03:00:00+28\"\r\nสวัสดี\r\nOK\r\n")
	if !ok { t.Fatal("expected message") }
	if msg.Index != 7 || msg.Body != "สวัสดี" { t.Fatalf("unexpected message: %+v", msg) }
}

func TestSMSEncodingAndSegments(t *testing.T) {
	if got := smsEncoding("hello"); got != domain.SMSEncodingGSM7 { t.Fatalf("got %s", got) }
	if got := smsEncoding("สวัสดี"); got != domain.SMSEncodingUCS2 { t.Fatalf("got %s", got) }
	parts := splitRunes(strings.Repeat("x", 161), 160, 153)
	if len(parts) != 2 || len([]rune(parts[0])) != 153 { t.Fatalf("unexpected segmentation: %#v", parts) }
	thai := splitRunes(strings.Repeat("ก", 71), 70, 67)
	if len(thai) != 2 || len([]rune(thai[0])) != 67 { t.Fatalf("unexpected UCS2 segmentation") }
}

func TestUCS2Hex(t *testing.T) {
	if got := ucs2Hex("ก"); got != "0E01" { t.Fatalf("got %s", got) }
}

func TestParseCMSError(t *testing.T) {
	err := parseCMSError("\r\n+CMS ERROR: 500\r\n")
	cms, ok := err.(*CMSError)
	if !ok || cms.Code != 500 { t.Fatalf("unexpected error: %#v", err) }
}

func TestStoreAndStripSMSURCs(t *testing.T) {
	c := NewClient("test")
	clean := c.storeAndStripSMSURCs("+CMTI: \"SM\",2\r\n+CSQ: 20,99\r\nOK\r\n")
	if strings.Contains(clean, "+CMTI") { t.Fatalf("URC leaked into response: %q", clean) }
	urcs := c.DrainSMSURCs()
	if len(urcs) != 1 || !strings.HasPrefix(urcs[0], "+CMTI:") { t.Fatalf("unexpected URCs: %#v", urcs) }
}
