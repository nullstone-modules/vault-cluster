package main

import "testing"

func TestParseTenantArgs(t *testing.T) {
	got, err := parseTenantArgs([]string{"acme", "--env", "pr-12", "--yes", "--purge-secrets"})
	if err != nil {
		t.Fatal(err)
	}
	if got != (tenantArgs{id: "acme", env: "pr-12", yes: true, purge: true}) {
		t.Fatalf("got %+v", got)
	}
	got, err = parseTenantArgs([]string{"--env=dev", "acme"})
	if err != nil || got.env != "dev" || got.id != "acme" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := parseTenantArgs([]string{"acme", "--env"}); err == nil {
		t.Fatal("--env without a value was accepted")
	}
	if _, err := parseTenantArgs([]string{"acme", "--force"}); err == nil {
		t.Fatal("unknown flag was accepted")
	}
}

func TestCheckEnvFlag(t *testing.T) {
	if err := checkEnvFlag(true, ""); err == nil {
		t.Fatal("shared cluster without --env was accepted")
	}
	if err := checkEnvFlag(false, "dev"); err == nil {
		t.Fatal("unshared cluster with --env was accepted")
	}
	if err := checkEnvFlag(true, "dev"); err != nil {
		t.Fatal(err)
	}
	if err := checkEnvFlag(false, ""); err != nil {
		t.Fatal(err)
	}
}
