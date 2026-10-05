package vaultcluster

import (
	"reflect"
	"testing"
)

func TestAdminPolicies(t *testing.T) {
	tests := []struct {
		access  []string
		want    []string
		wantErr bool
	}{
		{access: []string{"tenants"}, want: []string{"provisioning"}},
		{access: []string{"operator"}, want: []string{"operator"}},
		{access: []string{"operator", "tenants", "tenants"}, want: []string{"operator", "provisioning"}},
		{access: nil, wantErr: true},
		{access: []string{"root"}, wantErr: true},
		{access: []string{"tenants", "kv"}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := AdminPolicies(tt.access)
		if (err != nil) != tt.wantErr {
			t.Fatalf("%v: err = %v", tt.access, err)
		}
		if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%v: got %v want %v", tt.access, got, tt.want)
		}
	}
}

func configuredVault(t *testing.T) *Client {
	t.Helper()
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	return c
}

// AppRole stands in for the cloud auth method: the token carries the same policies an admin-* role grants.
func TestAdminAccessLevels(t *testing.T) {
	c := configuredVault(t)
	if err := c.enableAuthMount("aws"); err != nil {
		t.Fatal(err)
	}
	login := func(access string) *Client {
		t.Helper()
		policies, err := AdminPolicies([]string{access})
		if err != nil {
			t.Fatal(err)
		}
		role := "admin-" + access
		if _, err := c.API.Logical().Write("auth/approle/role/"+role, map[string]any{
			"token_policies": policies, "token_ttl": AdminTokenTTL, "token_max_ttl": AdminTokenMaxTTL,
		}); err != nil {
			t.Fatal(err)
		}
		tok, err := c.LoginAppRole(role)
		if err != nil {
			t.Fatal(err)
		}
		return c.WithToken(tok)
	}
	tenants := login("tenants")
	operator := login("operator")

	status := func(cl *Client, method, path string, body any) int {
		t.Helper()
		r, err := cl.Do(method, path, body)
		if err != nil && r.Status == 0 {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return r.Status
	}

	for _, admin := range []*Client{tenants, operator} {
		if err := admin.Health(); err != nil {
			t.Errorf("admin health: %v", err)
		}
	}

	if err := tenants.CreateTenant("acme", false); err != nil {
		t.Fatalf("tenants admin create: %v", err)
	}
	writer, err := c.LoginAppRole(c.Cfg.TenantRole("writer", "acme"))
	if err != nil {
		t.Fatal(err)
	}
	if got := status(c.WithToken(writer), "POST", c.Cfg.KVDataPath("acme", "app"), map[string]any{"data": map[string]any{"k": "FAKE-v"}}); got != 200 {
		t.Fatalf("tenant writer: HTTP %d", got)
	}

	for _, admin := range []struct {
		name string
		cl   *Client
	}{{"tenants", tenants}, {"operator", operator}} {
		for _, p := range []string{c.Cfg.KVDataPath("acme", "app"), c.Cfg.KVMetaPath("acme", "") + "?list=true"} {
			if got := status(admin.cl, "GET", p, nil); got != 403 {
				t.Errorf("%s GET %s: HTTP %d, want 403", admin.name, p, got)
			}
		}
		if got := status(admin.cl, "PUT", "sys/policies/acl/admin-x", map[string]any{"policy": `path "*" { capabilities = ["sudo"] }`}); got != 403 {
			t.Errorf("%s write admin policy: HTTP %d, want 403", admin.name, got)
		}
		if got := status(admin.cl, "POST", "auth/aws/role/admin-x", map[string]any{"auth_type": "iam", "bound_iam_principal_arn": []string{"arn:aws:iam::123456789012:role/x-*"}}); got != 403 {
			t.Errorf("%s write admin role: HTTP %d, want 403", admin.name, got)
		}
	}

	if got := status(operator, "GET", "sys/storage/raft/snapshot", nil); got == 403 {
		t.Errorf("operator snapshot: HTTP 403")
	}
	if got := status(tenants, "GET", "sys/storage/raft/snapshot", nil); got != 403 {
		t.Errorf("tenants snapshot: HTTP %d, want 403", got)
	}
	if err := operator.CreateTenant("rogue", false); err == nil {
		t.Errorf("operator admin must not create tenants")
	}

	if err := tenants.OffboardTenant("acme", false); err != nil {
		t.Fatalf("tenants admin destroy: %v", err)
	}
	if err := tenants.OffboardTenant("acme", true); err == nil {
		t.Errorf("tenants admin must not purge tenant secrets")
	}
}
