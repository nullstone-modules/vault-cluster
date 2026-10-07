package vaultcluster

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/policies goldens")

var allPolicies = []string{"provisioning", "operator", "apps-auth", "apps-reader", "apps-writer", "tenant-reader", "tenant-writer", "tenant-database"}

var goldenAccessors = TenantAccessors{Reader: "auth_approle_1a2b3c4d", Writer: "auth_approle_5e6f7a8b"}

func goldenConfig(shared bool) Config {
	return Config{KVMount: "kv", TenantPrefix: "customers", EnvPrefix: "envs", DatabaseMount: "database", AuthMount: "approle", SharedEnvs: shared}
}

// The unshared goldens must not move for shared-env support: an unshared cluster is unaffected by it.
// Any other change to a policy is deliberate and updates both sets. The shared goldens document what a shared cluster applies.
func TestRenderedPoliciesMatchGoldens(t *testing.T) {
	for _, mode := range []struct {
		dir    string
		shared bool
	}{{"unshared", false}, {"shared", true}} {
		cfg := goldenConfig(mode.shared)
		for _, name := range allPolicies {
			hcl, err := RenderPolicy(name, cfg, goldenAccessors)
			if err != nil {
				t.Fatal(err)
			}
			if err := LintOrError(name, hcl, cfg); err != nil {
				t.Fatalf("%s/%s: %v", mode.dir, name, err)
			}
			path := filepath.Join("testdata", "policies", mode.dir, name+".hcl")
			if *updateGolden {
				if err := os.WriteFile(path, []byte(hcl), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != hcl {
				t.Errorf("%s/%s differs from golden (run with -update to accept):\n%s", mode.dir, name, diffLines(string(want), hcl))
			}
		}
	}
}

func TestSharedPoliciesScopeByEntityMetadata(t *testing.T) {
	cfg := goldenConfig(true)
	env, tenant := entityMetaTemplate("env"), entityMetaTemplate("tenant")
	for _, name := range []string{"tenant-reader", "tenant-writer"} {
		hcl, _ := RenderPolicy(name, cfg, goldenAccessors)
		if !strings.Contains(hcl, `"kv/data/envs/`+env+`/customers/`+tenant+`/*"`) {
			t.Fatalf("%s must scope data to envs/<env>/customers/<tenant>:\n%s", name, hcl)
		}
		if strings.Contains(hcl, "role_name") || !strings.Contains(hcl, `"kv/data/envs/*"`) {
			t.Fatalf("%s must not use the role name and must deny the env prefix:\n%s", name, hcl)
		}
	}
	db, _ := RenderPolicy("tenant-database", cfg, goldenAccessors)
	if !strings.Contains(db, `"database/creds/tenant-`+env+`.`+tenant+`-*"`) {
		t.Fatalf("tenant-database must scope creds by env and tenant:\n%s", db)
	}
	for _, name := range []string{"apps-reader", "apps-writer"} {
		hcl, _ := RenderPolicy(name, cfg, goldenAccessors)
		mount := "approle-reader"
		if name == "apps-writer" {
			mount = "approle-writer"
		}
		if !strings.Contains(hcl, `"auth/`+mount+`/role/`+env+`.*"`) || !strings.Contains(hcl, `denied_parameters = { "*" = [] }`) {
			t.Fatalf("%s must mint only its env's roles with no parameters:\n%s", name, hcl)
		}
		if strings.Contains(hcl, "role/+/") {
			t.Fatalf("%s must not keep the any-tenant rules:\n%s", name, hcl)
		}
	}
	for _, name := range []string{"provisioning", "apps-auth"} {
		hcl, _ := RenderPolicy(name, cfg, goldenAccessors)
		if !strings.Contains(hcl, `path "identity/entity"`) || !strings.Contains(hcl, `denied_parameters = { "policies" = [], "disabled" = [] }`) {
			t.Fatalf("%s must write entities without policies:\n%s", name, hcl)
		}
		if strings.Contains(hcl, `path "identity/*"`) {
			t.Fatalf("%s must narrow the identity deny:\n%s", name, hcl)
		}
	}
	unshared, _ := RenderPolicy("provisioning", goldenConfig(false), goldenAccessors)
	if !strings.Contains(unshared, `path "identity/*"`) || strings.Contains(unshared, `path "identity/entity"`) {
		t.Fatalf("unshared provisioning must keep the identity deny:\n%s", unshared)
	}
}

func diffLines(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			b.WriteString("- " + wl + "\n+ " + gl + "\n")
		}
	}
	return b.String()
}
