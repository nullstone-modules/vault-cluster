package vaultcluster

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var testOIDC = &OIDCConfig{DiscoveryURL: "https://login.example.test/v2.0", ClientID: "vault"}

func bind(name, method, principal string, access ...string) AdminBinding {
	return AdminBinding{Name: name, Method: method, Principal: principal, Access: access}
}

func TestValidateAdminConfig(t *testing.T) {
	user := "arn:aws:iam::123456789012:user/brad"
	sso := "arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/AWSReservedSSO_VaultAdmin_*"
	sa := "vault-tenants@acme-prod.iam.gserviceaccount.com"
	tests := []struct {
		name    string
		cfg     AdminConfig
		wantErr string
	}{
		{name: "empty", cfg: AdminConfig{}},
		{name: "every method", cfg: AdminConfig{OIDC: testOIDC, Bindings: []AdminBinding{
			bind("admin-brad", "aws", user, "tenants", "operator"),
			bind("admin-sso", "aws", sso, "tenants"),
			bind("admin-gcp", "gcp", sa, "tenants"),
			bind("admin-entra", "oidc", "8a3f0c1e-vault-admins", "operator"),
		}}},
		{name: "same principal on two methods", cfg: AdminConfig{OIDC: testOIDC, Bindings: []AdminBinding{
			bind("admin-a", "gcp", sa, "tenants"), bind("admin-b", "oidc", sa, "tenants"),
		}}},
		{name: "missing prefix", cfg: AdminConfig{Bindings: []AdminBinding{bind("brad", "aws", user, "tenants")}}, wantErr: "invalid admin role name"},
		{name: "duplicate name", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "aws", user, "tenants"), bind("admin-a", "aws", sso, "tenants")}}, wantErr: "duplicate"},
		{name: "duplicate principal", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "aws", user, "tenants"), bind("admin-b", "aws", user, "operator")}}, wantErr: "already bound"},
		{name: "unknown method", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "azure", "x", "tenants")}}, wantErr: "unknown method"},
		{name: "aws account wildcard", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "aws", "arn:aws:iam::123456789012:*", "tenants")}}, wantErr: "IAM user or role"},
		{name: "aws group", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "aws", "arn:aws:iam::123456789012:group/admins", "tenants")}}, wantErr: "IAM user or role"},
		{name: "gcp user email", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "gcp", "brad@acme.com", "tenants")}}, wantErr: "service account"},
		{name: "oidc wildcard", cfg: AdminConfig{OIDC: testOIDC, Bindings: []AdminBinding{bind("admin-a", "oidc", "vault-*", "tenants")}}, wantErr: "without wildcards"},
		{name: "oidc without settings", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "oidc", "admins", "tenants")}}, wantErr: "oidc settings"},
		{name: "oidc http discovery", cfg: AdminConfig{OIDC: &OIDCConfig{DiscoveryURL: "http://idp", ClientID: "v"}}, wantErr: "https"},
		{name: "unknown access", cfg: AdminConfig{Bindings: []AdminBinding{bind("admin-a", "aws", user, "root")}}, wantErr: "unknown access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAdminConfig(tt.cfg)
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

func TestLoadAdminConfig(t *testing.T) {
	write := func(body string) string {
		path := filepath.Join(t.TempDir(), "admins.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cfg, err := LoadAdminConfig(write(`{"bindings":[{"name":"admin-a","method":"aws","principal":"arn:aws:iam::123456789012:user/a","access":["tenants"]}]}`))
	if err != nil || len(cfg.Bindings) != 1 {
		t.Fatalf("cfg = %+v, %v", cfg, err)
	}
	if _, err := LoadAdminConfig(write(`{"bindings":[{"name":"admin-a","principal_arn":"x","access":["tenants"]}]}`)); err == nil {
		t.Fatal("expected unknown fields to be rejected")
	}
}

func TestOIDCRoleBody(t *testing.T) {
	cfg := AdminConfig{OIDC: &OIDCConfig{DiscoveryURL: "https://idp", ClientID: "v", GroupsClaim: "roles", RedirectURIs: []string{"https://vault.acme.example.com:8200/ui/vault/auth/oidc/oidc/callback"}}}
	body := oidcAdminAuth{}.roleBody(bind("admin-a", "oidc", "vault-admins", "tenants"), cfg)
	if body["user_claim"] != "email" || body["groups_claim"] != "roles" {
		t.Fatalf("claims = %v", body)
	}
	if !reflect.DeepEqual(body["bound_claims"], map[string]any{"roles": []string{"vault-admins"}}) {
		t.Fatalf("bound_claims = %v", body["bound_claims"])
	}
	if uris := body["allowed_redirect_uris"].([]string); len(uris) != 2 || uris[0] != oidcCLIRedirect {
		t.Fatalf("redirects = %v", uris)
	}
}

// Wildcard ARNs keep Vault from calling IAM and gcp role writes call nothing, so this runs offline.
func TestReconcileAdmins(t *testing.T) {
	c := configuredVault(t)
	tok, err := c.issueOrphanToken("admin-auth")
	if err != nil {
		t.Fatal(err)
	}
	adminAuth := c.WithToken(tok)
	appTok, err := c.issueOrphanToken("aws-auth")
	if err != nil {
		t.Fatal(err)
	}
	app := c.WithToken(appTok)

	appARN := "arn:aws:iam::123456789012:role/app-*"
	if err := app.enableAuthMount("aws"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Must("POST", "auth/aws/role/billing", map[string]any{
		"auth_type": "iam", "bound_iam_principal_arn": []string{appARN}, "token_policies": []string{"app"},
	}); err != nil {
		t.Fatal(err)
	}

	sa := "vault-tenants@acme-prod.iam.gserviceaccount.com"
	first := AdminConfig{Bindings: []AdminBinding{
		bind("admin-tenants", "aws", "arn:aws:iam::123456789012:role/vault-tenants-*", "tenants"),
		bind("admin-old", "aws", "arn:aws:iam::123456789012:role/old-*", "operator"),
		bind("admin-gcp", "gcp", sa, "tenants"),
	}}
	if err := adminAuth.ReconcileAdmins(first); err != nil {
		t.Fatal(err)
	}
	role := readAuthRole(t, c, "aws", "admin-tenants")
	if !reflect.DeepEqual(role.Policies, []string{"provisioning"}) || role.TTL != 3600 || role.MaxTTL != 28800 || role.Period != 0 {
		t.Fatalf("admin-tenants = %+v", role)
	}
	if got := readAuthRole(t, c, "gcp", "admin-gcp"); !reflect.DeepEqual(got.ServiceAccounts, []string{sa}) || !reflect.DeepEqual(got.Policies, []string{"provisioning"}) {
		t.Fatalf("admin-gcp = %+v", got)
	}

	if r, _ := app.Do("POST", "auth/aws/role/admin-tenants", map[string]any{"auth_type": "iam", "bound_iam_principal_arn": []string{appARN}}); r.Status != 403 {
		t.Fatalf("aws-auth token rewrote an admin role: HTTP %d", r.Status)
	}
	if r, _ := app.Do("GET", "auth/aws/role/admin-tenants", nil); r.Status != 200 {
		t.Fatalf("aws-auth token must read admin roles for its guard: HTTP %d", r.Status)
	}
	if r, _ := adminAuth.Do("POST", "auth/aws/role/billing", map[string]any{"auth_type": "iam", "bound_iam_principal_arn": []string{appARN}}); r.Status != 403 {
		t.Fatalf("admin-auth token rewrote an app role: HTTP %d", r.Status)
	}

	second := AdminConfig{Bindings: []AdminBinding{
		bind("admin-tenants", "aws", "arn:aws:iam::123456789012:role/vault-tenants-*", "tenants", "operator"),
		bind("admin-thief", "aws", appARN, "operator"),
	}}
	err = adminAuth.ReconcileAdmins(second)
	if err == nil || !strings.Contains(err.Error(), "already bound to aws role billing") {
		t.Fatalf("expected the app principal to be refused, got %v", err)
	}
	if got := readAuthRole(t, c, "aws", "admin-tenants").Policies; !reflect.DeepEqual(got, []string{"operator", "provisioning"}) {
		t.Fatalf("admin-tenants policies = %v", got)
	}
	for _, gone := range []string{"aws/role/admin-old", "aws/role/admin-thief", "gcp/role/admin-gcp"} {
		if r, _ := c.Do("GET", "auth/"+gone, nil); r.Status != 404 {
			t.Fatalf("%s: HTTP %d, want 404", gone, r.Status)
		}
	}
	if r, _ := c.Do("GET", "auth/aws/role/billing", nil); r.Status != 200 {
		t.Fatalf("app role must be untouched, HTTP %d", r.Status)
	}

	if err := adminAuth.ReconcileAdmins(AdminConfig{}); err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Do("GET", "auth/aws/role/admin-tenants", nil); r.Status != 404 {
		t.Fatalf("empty bindings must revoke admin-tenants, HTTP %d", r.Status)
	}
}

// oidc mount config needs a reachable IdP, so the role is written with the backend's body directly.
func TestOIDCAdminRoles(t *testing.T) {
	c := configuredVault(t)
	tok, err := c.issueOrphanToken("admin-auth")
	if err != nil {
		t.Fatal(err)
	}
	adminAuth := c.WithToken(tok)
	if err := adminAuth.enableAuthMount("oidc"); err != nil {
		t.Fatal(err)
	}
	cfg := AdminConfig{OIDC: &OIDCConfig{DiscoveryURL: "https://login.example.test/v2.0", ClientID: "vault", GroupsClaim: "roles"}}
	body := oidcAdminAuth{}.roleBody(bind("admin-entra", "oidc", "vault-admins", "tenants"), cfg)
	body["token_policies"] = []string{"provisioning"}
	if _, err := adminAuth.Must("POST", "auth/oidc/role/admin-entra", body); err != nil {
		t.Fatal(err)
	}
	if r, _ := adminAuth.Do("POST", "auth/oidc/role/app", body); r.Status != 403 {
		t.Fatalf("admin-auth must write only admin-* oidc roles: HTTP %d", r.Status)
	}
	bound, err := adminAuth.authRoleBindings("oidc", oidcAdminAuth{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bound["admin-entra"], []string{"vault-admins"}) {
		t.Fatalf("bound = %v", bound)
	}
	if role := readAuthRole(t, c, "oidc", "admin-entra"); role.UserClaim != "email" || role.GroupsClaim != "roles" {
		t.Fatalf("admin-entra = %+v", role)
	}
	if err := adminAuth.ReconcileAdmins(AdminConfig{}); err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Do("GET", "auth/oidc/role/admin-entra", nil); r.Status != 404 {
		t.Fatalf("an enabled mount with no bindings must lose its admin roles, HTTP %d", r.Status)
	}
}

type authRole struct {
	Policies        []string `json:"token_policies"`
	TTL             int      `json:"token_ttl"`
	MaxTTL          int      `json:"token_max_ttl"`
	Period          int      `json:"token_period"`
	ServiceAccounts []string `json:"bound_service_accounts"`
	UserClaim       string   `json:"user_claim"`
	GroupsClaim     string   `json:"groups_claim"`
}

func readAuthRole(t *testing.T, c *Client, method, name string) authRole {
	t.Helper()
	r, err := c.Must("GET", "auth/"+method+"/role/"+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wrap struct {
		Data authRole `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &wrap); err != nil {
		t.Fatal(err)
	}
	return wrap.Data
}
