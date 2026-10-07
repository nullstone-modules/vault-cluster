package vaultcluster

import (
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
)

// An app holds a broker token (apps-reader or apps-writer). It can turn that into a token for any
// onboarded tenant at its level, one tenant per login, and nothing else.
func TestAppsBrokerTokens(t *testing.T) {
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	c.Cfg.TokenTTL = "15m"
	c.Cfg.TokenMaxTTL = "15m"
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tenant-a", "tenant-b"} {
		if err := c.CreateTenant("", id); err != nil {
			t.Fatal(err)
		}
	}

	broker := func(kind string) *Client {
		t.Helper()
		sec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{
			Policies: []string{c.Cfg.AppsPolicy(kind)}, NoParent: true, DisplayName: "app-" + kind,
		})
		if err != nil || sec == nil || sec.Auth == nil {
			t.Fatalf("%s broker token: %v", kind, err)
		}
		return c.WithToken(sec.Auth.ClientToken)
	}
	reader, writer := broker("reader"), broker("writer")

	status := func(cl *Client, method, path string, body any) int {
		t.Helper()
		r, err := cl.Do(method, path, body)
		if err != nil && r.Status == 0 {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return r.Status
	}
	deny := func(cl *Client, method, path string, body any) {
		t.Helper()
		if got := status(cl, method, path, body); got != 403 {
			t.Fatalf("%s %s: got HTTP %d want 403", method, path, got)
		}
	}
	readerMount, writerMount := c.Cfg.TenantMount("reader"), c.Cfg.TenantMount("writer")

	// Writer fixture so reads have something to find.
	tokAW, err := writer.LoginAppRole(writerMount, "tenant-a")
	if err != nil {
		t.Fatalf("writer broker login as tenant-a: %v", err)
	}
	if _, err := c.WithToken(tokAW).API.Logical().Write(c.Cfg.KVDataPath("", "tenant-a", "fixture"), map[string]any{"data": map[string]any{"v": "FAKE-a"}}); err != nil {
		t.Fatal(err)
	}

	// Each login is one tenant.
	for _, id := range []string{"tenant-a", "tenant-b"} {
		tok, err := reader.LoginAppRole(readerMount, id)
		if err != nil {
			t.Fatalf("reader broker login as %s: %v", id, err)
		}
		cl := c.WithToken(tok)
		other := "tenant-b"
		if id == "tenant-b" {
			other = "tenant-a"
		}
		if got := status(cl, "GET", c.Cfg.KVDataPath("", id, "fixture"), nil); got != 200 && got != 404 {
			t.Fatalf("%s reading its own secret: HTTP %d", id, got)
		}
		deny(cl, "GET", c.Cfg.KVDataPath("", other, "fixture"), nil)
		deny(cl, "POST", c.Cfg.KVDataPath("", id, "fixture"), map[string]any{"data": map[string]any{"v": "FAKE-write"}})
		deny(cl, "GET", "auth/"+readerMount+"/role/"+other+"/role-id", nil)

		self, err := cl.API.Auth().Token().LookupSelf()
		if err != nil {
			t.Fatal(err)
		}
		ttl, _ := self.TokenTTL()
		if ttl > 15*time.Minute || ttl <= 0 {
			t.Fatalf("%s tenant token ttl = %s, want at most 15m", id, ttl)
		}
		if self.Data["renewable"] == true {
			if _, err := cl.API.Auth().Token().RenewSelf(3600); err == nil {
				ttl2, _ := cl.API.Auth().Token().LookupSelf()
				if d, _ := ttl2.TokenTTL(); d > 15*time.Minute {
					t.Fatalf("%s tenant token renewed past 15m: %s", id, d)
				}
			}
		}
	}

	// The broker token itself holds no tenant access and cannot pick a level it was not given.
	deny(reader, "GET", c.Cfg.KVDataPath("", "tenant-a", "fixture"), nil)
	deny(reader, "GET", "auth/"+readerMount+"/role?list=true", nil)
	deny(reader, "GET", "auth/"+readerMount+"/role/tenant-a", nil)
	deny(reader, "POST", "auth/"+readerMount+"/role/tenant-c", map[string]any{"token_policies": []string{"tenant-reader"}})
	deny(reader, "GET", "auth/"+writerMount+"/role/tenant-a/role-id", nil)
	deny(reader, "POST", "auth/"+writerMount+"/role/tenant-a/secret-id", map[string]any{})
	deny(reader, "POST", "auth/token/create", map[string]any{"policies": []string{"tenant-writer"}})
	deny(reader, "GET", "sys/policies/acl/tenant-reader", nil)
	deny(writer, "GET", "auth/"+readerMount+"/role/tenant-a/role-id", nil)
	deny(writer, "GET", c.Cfg.KVDataPath("", "tenant-a", "fixture"), nil)
	deny(writer, "GET", c.Cfg.DatabaseMount+"/creds/tenant-tenant-a-readonly", nil)

	// Unknown tenants have no role, so there is nothing to log in as.
	if got := status(reader, "GET", "auth/"+readerMount+"/role/tenant-c/role-id", nil); got != 404 {
		t.Fatalf("role-id of an unknown tenant: HTTP %d want 404", got)
	}

	// A one-time secret ID cannot be replayed.
	roleID, err := reader.API.Logical().Read("auth/" + readerMount + "/role/tenant-a/role-id")
	if err != nil {
		t.Fatal(err)
	}
	secretID, err := reader.API.Logical().Write("auth/"+readerMount+"/role/tenant-a/secret-id", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	login := map[string]any{"role_id": roleID.Data["role_id"], "secret_id": secretID.Data["secret_id"]}
	if got := status(c, "POST", "auth/"+readerMount+"/login", login); got != 200 {
		t.Fatalf("first login: HTTP %d", got)
	}
	if got := status(c, "POST", "auth/"+readerMount+"/login", login); got < 400 {
		t.Fatalf("replayed secret ID logged in: HTTP %d", got)
	}
}

// The role lists are the tenant registry: list shows onboarded tenants, flags a half-created one, and
// is readable by provisioning but not by a broker.
func TestListTenants(t *testing.T) {
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ListTenants(); err != nil || len(got) != 0 {
		t.Fatalf("empty cluster: %v %v", got, err)
	}
	for _, id := range []string{"tenant-b", "tenant-a"} {
		if err := c.CreateTenant("", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.deleteMissingOK("auth/" + c.Cfg.TenantMount("writer") + "/role/tenant-b"); err != nil {
		t.Fatal(err)
	}

	prov, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{Policies: []string{"provisioning"}, NoParent: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.WithToken(prov.Auth.ClientToken).ListTenants()
	if err != nil {
		t.Fatal(err)
	}
	want := []Tenant{{ID: "tenant-a", Reader: true, Writer: true}, {ID: "tenant-b", Reader: true}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v want %+v", got, want)
	}
	var out strings.Builder
	PrintTenants(&out, got, false)
	if !strings.Contains(out.String(), "tenant-b\t(no writer role") || !strings.HasPrefix(out.String(), "tenant-a\n") {
		t.Fatalf("output:\n%s", out.String())
	}

	app, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{Policies: []string{c.Cfg.AppsPolicy("reader")}, NoParent: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.WithToken(app.Auth.ClientToken).ListTenants(); err == nil {
		t.Fatal("a broker token listed tenants")
	}
}
