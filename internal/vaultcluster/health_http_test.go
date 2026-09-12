package vaultcluster

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthHandlerUnavailable(t *testing.T) {
	c, err := New(Config{Addr: "http://127.0.0.1:1", HTTPTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c.HealthHandler("n1").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}
