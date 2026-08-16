package repository

import (
	"fmt"
	"strings"

	"fm350-monitor/internal/pkg/domain"
)

// DeactivatePDP tears down a PDP context. It is an optional concrete capability
// used by the WAN facade without widening the base ATRepository interface.
func (c *Client) DeactivatePDP(cid int) error {
	if cid <= 0 {
		cid = 1
	}
	if err := c.EnsureConnected(); err != nil {
		return err
	}
	cmd := CmdCGACTSet(cid, false)
	resp, err := c.SendRaw(cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, ATResultOK) {
		return fmt.Errorf("%s failed: %s", cmd, resp)
	}
	return nil
}

func (c *TieredClient) DeactivatePDP(cid int) error {
	if err := c.Client.DeactivatePDP(cid); err != nil {
		return err
	}
	if cid <= 0 {
		cid = 1
	}
	c.cacheMu.Lock()
	c.cache.pdp = domain.PDPSession{CID: cid}
	c.cache.mediumAt = c.cache.mediumAt.Add(-atMediumPollInterval)
	c.cacheMu.Unlock()
	return nil
}
