package vaultcluster

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
)

type memKeyStore struct {
	init    *api.InitResponse
	loads   int
	failFor int
}

func (s *memKeyStore) SaveInit(resp *api.InitResponse) error {
	s.init = resp
	return nil
}

func (s *memKeyStore) LoadInit() (*api.InitResponse, error) {
	s.loads++
	if s.init == nil || s.loads <= s.failFor {
		return nil, fmt.Errorf("empty")
	}
	return s.init, nil
}

func (s *memKeyStore) SaveToken(string, string) error { return nil }
func (s *memKeyStore) LoadToken(string) (string, error) {
	return "", fmt.Errorf("empty")
}

func TestInitRequestAutoUnsealUsesRecovery(t *testing.T) {
	req := initRequest(BootstrapOptions{Shares: 1, Threshold: 1, AutoUnseal: true})
	if req.RecoveryShares != 1 || req.RecoveryThreshold != 1 {
		t.Fatalf("recovery: %+v", req)
	}
	if req.SecretShares != 0 || req.SecretThreshold != 0 {
		t.Fatalf("shamir should be unset: %+v", req)
	}
}

func TestInitRequestLocalUsesShamir(t *testing.T) {
	req := initRequest(BootstrapOptions{Shares: 5, Threshold: 3})
	if req.SecretShares != 5 || req.SecretThreshold != 3 {
		t.Fatalf("shamir: %+v", req)
	}
	if req.RecoveryShares != 0 {
		t.Fatalf("recovery should be unset: %+v", req)
	}
}

func TestWaitInitMaterialSucceedsAfterRetry(t *testing.T) {
	c := &Client{}
	store := &memKeyStore{init: &api.InitResponse{RootToken: "x"}, failFor: 1}
	if err := c.waitInitMaterial(store, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitInitMaterialTimesOut(t *testing.T) {
	c := &Client{}
	if err := c.waitInitMaterial(&memKeyStore{}, 10*time.Millisecond); err == nil {
		t.Fatal("expected timeout")
	}
}
