package repository

import (
	"strings"
	"sync"
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
