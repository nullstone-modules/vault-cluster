package vaultcluster

import (
	"os"
	"path/filepath"
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
	for _, name := range []string{"provisioning", "operator"} {
		hcl, err := RenderPolicy(name, "", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := LintOrError(name, hcl, cfg); err != nil {
			t.Fatal(err)
		}
	}
	for _, tmpl := range []string{"tenant-reader", "tenant-writer", "tenant-database"} {
		hcl, err := RenderPolicy(tmpl, "tenant-a", cfg)
		if err != nil {
			t.Fatal(err)
		}
		name := "tenant-tenant-a-" + strings.TrimPrefix(tmpl, "tenant-")
		if err := LintOrError(name, hcl, cfg); err != nil {
			t.Fatal(err)
		}
	}
}
