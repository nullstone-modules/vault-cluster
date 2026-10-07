package vaultcluster

import "testing"

func TestTenantRole(t *testing.T) {
	cfg := Config{KVMount: "kv", TenantPrefix: "customers", EnvPrefix: "envs"}
	if got := cfg.TenantRole("", "acme"); got != "acme" {
		t.Fatalf("unscoped role = %q", got)
	}
	if got := cfg.TenantRole("pr-12", "acme"); got != "pr-12.acme" {
		t.Fatalf("scoped role = %q", got)
	}
	for _, tt := range []struct{ role, env, id string }{
		{"acme", "", "acme"}, {"pr-12.acme", "pr-12", "acme"}, {"dev.a.b", "dev", "a.b"},
	} {
		env, id := SplitTenantRole(tt.role)
		if env != tt.env || id != tt.id {
			t.Fatalf("SplitTenantRole(%q) = %q, %q", tt.role, env, id)
		}
	}
	if got := cfg.KVDataPath("", "acme", "cfg"); got != "kv/data/customers/acme/cfg" {
		t.Fatalf("unscoped data path = %q", got)
	}
	if got := cfg.KVDataPath("pr-12", "acme", "/cfg"); got != "kv/data/envs/pr-12/customers/acme/cfg" {
		t.Fatalf("scoped data path = %q", got)
	}
	if got := cfg.KVMetaPath("pr-12", "acme", ""); got != "kv/metadata/envs/pr-12/customers/acme" {
		t.Fatalf("scoped metadata path = %q", got)
	}
}

func TestCheckEnv(t *testing.T) {
	shared, unshared := Config{SharedEnvs: true}, Config{}
	if err := shared.checkEnv(""); err == nil {
		t.Fatal("shared cluster accepted an empty env")
	}
	if err := shared.checkEnv("pr-12"); err != nil {
		t.Fatal(err)
	}
	if err := shared.checkEnv("a.b"); err == nil {
		t.Fatal("shared cluster accepted an env with a dot")
	}
	if err := unshared.checkEnv("pr-12"); err == nil {
		t.Fatal("unshared cluster accepted an env")
	}
	if err := unshared.checkEnv(""); err != nil {
		t.Fatal(err)
	}
}

func TestValidateEnvName(t *testing.T) {
	for _, env := range []string{"dev", "prod", "pr-123", "previews-shared", "sys", "data"} {
		if err := ValidateEnvName(env); err != nil {
			t.Errorf("%s: %v", env, err)
		}
	}
	for _, env := range []string{"", "a.b", "a/b", "pr-*", "PR", "x", "-x", "x-", "a b"} {
		if err := ValidateEnvName(env); err == nil {
			t.Errorf("%q should have been rejected", env)
		}
	}
	if err := ValidateTenantID("envs"); err == nil {
		t.Error("envs must be a reserved tenant ID")
	}
}
