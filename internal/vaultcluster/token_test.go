package vaultcluster

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRenewTokenCallsRenewSelf(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/token/renew-self" || r.Header.Get("X-Vault-Token") != "tok" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"auth":{"client_token":"tok","renewable":true,"lease_duration":86400}}`))
	}))
	defer srv.Close()

	c, err := New(Config{Addr: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		c.RenewToken(10*time.Millisecond, stop)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	<-done
	if calls.Load() < 2 {
		t.Fatalf("expected at least 2 renewals, got %d", calls.Load())
	}
}
