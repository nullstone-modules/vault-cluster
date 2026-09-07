package vaultcluster

import (
	"net/http"
)

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
	return http.ListenAndServe(addr, c.HealthHandler(nodeID))
}
