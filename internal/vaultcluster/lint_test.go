package vaultcluster

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLintFixtures(t *testing.T) {
	cfg := Config{KVMount: "kv", TenantPrefix: "customers", DatabaseMount: "database", AuthMount: "approle"}
	root := filepath.Join("testdata", "lint")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".hcl") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(e.Name(), ".hcl")
		findings := LintPolicy(name, string(b), cfg)
		wantBad := strings.HasPrefix(e.Name(), "BAD-")
		if wantBad && len(findings) == 0 {
			t.Errorf("%s: expected findings, got none", e.Name())
		}
		if !wantBad && len(findings) > 0 {
			t.Errorf("%s: unexpected findings: %v", e.Name(), findings)
		}
	}
}

func TestRenderAndLintPlatformPolicies(t *testing.T) {
	cfg := Config{KVMount: "kv", TenantPrefix: "customers", DatabaseMount: "database", AuthMount: "approle"}
	acc := TenantAccessors{Reader: "auth_approle_1a2b3c4d", Writer: "auth_approle_5e6f7a8b"}
	for _, name := range []string{"provisioning", "operator", "apps-auth", "apps-reader", "apps-writer"} {
		hcl, err := RenderPolicy(name, cfg, acc)
		if err != nil {
			t.Fatal(err)
		}
		if err := LintOrError(name, hcl, cfg); err != nil {
			t.Fatal(err)
		}
		if name == "operator" {
			if !strings.Contains(hcl, `path "sys/generate-root"`) || !strings.Contains(hcl, `path "sys/generate-root/*"`) {
				t.Fatal("operator must be able to start generate-root")
			}
			if !strings.Contains(hcl, `path "sys/storage/raft/snapshot-force"`) {
				t.Fatal("operator must still deny snapshot-force")
			}
		}
	}
	for _, name := range []string{"tenant-reader", "tenant-writer", "tenant-database"} {
		hcl, err := RenderPolicy(name, cfg, acc)
		if err != nil {
			t.Fatal(err)
		}
		if err := LintOrError(name, hcl, cfg); err != nil {
			t.Fatal(err)
		}
		want := "{{identity.entity.aliases." + acc.Reader + ".metadata.role_name}}"
		if name != "tenant-reader" {
			want = "{{identity.entity.aliases." + acc.Writer + ".metadata.role_name}}"
		}
		if !strings.Contains(hcl, want) {
			t.Fatalf("%s must scope to the login role name %s:\n%s", name, want, hcl)
		}
	}
	if _, err := RenderPolicy("tenant-reader", cfg, TenantAccessors{}); err == nil {
		t.Fatal("tenant policies must not render without mount accessors")
	}

	prov, err := RenderPolicy("provisioning", cfg, acc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prov, "sys/policies/acl/tenant") {
		t.Fatal("provisioning must not write policies")
	}
	for _, s := range dbCreationStatements() {
		if !strings.Contains(prov, strconv.Quote(s)) {
			t.Fatalf("provisioning must pin creation statement %q", s)
		}
	}
}
