package vaultcluster

import (
	"fmt"
	"testing"

	"github.com/hashicorp/vault/api"
)

func TestIsolationMatrix(t *testing.T) {
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-a", false); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-b", false); err != nil {
		t.Fatal(err)
	}

	tokA, err := c.LoginAppRole(c.Cfg.TenantRole("reader", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	tokB, err := c.LoginAppRole(c.Cfg.TenantRole("reader", "tenant-b"))
	if err != nil {
		t.Fatal(err)
	}
	tokAW, err := c.LoginAppRole(c.Cfg.TenantRole("writer", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	tokBW, err := c.LoginAppRole(c.Cfg.TenantRole("writer", "tenant-b"))
	if err != nil {
		t.Fatal(err)
	}

	provSec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{
		Policies:    []string{"provisioning"},
		Period:      "1h",
		NoParent:    true,
		DisplayName: "provisioning",
	})
	if err != nil || provSec == nil || provSec.Auth == nil {
		t.Fatalf("provisioning token: %v", err)
	}
	tokProv := provSec.Auth.ClientToken

	mustWrite := func(token, path, val string) {
		t.Helper()
		cl := c.WithToken(token)
		if _, err := cl.API.Logical().Write(path, map[string]any{"data": map[string]any{"value": val}}); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite(tokAW, c.Cfg.KVDataPath("tenant-a", "fixture"), "secret-a")
	mustWrite(tokBW, c.Cfg.KVDataPath("tenant-b", "fixture"), "secret-b")

	assertStatus := func(token, method, path string, body any, want int) {
		t.Helper()
		cl := c.WithToken(token)
		r, err := cl.Do(method, path, body)
		if err != nil && r.Status == 0 {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		if r.Status != want {
			t.Fatalf("%s %s: got HTTP %d want %d body %s", method, path, r.Status, want, r.Body)
		}
	}
	deny := func(token, method, path string, body any) {
		t.Helper()
		assertStatus(token, method, path, body, 403)
	}

	aData := c.Cfg.KVDataPath("tenant-a", "fixture")
	bData := c.Cfg.KVDataPath("tenant-b", "fixture")
	aMeta := c.Cfg.KVMetaPath("tenant-a", "fixture")
	bMeta := c.Cfg.KVMetaPath("tenant-b", "fixture")
	crossWrite := map[string]any{"data": map[string]any{"value": "FAKE-cross-tenant-write"}}

	assertStatus(tokA, "GET", aData, nil, 200)
	assertStatus(tokB, "GET", bData, nil, 200)
	assertStatus(tokA, "GET", "auth/token/lookup-self", nil, 200)

	deny(tokA, "GET", bData, nil)
	deny(tokB, "GET", aData, nil)
	deny(tokAW, "GET", bData, nil)
	deny(tokBW, "GET", aData, nil)
	deny(tokAW, "POST", bData, crossWrite)
	deny(tokBW, "POST", aData, crossWrite)
	deny(tokAW, "POST", c.Cfg.KVDataPath("tenant-b", "newly-planted-secret"), map[string]any{
		"data": map[string]any{"value": "FAKE-planted"},
	})
	deny(tokAW, "DELETE", bData, nil)
	deny(tokAW, "POST", c.Cfg.KVMount+"/destroy/"+c.Cfg.TenantPrefix+"/tenant-b/fixture", map[string]any{"versions": []int{1}})
	deny(tokAW, "DELETE", bMeta, nil)
	deny(tokA, "GET", bMeta, nil)
	deny(tokB, "GET", aMeta, nil)
	deny(tokA, "GET", c.Cfg.KVMount+"/metadata/"+c.Cfg.TenantPrefix+"?list=true", nil)
	deny(tokB, "GET", c.Cfg.KVMount+"/metadata/"+c.Cfg.TenantPrefix+"?list=true", nil)
	deny(tokA, "GET", c.Cfg.KVMetaPath("tenant-b", "")+"?list=true", nil)

	deny(tokA, "GET", c.Cfg.KVMount+"/data/"+c.Cfg.TenantPrefix, nil)
	deny(tokA, "GET", c.Cfg.KVMount+"/data", nil)
	deny(tokA, "GET", c.Cfg.KVMount+"/metadata/"+c.Cfg.TenantPrefix, nil)
	deny(tokAW, "POST", c.Cfg.KVMount+"/data/"+c.Cfg.TenantPrefix+"/shared-secret", map[string]any{
		"data": map[string]any{"value": "FAKE-planted-at-parent"},
	})
	deny(tokA, "GET", c.Cfg.KVMount+"/data/platform/root-credentials", nil)
	deny(tokAW, "POST", c.Cfg.KVMount+"/data/platform/root-credentials", map[string]any{
		"data": map[string]any{"value": "FAKE-planted-sibling"},
	})
	deny(tokA, "GET", c.Cfg.KVDataPath("tenant-a-extended", "secret"), nil)
	deny(tokA, "GET", c.Cfg.KVDataPath("tenant-ax", "secret"), nil)
	deny(tokA, "GET", c.Cfg.KVMount+"/data/"+c.Cfg.TenantPrefix+"/*", nil)

	deny(tokA, "GET", "sys/mounts", nil)
	deny(tokA, "GET", "sys/auth", nil)
	deny(tokAW, "POST", "sys/mounts/rogue", map[string]any{"type": "kv", "options": map[string]any{"version": "2"}})
	deny(tokAW, "DELETE", "sys/mounts/"+c.Cfg.KVMount, nil)
	deny(tokA, "GET", "sys/audit", nil)
	deny(tokA, "GET", "sys/policies/acl?list=true", nil)
	deny(tokA, "GET", "sys/policies/acl/"+c.Cfg.TenantPolicy("reader", "tenant-b"), nil)
	deny(tokAW, "PUT", "sys/policies/acl/"+c.Cfg.TenantPolicy("reader", "tenant-a"), map[string]any{
		"policy": `path "kv/data/*" { capabilities = ["read", "list"] }`,
	})
	deny(tokA, "GET", "auth/"+c.Cfg.AuthMount+"/role?list=true", nil)
	deny(tokA, "GET", "auth/"+c.Cfg.AuthMount+"/role/"+c.Cfg.TenantRole("reader", "tenant-b")+"/role-id", nil)
	deny(tokAW, "POST", "auth/"+c.Cfg.AuthMount+"/role/"+c.Cfg.TenantRole("reader", "tenant-b")+"/secret-id", map[string]any{})
	deny(tokAW, "POST", "auth/token/create", map[string]any{"policies": []string{"operator"}})
	deny(tokA, "GET", "auth/token/accessors?list=true", nil)

	deny(tokProv, "GET", aData, nil)
	deny(tokProv, "GET", aMeta, nil)
	deny(tokProv, "POST", aData, map[string]any{"data": map[string]any{"value": "FAKE-written-by-provisioning"}})
	deny(tokProv, "PUT", "sys/policies/acl/provisioning", map[string]any{
		"policy": `path "kv/data/*" { capabilities = ["read"] }`,
	})
	assertStatus(tokProv, "GET", "auth/"+c.Cfg.AuthMount+"/role/"+c.Cfg.TenantRole("reader", "tenant-a")+"/role-id", nil, 200)

	for _, traversal := range []string{
		fmt.Sprintf("%s/data/%s/tenant-a/../tenant-b/fixture", c.Cfg.KVMount, c.Cfg.TenantPrefix),
		fmt.Sprintf("%s/data/%s/tenant-a/..%%2ftenant-b/fixture", c.Cfg.KVMount, c.Cfg.TenantPrefix),
	} {
		cl := c.WithToken(tokA)
		r, err := cl.Do("GET", traversal, nil)
		if err != nil && r.Status == 0 {
			t.Fatalf("traversal %s: %v", traversal, err)
		}
		if r.Status >= 200 && r.Status < 300 {
			t.Fatalf("traversal granted HTTP %d: %s", r.Status, traversal)
		}
	}

	if err := c.OffboardTenant("tenant-a", false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LoginAppRole(c.Cfg.TenantRole("reader", "tenant-a")); err == nil {
		t.Fatal("expected AppRole login to fail after offboard")
	}
}
