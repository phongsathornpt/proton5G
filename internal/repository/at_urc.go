package repository

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"fm350-monitor/internal/pkg/domain"
)

var clientURCs sync.Map // map[*Client]*urcQueue

type urcQueue struct {
	mu    sync.Mutex
	items []string
}

func (c *Client) urcQueue() *urcQueue {
	if v, ok := clientURCs.Load(c); ok {
		return v.(*urcQueue)
	}
	q := &urcQueue{}
	actual, _ := clientURCs.LoadOrStore(c, q)
	return actual.(*urcQueue)
}

func isURCLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "+CMTI:") ||
		strings.HasPrefix(line, "+CMT:") ||
		strings.HasPrefix(line, "+CDS:")
}

// storeAndStripSMSURCs removes SMS unsolicited result-code lines from a command
// response while retaining them for consumers. This keeps polling responses clean
// without resetting the serial input buffer and losing inbound notifications.
func (c *Client) storeAndStripSMSURCs(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	kept := make([]string, 0, len(lines))
	q := c.urcQueue()
	for _, line := range lines {
		if isURCLine(line) {
			q.mu.Lock()
			if len(q.items) >= 128 {
				q.items = append(q.items[:0], q.items[len(q.items)-127:]...)
			}
			q.items = append(q.items, strings.TrimSpace(line))
			q.mu.Unlock()
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func (c *Client) DrainSMSURCs() []string {
	q := c.urcQueue()
	q.mu.Lock()
	defer q.mu.Unlock()
	out := append([]string(nil), q.items...)
	q.items = nil
	return out
}

func (c *Client) DrainSMSNotifications() []domain.SMSNotification {
	raw := c.DrainSMSURCs()
	out := make([]domain.SMSNotification, 0, len(raw))
	now := time.Now().UTC()
	for _, line := range raw {
		n := domain.SMSNotification{Raw: line, ReceivedAt: now}
		switch {
		case strings.HasPrefix(line, "+CMTI:"):
			n.Type = domain.SMSNotificationStored
			fields := splitCSVQuoted(strings.TrimSpace(strings.TrimPrefix(line, "+CMTI:")))
			if len(fields) >= 2 {
				n.Storage = strings.Trim(fields[0], "\" ")
				n.Index, _ = strconv.Atoi(strings.TrimSpace(fields[1]))
			}
		case strings.HasPrefix(line, "+CDS:"):
			n.Type = domain.SMSNotificationDeliveryReport
			// In text mode many modems include the message reference as the first
			// numeric field; retain the raw URC as the authoritative representation.
			fields := splitCSVQuoted(strings.TrimSpace(strings.TrimPrefix(line, "+CDS:")))
			if len(fields) > 0 {
				n.MessageRef, _ = strconv.Atoi(strings.Trim(fields[0], "\" "))
			}
		case strings.HasPrefix(line, "+CMT:"):
			n.Type = domain.SMSNotificationDirect
		default:
			continue
		}
		out = append(out, n)
	}
	return out
}
