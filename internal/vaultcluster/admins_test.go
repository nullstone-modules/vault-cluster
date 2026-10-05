package vaultcluster

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestValidateAdminBindings(t *testing.T) {
	user := "arn:aws:iam::123456789012:user/brad"
	sso := "arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/AWSReservedSSO_VaultAdmin_*"
	tests := []struct {
		name     string
		bindings []AdminBinding
		wantErr  string
	}{
		{name: "empty", bindings: nil},
		{name: "user and sso", bindings: []AdminBinding{
			{Name: "admin-brad", PrincipalARN: user, Access: []string{"tenants", "operator"}},
			{Name: "admin-sso", PrincipalARN: sso, Access: []string{"tenants"}},
		}},
		{name: "missing prefix", bindings: []AdminBinding{{Name: "brad", PrincipalARN: user, Access: []string{"tenants"}}}, wantErr: "invalid admin role name"},
		{name: "duplicate name", bindings: []AdminBinding{
			{Name: "admin-a", PrincipalARN: user, Access: []string{"tenants"}},
			{Name: "admin-a", PrincipalARN: sso, Access: []string{"tenants"}},
		}, wantErr: "duplicate"},
		{name: "duplicate principal", bindings: []AdminBinding{
			{Name: "admin-a", PrincipalARN: user, Access: []string{"tenants"}},
			{Name: "admin-b", PrincipalARN: user, Access: []string{"operator"}},
		}, wantErr: "already bound"},
		{name: "account wildcard", bindings: []AdminBinding{{Name: "admin-a", PrincipalARN: "arn:aws:iam::123456789012:*", Access: []string{"tenants"}}}, wantErr: "IAM user or role"},
		{name: "group", bindings: []AdminBinding{{Name: "admin-a", PrincipalARN: "arn:aws:iam::123456789012:group/admins", Access: []string{"tenants"}}}, wantErr: "IAM user or role"},
		{name: "unknown access", bindings: []AdminBinding{{Name: "admin-a", PrincipalARN: user, Access: []string{"root"}}}, wantErr: "unknown access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAdminBindings(tt.bindings)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
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

// Wildcard ARNs keep Vault from calling IAM, so this runs without AWS.
func TestReconcileAdmins(t *testing.T) {
	c := configuredVault(t)
	tok, err := c.issueOrphanToken("aws-auth")
	if err != nil {
		t.Fatal(err)
	}
	awsAuth := c.WithToken(tok)

	app := "arn:aws:iam::123456789012:role/app-*"
	if err := awsAuth.enableAWSAuth(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Must("POST", "auth/aws/role/billing", map[string]any{
		"auth_type": "iam", "bound_iam_principal_arn": []string{app}, "token_policies": []string{"app"},
	}); err != nil {
		t.Fatal(err)
	}

	first := []AdminBinding{
		{Name: "admin-tenants", PrincipalARN: "arn:aws:iam::123456789012:role/vault-tenants-*", Access: []string{"tenants"}},
		{Name: "admin-old", PrincipalARN: "arn:aws:iam::123456789012:role/old-*", Access: []string{"operator"}},
	}
	if err := awsAuth.ReconcileAdmins(first); err != nil {
		t.Fatal(err)
	}
	role := readAWSRole(t, c, "admin-tenants")
	if !reflect.DeepEqual(role.Policies, []string{"provisioning"}) || role.TTL != 3600 || role.MaxTTL != 28800 || role.Period != 0 {
		t.Fatalf("admin-tenants = %+v", role)
	}

	second := []AdminBinding{
		{Name: "admin-tenants", PrincipalARN: "arn:aws:iam::123456789012:role/vault-tenants-*", Access: []string{"tenants", "operator"}},
		{Name: "admin-thief", PrincipalARN: app, Access: []string{"operator"}},
	}
	err = awsAuth.ReconcileAdmins(second)
	if err == nil || !strings.Contains(err.Error(), "already bound to vault role billing") {
		t.Fatalf("expected the app principal to be refused, got %v", err)
	}
	if got := readAWSRole(t, c, "admin-tenants").Policies; !reflect.DeepEqual(got, []string{"operator", "provisioning"}) {
		t.Fatalf("admin-tenants policies = %v", got)
	}
	for _, gone := range []string{"admin-old", "admin-thief"} {
		if r, _ := c.Do("GET", "auth/aws/role/"+gone, nil); r.Status != 404 {
			t.Fatalf("%s: HTTP %d, want 404", gone, r.Status)
		}
	}
	if r, _ := c.Do("GET", "auth/aws/role/billing", nil); r.Status != 200 {
		t.Fatalf("app role must be untouched, HTTP %d", r.Status)
	}

	if err := awsAuth.ReconcileAdmins(nil); err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Do("GET", "auth/aws/role/admin-tenants", nil); r.Status != 404 {
		t.Fatalf("empty bindings must revoke admin-tenants, HTTP %d", r.Status)
	}
}

type awsRole struct {
	Policies []string `json:"token_policies"`
	TTL      int      `json:"token_ttl"`
	MaxTTL   int      `json:"token_max_ttl"`
	Period   int      `json:"token_period"`
}

func readAWSRole(t *testing.T, c *Client, name string) awsRole {
	t.Helper()
	r, err := c.Must("GET", "auth/aws/role/"+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wrap struct {
		Data awsRole `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &wrap); err != nil {
		t.Fatal(err)
	}
	return wrap.Data
}

// AppRole stands in for AWS auth: the token carries the same policies an admin-* role grants.
func TestAdminAccessLevels(t *testing.T) {
	c := configuredVault(t)
	if err := c.enableAWSAuth(); err != nil {
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
