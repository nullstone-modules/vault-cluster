package vaultcluster

import (
	"log"
	"time"
)

// RenewToken keeps the client's periodic token alive for long-running commands.
// Bootstrap issues 24h periodic tokens, which expire unless renewed within the period.
// A failed renewal is logged and retried on the next tick; the caller decides the interval.
func (c *Client) RenewToken(every time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if _, err := c.API.Auth().Token().RenewSelf(0); err != nil {
				log.Printf("token renewal failed: %v", err)
			}
		}
	}
}
