package vaultcluster

import (
	"net/http"
	"time"
)

// The NLB marks a probe failed after 6s. Answers slower than that are useless, and a
// stalled Vault must not pin probe connections open, so both sides are bounded below it.
const healthProbeTimeout = 5 * time.Second

func (c *Client) HealthHandler(nodeID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := c.NodeHealthOK(nodeID); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
}

func (c *Client) ServeHealth(addr, nodeID string) error {
	c.API.SetClientTimeout(healthProbeTimeout)
	srv := &http.Server{
		Addr:              addr,
		Handler:           c.HealthHandler(nodeID),
		ReadHeaderTimeout: healthProbeTimeout,
		ReadTimeout:       healthProbeTimeout,
		WriteTimeout:      healthProbeTimeout,
		IdleTimeout:       time.Minute,
	}
	return srv.ListenAndServe()
}
