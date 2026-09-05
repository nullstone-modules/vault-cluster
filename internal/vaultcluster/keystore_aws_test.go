package vaultcluster

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/vault/api"
)

type memSecrets map[string][]byte

func (m memSecrets) Get(_ context.Context, arn string) ([]byte, error) {
	b, ok := m[arn]
	if !ok {
		return nil, errors.New("missing")
	}
	return b, nil
}

func (m memSecrets) Put(_ context.Context, arn string, val []byte) error {
	if arn == "fail" {
		return errors.New("denied")
	}
	m[arn] = append([]byte(nil), val...)
	return nil
}

func testSMStore(m memSecrets) SecretsManagerKeyStore {
	return SecretsManagerKeyStore{
		Secrets:         m,
		InitARN:         "arn:init",
		ProvisioningARN: "arn:provisioning",
		OperatorARN:     "arn:operator",
	}
}

func TestSecretsManagerKeyStoreRoundTrip(t *testing.T) {
	store := testSMStore(memSecrets{})
	init := &api.InitResponse{RootToken: "hvs.root", RecoveryKeysB64: []string{"abc"}}
	if err := store.SaveInit(init); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadInit()
	if err != nil {
		t.Fatal(err)
	}
	if got.RootToken != "hvs.root" || len(got.RecoveryKeysB64) != 1 {
		t.Fatalf("init mismatch: %+v", got)
	}
	if err := store.SaveToken("operator", "hvs.op"); err != nil {
		t.Fatal(err)
	}
	tok, err := store.LoadToken("operator")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "hvs.op" {
		t.Fatalf("token %q", tok)
	}
}

func TestSecretsManagerKeyStoreFailClosed(t *testing.T) {
	store := testSMStore(memSecrets{})
	if _, err := store.LoadInit(); err == nil {
		t.Fatal("expected missing init to fail")
	}
	if _, err := store.LoadToken("operator"); err == nil {
		t.Fatal("expected missing token to fail")
	}
	if err := store.SaveToken("root", "x"); err == nil {
		t.Fatal("expected unknown token name to fail")
	}
	store.InitARN = "fail"
	if err := store.SaveInit(&api.InitResponse{RootToken: "x"}); err == nil {
		t.Fatal("expected put failure")
	}
}

func TestNewSecretsManagerKeyStoreRequiresARNs(t *testing.T) {
	if _, err := NewSecretsManagerKeyStore("", "a", "b"); err == nil {
		t.Fatal("expected error")
	}
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
