package repository

import "fm350-monitor/internal/pkg/domain"

// ActivatePDP invalidates medium-tier PDP state so the next background sample
// observes the newly activated bearer immediately instead of waiting up to 10s.
func (c *TieredClient) ActivatePDP(cid int) error {
	if err := c.Client.ActivatePDP(cid); err != nil {
		return err
	}
	c.cacheMu.Lock()
	c.cache.mediumAt = c.cache.mediumAt.Add(-atMediumPollInterval)
	c.cacheMu.Unlock()
	return nil
}

// QueryPDP keeps explicit control-path queries and the background cache aligned.
func (c *TieredClient) QueryPDP(cid int) (domain.PDPSession, error) {
	sess, err := c.Client.QueryPDP(cid)
	if err != nil {
		return sess, err
	}
	c.cacheMu.Lock()
	c.cache.pdp = sess
	c.cache.mediumAt = c.now()
	c.cacheMu.Unlock()
	return sess, nil
}
