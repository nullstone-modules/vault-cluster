package secretsmanager

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

func testStore(m memSecrets) KeyStore {
	return KeyStore{
		Secrets:         m,
		InitARN:         "arn:init",
		ProvisioningARN: "arn:provisioning",
		OperatorARN:     "arn:operator",
	}
}

func TestKeyStoreRoundTrip(t *testing.T) {
	store := testStore(memSecrets{})
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

func TestKeyStoreFailClosed(t *testing.T) {
	store := testStore(memSecrets{})
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

func TestNewRequiresARNs(t *testing.T) {
	if _, err := New("", "a", "b"); err == nil {
		t.Fatal("expected error")
	}
}
