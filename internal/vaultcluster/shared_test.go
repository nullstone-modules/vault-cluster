package vaultcluster

import (
	"strings"
	"testing"

	"github.com/hashicorp/vault/api"
)

// A shared cluster. Everything an unshared cluster promises still holds; on top, two envs never see each
// other's secrets even for the same tenant ID, and an app's broker mints only its own env's logins.

const appsTestMount = "apps-test"

func startSharedVault(t *testing.T) *Client {
	t.Helper()
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.EnvPrefix = "envs"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	c.Cfg.SharedEnvs = true
	c.Cfg.TokenTTL = "15m"
	c.Cfg.TokenMaxTTL = "15m"
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	return c
}

// appBroker logs in as an app would through the cluster function: one auth role, one entity with env, one
// broker policy. Stand-in mount is AppRole; the aws and gcp mounts alias by role_id the same way.
func appBroker(t *testing.T, c *Client, env, kind string) *Client {
	t.Helper()
	auths, err := c.API.Sys().ListAuth()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := auths[appsTestMount+"/"]; !ok {
		if err := c.API.Sys().EnableAuthWithOptions(appsTestMount, &api.EnableAuthOptions{Type: "approle"}); err != nil {
			t.Fatal(err)
		}
	}
	role := "api-" + kind + "-" + env
	if env == "" {
		role = "api-" + kind + "-noenv"
	}
	if _, err := c.API.Logical().Write("auth/"+appsTestMount+"/role/"+role, map[string]any{
		"token_policies": []string{c.Cfg.AppsPolicy(kind)},
	}); err != nil {
		t.Fatal(err)
	}
	if env != "" {
		rid, err := c.API.Logical().Read("auth/" + appsTestMount + "/role/" + role + "/role-id")
		if err != nil {
			t.Fatal(err)
		}
		acc, err := c.authAccessor(appsTestMount)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.ensureEntityAlias(appsTestMount+"/"+role, map[string]any{entityMetaEnv: env}, rid.Data["role_id"].(string), acc); err != nil {
			t.Fatal(err)
		}
	}
	tok, err := c.LoginAppRole(appsTestMount, role)
	if err != nil {
		t.Fatal(err)
	}
	return c.WithToken(tok)
}

func httpStatus(t *testing.T, cl *Client, method, path string, body any) int {
	t.Helper()
	r, err := cl.Do(method, path, body)
	if err != nil && r.Status == 0 {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return r.Status
}

func mustDeny(t *testing.T, cl *Client, method, path string, body any) {
	t.Helper()
	if got := httpStatus(t, cl, method, path, body); got != 403 {
		t.Fatalf("%s %s: got HTTP %d want 403", method, path, got)
	}
}

func TestIsolationMatrixShared(t *testing.T) {
	c := startSharedVault(t)
	if err := c.CreateTenant("", "acme"); err == nil {
		t.Fatal("shared cluster created a tenant without an env")
	}
	for _, env := range []string{"env-a", "env-b"} {
		for _, id := range []string{"acme", "beta"} {
			if err := c.CreateTenant(env, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Re-running create is a no-op, not a failure.
	if err := c.CreateTenant("env-a", "acme"); err != nil {
		t.Fatalf("second create: %v", err)
	}

	login := func(kind, env, id string) *Client {
		t.Helper()
		tok, err := c.LoginAppRole(c.Cfg.TenantMount(kind), c.Cfg.TenantRole(env, id))
		if err != nil {
			t.Fatal(err)
		}
		return c.WithToken(tok)
	}
	aW, bW := login("writer", "env-a", "acme"), login("writer", "env-b", "acme")
	aR, bR := login("reader", "env-a", "acme"), login("reader", "env-b", "acme")
	betaR := login("reader", "env-a", "beta")

	write := map[string]any{"data": map[string]any{"value": "FAKE"}}
	aData, bData := c.Cfg.KVDataPath("env-a", "acme", "fixture"), c.Cfg.KVDataPath("env-b", "acme", "fixture")
	if got := httpStatus(t, aW, "POST", aData, write); got != 200 {
		t.Fatalf("env-a writer own secret: HTTP %d", got)
	}
	if got := httpStatus(t, bW, "POST", bData, write); got != 200 {
		t.Fatalf("env-b writer own secret: HTTP %d", got)
	}
	if got := httpStatus(t, aR, "GET", aData, nil); got != 200 {
		t.Fatalf("env-a reader own secret: HTTP %d", got)
	}
	if got := httpStatus(t, bR, "GET", bData, nil); got != 200 {
		t.Fatalf("env-b reader own secret: HTTP %d", got)
	}

	// Same tenant ID, other env.
	mustDeny(t, aR, "GET", bData, nil)
	mustDeny(t, bR, "GET", aData, nil)
	mustDeny(t, aW, "POST", bData, write)
	mustDeny(t, aW, "DELETE", bData, nil)
	mustDeny(t, aW, "DELETE", c.Cfg.KVMetaPath("env-b", "acme", "fixture"), nil)
	// Other tenant, same env.
	mustDeny(t, betaR, "GET", aData, nil)
	mustDeny(t, aR, "GET", c.Cfg.KVDataPath("env-a", "beta", "fixture"), nil)
	// Parents, wildcards, the env prefix, and the unscoped layout.
	for _, p := range []string{
		"kv/data/envs/env-a/customers/acme-extended/secret",
		"kv/data/envs/env-a/customers/shared-secret",
		"kv/data/envs/env-a/customers/*",
		"kv/data/envs/env-a/platform/x",
		"kv/data/envs/*",
		"kv/data/envs/env-a",
		"kv/data/customers/acme/fixture",
		"kv/data/customers/env-a/acme/fixture",
		"kv/data/platform/root-credentials",
	} {
		mustDeny(t, aR, "GET", p, nil)
		mustDeny(t, aW, "POST", p, write)
	}
	mustDeny(t, aR, "GET", "kv/metadata/envs?list=true", nil)
	mustDeny(t, aR, "GET", "kv/metadata/envs/env-a?list=true", nil)
	mustDeny(t, aR, "GET", "kv/metadata/envs/env-a/customers?list=true", nil)
	mustDeny(t, aR, "GET", c.Cfg.KVMetaPath("env-b", "acme", "")+"?list=true", nil)
	for _, traversal := range []string{
		"kv/data/envs/env-a/customers/acme/../../../env-b/customers/acme/fixture",
		"kv/data/envs/env-a/..%2fenv-b/customers/acme/fixture",
	} {
		if got := httpStatus(t, aR, "GET", traversal, nil); got >= 200 && got < 300 {
			t.Fatalf("traversal granted HTTP %d: %s", got, traversal)
		}
	}
	// Vault lets a token read its own entity. It must not read another's or change its own metadata.
	mustDeny(t, aR, "GET", "identity/entity/name/approle-reader/env-b.acme", nil)
	mustDeny(t, aR, "GET", "identity/entity/name/approle-writer/env-a.acme", nil)
	mustDeny(t, aW, "POST", "identity/entity/name/approle-writer/env-a.acme", map[string]any{"metadata": map[string]any{"env": "env-b"}})
	mustDeny(t, aW, "POST", "identity/entity-alias", map[string]any{"name": "x", "canonical_id": "y", "mount_accessor": "z"})

	// Provisioning: creates and offboards, but cannot read tenant data, write policies, grant an entity
	// policies, or touch groups.
	provSec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{Policies: []string{"provisioning"}, NoParent: true})
	if err != nil {
		t.Fatal(err)
	}
	prov := c.WithToken(provSec.Auth.ClientToken)
	if err := prov.CreateTenant("env-c", "gamma"); err != nil {
		t.Fatalf("provisioning create: %v", err)
	}
	if _, err := c.LoginAppRole(c.Cfg.TenantMount("reader"), "env-c.gamma"); err != nil {
		t.Fatal(err)
	}
	mustDeny(t, prov, "GET", aData, nil)
	mustDeny(t, prov, "POST", "identity/entity", map[string]any{"name": "evil", "policies": []string{"operator"}})
	mustDeny(t, prov, "POST", "identity/entity/name/approle-reader/env-a.acme", map[string]any{"policies": []string{"operator"}})
	mustDeny(t, prov, "POST", "identity/group", map[string]any{"name": "g"})
	mustDeny(t, prov, "PUT", "sys/policies/acl/tenant-reader", map[string]any{"policy": `path "kv/*" { capabilities = ["read"] }`})
	if err := prov.OffboardTenant("env-c", "gamma", false); err != nil {
		t.Fatalf("provisioning offboard: %v", err)
	}
	if _, err := c.LoginAppRole(c.Cfg.TenantMount("reader"), "env-c.gamma"); err == nil {
		t.Fatal("login after offboard succeeded")
	}
	if r, _ := c.Do("GET", "identity/entity/name/approle-reader/env-c.gamma", nil); r.Status != 404 {
		t.Fatalf("entity survived offboard: HTTP %d", r.Status)
	}

	// A login that happened before the entity existed is re-pointed at the metadata-bearing entity.
	if err := c.writeAppRole("reader", "env-d.delta", []string{"tenant-reader"}); err != nil {
		t.Fatal(err)
	}
	early, err := c.LoginAppRole(c.Cfg.TenantMount("reader"), "env-d.delta")
	if err != nil {
		t.Fatal(err)
	}
	mustDeny(t, c.WithToken(early), "GET", c.Cfg.KVDataPath("env-d", "delta", "x"), nil)
	if err := c.CreateTenant("env-d", "delta"); err != nil {
		t.Fatal(err)
	}
	late := login("reader", "env-d", "delta")
	if got := httpStatus(t, late, "GET", c.Cfg.KVDataPath("env-d", "delta", "x"), nil); got != 404 {
		t.Fatalf("after bind, own path should be reachable (404 = allowed, missing): HTTP %d", got)
	}
}

func TestAppsBrokerTokensShared(t *testing.T) {
	c := startSharedVault(t)
	for _, env := range []string{"env-a", "env-b"} {
		if err := c.CreateTenant(env, "acme"); err != nil {
			t.Fatal(err)
		}
	}
	readerMount, writerMount := c.Cfg.TenantMount("reader"), c.Cfg.TenantMount("writer")
	reader, writer := appBroker(t, c, "env-a", "reader"), appBroker(t, c, "env-a", "writer")
	noEnv := appBroker(t, c, "", "reader")

	tok, err := reader.LoginAppRole(readerMount, "env-a.acme")
	if err != nil {
		t.Fatalf("broker login as its env's tenant: %v", err)
	}
	if got := httpStatus(t, c.WithToken(tok), "GET", c.Cfg.KVDataPath("env-a", "acme", "fixture"), nil); got != 404 {
		t.Fatalf("tenant token reading own (missing) secret: HTTP %d want 404", got)
	}
	mustDeny(t, c.WithToken(tok), "GET", c.Cfg.KVDataPath("env-b", "acme", "fixture"), nil)
	if _, err := writer.LoginAppRole(writerMount, "env-a.acme"); err != nil {
		t.Fatalf("writer broker login: %v", err)
	}

	// Other env: nothing.
	mustDeny(t, reader, "GET", "auth/"+readerMount+"/role/env-b.acme/role-id", nil)
	mustDeny(t, reader, "POST", "auth/"+readerMount+"/role/env-b.acme/secret-id", map[string]any{})
	if _, err := reader.LoginAppRole(readerMount, "env-b.acme"); err == nil {
		t.Fatal("broker logged in as another env's tenant")
	}
	// Own env, but the role itself stays read-only: no field can be sent.
	mustDeny(t, reader, "POST", "auth/"+readerMount+"/role/env-a.acme", map[string]any{"token_policies": []string{"operator"}})
	mustDeny(t, reader, "POST", "auth/"+readerMount+"/role/env-a.acme/role-id", map[string]any{"role_id": "chosen"})
	mustDeny(t, reader, "POST", "auth/"+readerMount+"/role/env-a.acme/policies", map[string]any{"token_policies": []string{"operator"}})
	mustDeny(t, reader, "POST", "auth/"+readerMount+"/role/env-a.acme/secret-id", map[string]any{"metadata": `{"env":"env-b"}`})
	mustDeny(t, reader, "DELETE", "auth/"+readerMount+"/role/env-a.acme", nil)
	mustDeny(t, reader, "GET", "auth/"+readerMount+"/role?list=true", nil)
	mustDeny(t, reader, "GET", "auth/"+readerMount+"/role/env-a.acme/secret-id?list=true", nil)
	role, err := c.API.Logical().Read("auth/" + readerMount + "/role/env-a.acme")
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := role.Data["token_policies"].([]any); len(p) != 1 || p[0] != "tenant-reader" {
		t.Fatalf("role was changed by the broker: %v", role.Data)
	}
	// Wrong level, KV, identity, and policies stay closed.
	mustDeny(t, reader, "GET", "auth/"+writerMount+"/role/env-a.acme/role-id", nil)
	mustDeny(t, writer, "GET", "auth/"+readerMount+"/role/env-a.acme/role-id", nil)
	mustDeny(t, reader, "GET", c.Cfg.KVDataPath("env-a", "acme", "fixture"), nil)
	mustDeny(t, reader, "GET", "identity/entity/id?list=true", nil)
	mustDeny(t, reader, "POST", "identity/entity", map[string]any{"name": "x", "metadata": map[string]any{"env": "env-b"}})
	mustDeny(t, reader, "GET", "sys/policies/acl/apps-reader", nil)
	if got := httpStatus(t, reader, "GET", "auth/"+readerMount+"/role/env-a.nobody/role-id", nil); got != 404 {
		t.Fatalf("unknown tenant in own env: HTTP %d want 404", got)
	}
	// No env on the entity: no role matches at all.
	mustDeny(t, noEnv, "GET", "auth/"+readerMount+"/role/env-a.acme/role-id", nil)
	mustDeny(t, noEnv, "GET", "auth/"+readerMount+"/role/env-b.acme/role-id", nil)
}

func TestEnvsAndTenantListShared(t *testing.T) {
	c := startSharedVault(t)
	if envs, err := c.ListEnvs(); err != nil || len(envs) != 0 {
		t.Fatalf("empty cluster: %v %v", envs, err)
	}
	for _, tc := range [][2]string{{"env-b", "acme"}, {"env-a", "beta"}, {"env-a", "acme"}} {
		if err := c.CreateTenant(tc[0], tc[1]); err != nil {
			t.Fatal(err)
		}
	}
	// An unscoped role left over from before the cluster was shared shows up flagged.
	if err := c.writeAppRole("reader", "legacy", []string{"tenant-reader"}); err != nil {
		t.Fatal(err)
	}
	envs, err := c.ListEnvs()
	if err != nil || strings.Join(envs, ",") != "env-a,env-b" {
		t.Fatalf("envs = %v, %v", envs, err)
	}
	tenants, err := c.ListTenants()
	if err != nil {
		t.Fatal(err)
	}
	want := []Tenant{{ID: "legacy", Reader: true}, {Env: "env-a", ID: "acme", Reader: true, Writer: true}, {Env: "env-a", ID: "beta", Reader: true, Writer: true}, {Env: "env-b", ID: "acme", Reader: true, Writer: true}}
	if len(tenants) != len(want) {
		t.Fatalf("got %+v want %+v", tenants, want)
	}
	for i := range want {
		if tenants[i] != want[i] {
			t.Fatalf("got %+v want %+v", tenants, want)
		}
	}
	var out strings.Builder
	PrintTenants(&out, tenants, true)
	if !strings.Contains(out.String(), "legacy\t(no writer role; re-run tenants create; not env-scoped; this cluster is shared)") || !strings.Contains(out.String(), "env-a\tacme\n") {
		t.Fatalf("output:\n%s", out.String())
	}

	// Destroy one env with purge: its roles, entities, and secrets go; the other env is untouched.
	tokA, err := c.LoginAppRole(c.Cfg.TenantMount("writer"), "env-a.acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{c.Cfg.KVDataPath("env-a", "acme", "cfg"), c.Cfg.KVDataPath("env-a", "acme", "nested/deep")} {
		if got := httpStatus(t, c.WithToken(tokA), "POST", p, map[string]any{"data": map[string]any{"v": "FAKE"}}); got != 200 {
			t.Fatalf("seed %s: HTTP %d", p, got)
		}
	}
	tokB, err := c.LoginAppRole(c.Cfg.TenantMount("writer"), "env-b.acme")
	if err != nil {
		t.Fatal(err)
	}
	if got := httpStatus(t, c.WithToken(tokB), "POST", c.Cfg.KVDataPath("env-b", "acme", "cfg"), map[string]any{"data": map[string]any{"v": "FAKE"}}); got != 200 {
		t.Fatalf("seed env-b: HTTP %d", got)
	}
	if err := c.DestroyEnv("env-a", true); err != nil {
		t.Fatal(err)
	}
	envs, _ = c.ListEnvs()
	if strings.Join(envs, ",") != "env-b" {
		t.Fatalf("envs after destroy = %v", envs)
	}
	if r, _ := c.Do("GET", "kv/metadata/envs/env-a?list=true", nil); r.Status != 404 {
		t.Fatalf("env-a secrets survived purge: HTTP %d %s", r.Status, r.Body)
	}
	if r, _ := c.Do("GET", c.Cfg.KVDataPath("env-b", "acme", "cfg"), nil); r.Status != 200 {
		t.Fatalf("env-b secret lost: HTTP %d", r.Status)
	}
	if r, _ := c.Do("GET", "identity/entity/name/approle-reader/env-a.acme", nil); r.Status != 404 {
		t.Fatalf("env-a entity survived: HTTP %d", r.Status)
	}
	if _, err := c.LoginAppRole(c.Cfg.TenantMount("reader"), "env-a.acme"); err == nil {
		t.Fatal("env-a login survived destroy")
	}

	unshared := c.WithToken(c.Cfg.Token)
	unshared.Cfg.SharedEnvs = false
	if _, err := unshared.ListEnvs(); err == nil {
		t.Fatal("envs list on an unshared cluster succeeded")
	}
	if err := unshared.DestroyEnv("env-b", false); err == nil {
		t.Fatal("envs destroy on an unshared cluster succeeded")
	}
}
