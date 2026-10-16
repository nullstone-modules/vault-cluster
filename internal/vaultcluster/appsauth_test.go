package vaultcluster

import (
	"strings"
	"testing"
)

// Configure mounts auth/aws and binds the cluster function's role to apps-auth, and does it again without
// error. The ARN ends in a wildcard so Vault does not resolve it through IAM, which the test host cannot reach.
func TestConfigureBindsAppsAuthRole(t *testing.T) {
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	c.Cfg.AdminAuthMethod = "aws"
	c.Cfg.AdminPrincipal = "arn:aws:iam::123456789012:role/vault-apps-auth*"
	for i := 0; i < 2; i++ {
		if err := c.Configure(); err != nil {
			t.Fatalf("configure run %d: %v", i+1, err)
		}
	}
	auths, err := c.API.Sys().ListAuth()
	if err != nil {
		t.Fatal(err)
	}
	if a := auths["aws/"]; a == nil || a.Type != "aws" {
		t.Fatalf("auth/aws is not mounted: %v", auths)
	}
	role, err := c.API.Logical().Read("auth/aws/role/" + AppsAuthRole)
	if err != nil || role == nil {
		t.Fatalf("read role: %v", err)
	}
	bound, _ := role.Data["bound_iam_principal_arn"].([]any)
	policies, _ := role.Data["token_policies"].([]any)
	if len(bound) != 1 || bound[0] != c.Cfg.AdminPrincipal || len(policies) != 1 || policies[0] != "apps-auth" {
		t.Fatalf("role = %v", role.Data)
	}
	if ttl, _ := role.Data["token_ttl"].(interface{ String() string }); ttl != nil && ttl.String() != "300" {
		t.Fatalf("token_ttl = %v", role.Data["token_ttl"])
	}
	if role.Data["auth_type"] != "iam" {
		t.Fatalf("auth_type = %v", role.Data["auth_type"])
	}
}

// Without a declared method (local stack) Configure mounts nothing.
func TestConfigureSkipsAppsAuthWithoutMethod(t *testing.T) {
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	auths, err := c.API.Sys().ListAuth()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := auths["aws/"]; ok {
		t.Fatal("auth/aws was mounted without VAULT_ADMIN_AUTH_METHOD")
	}
}

// The apps-auth policy no longer owns the aws and gcp mounts: no sys/auth/<mount> grant, no sudo.
func TestAppsAuthPolicyDoesNotOwnMounts(t *testing.T) {
	for _, shared := range []bool{false, true} {
		hcl, err := RenderPolicy("apps-auth", goldenConfig(shared), goldenAccessors)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{`"sys/auth/aws"`, `"sys/auth/gcp"`, `"sudo"`} {
			if strings.Contains(hcl, bad) {
				t.Fatalf("apps-auth (shared=%v) still grants %s:\n%s", shared, bad, hcl)
			}
		}
		for _, want := range []string{`path "auth/aws/role/*"`, `path "auth/gcp/role/*"`, `path "auth/token/lookup-self"`, `path "auth/token/renew-self"`} {
			if !strings.Contains(hcl, want) {
				t.Fatalf("apps-auth (shared=%v) lost %s:\n%s", shared, want, hcl)
			}
		}
	}
}

// A declared method the table does not know, or a method without a principal, is refused before any Vault call.
func TestEnableAppsAuthRefusesBadDeclarations(t *testing.T) {
	for _, cfg := range []Config{
		{AdminAuthMethod: "azure", AdminPrincipal: "x"},
		{AdminAuthMethod: "aws"},
	} {
		c := &Client{Cfg: cfg}
		if err := c.enableAppsAuth(); err == nil {
			t.Fatalf("%+v was accepted", cfg)
		}
	}
	if body := adminAuthMethods["gcp"].roleBody("fn@proj.iam.gserviceaccount.com"); body["type"] != "iam" || body["bound_service_accounts"].([]string)[0] != "fn@proj.iam.gserviceaccount.com" {
		t.Fatalf("gcp body = %v", body)
	}
}
