package nsenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var awsEC2 = types.ModuleContractName{Category: "datastore", Provider: "aws", Platform: "vault", Subplatform: "ec2"}

var tlsCluster = map[string]any{
	"vault_fqdn":      "vault.internal",
	"user_fqdn":       "vault.acme.example.com",
	"vault_api_port":  "8200",
	"vault_addr":      "https://vault.internal:8200",
	"user_vault_addr": "https://vault.acme.example.com:8200",
	"tls_server_name": "vault.acme.example.com",
}

func TestSettingsFromOutputs(t *testing.T) {
	tests := []struct {
		name     string
		outputs  map[string]any
		internal bool
		want     Settings
		wantNote bool
		wantErr  bool
	}{
		{name: "user-facing", outputs: tlsCluster, want: Settings{Addr: "https://vault.acme.example.com:8200"}},
		{name: "internal over tls", outputs: tlsCluster, internal: true, want: Settings{Addr: "https://vault.internal:8200", TLSServerName: "vault.acme.example.com"}},
		{name: "missing user_fqdn", outputs: map[string]any{
			"vault_fqdn": "vault.internal", "user_fqdn": "", "vault_api_port": "8200",
			"vault_addr": "http://vault.internal:8200", "user_vault_addr": "",
		}, want: Settings{Addr: "http://vault.internal:8200"}, wantNote: true},
		{name: "legacy outputs with subdomain", outputs: map[string]any{
			"vault_fqdn": "vault.internal", "user_fqdn": "vault.acme.example.com", "vault_api_port": "8200",
		}, internal: true, want: Settings{Addr: "https://vault.internal:8200", TLSServerName: "vault.acme.example.com"}},
		{name: "legacy outputs without subdomain", outputs: map[string]any{
			"vault_fqdn": "vault.internal", "vault_api_port": "8443",
		}, want: Settings{Addr: "http://vault.internal:8443"}, wantNote: true},
		{name: "not applied", outputs: map[string]any{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SettingsFromOutputs(Workspace{Contract: awsEC2, Outputs: tt.outputs}, tt.internal)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tt.wantErr {
				return
			}
			if (got.Note != "") != tt.wantNote {
				t.Fatalf("note = %q", got.Note)
			}
			got.Note = ""
			if got != tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	withSNI := Settings{Addr: "https://vault.internal:8200", TLSServerName: "vault.acme.example.com"}
	plain := Settings{Addr: "http://vault.internal:8200"}
	tests := []struct {
		shell string
		in    Settings
		want  string
	}{
		{"bash", withSNI, "export VAULT_ADDR='https://vault.internal:8200'\nexport VAULT_TLS_SERVER_NAME='vault.acme.example.com'\n"},
		{"zsh", plain, "export VAULT_ADDR='http://vault.internal:8200'\nunset VAULT_TLS_SERVER_NAME\n"},
		{"fish", withSNI, "set -gx VAULT_ADDR 'https://vault.internal:8200';\nset -gx VAULT_TLS_SERVER_NAME 'vault.acme.example.com';\n"},
		{"fish", plain, "set -gx VAULT_ADDR 'http://vault.internal:8200';\nset -e VAULT_TLS_SERVER_NAME;\n"},
		{"powershell", withSNI, "$env:VAULT_ADDR = 'https://vault.internal:8200'\n$env:VAULT_TLS_SERVER_NAME = 'vault.acme.example.com'\n"},
		{"powershell", plain, "$env:VAULT_ADDR = 'http://vault.internal:8200'\nRemove-Item Env:VAULT_TLS_SERVER_NAME -ErrorAction SilentlyContinue\n"},
		{"bash", Settings{Addr: "http://a'b"}, "export VAULT_ADDR='http://a'\\''b'\nunset VAULT_TLS_SERVER_NAME\n"},
		{"powershell", Settings{Addr: "http://a'b"}, "$env:VAULT_ADDR = 'http://a''b'\nRemove-Item Env:VAULT_TLS_SERVER_NAME -ErrorAction SilentlyContinue\n"},
	}
	for _, tt := range tests {
		got, err := Render(tt.shell, tt.in)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Fatalf("%s:\n got %q\nwant %q", tt.shell, got, tt.want)
		}
	}
	if _, err := Render("cmd", plain); err == nil {
		t.Fatal("expected unknown shell to fail")
	}
}

type fakeResolver struct{ ws Workspace }

func (f fakeResolver) Workspace(context.Context, string, string, string) (Workspace, error) {
	return f.ws, nil
}

func TestCheckContract(t *testing.T) {
	tests := []struct {
		name     string
		contract types.ModuleContractName
		ok       bool
	}{
		{"ec2 cluster", types.ModuleContractName{Category: "datastore", Provider: "aws", Platform: "vault", Subplatform: "ec2"}, true},
		{"postgres", types.ModuleContractName{Category: "datastore", Provider: "aws", Platform: "postgres", Subplatform: "rds"}, false},
		{"vault access capability", types.ModuleContractName{Category: "capability", Subcategory: "datastores", Provider: "aws", Platform: "vault"}, false},
		{"gcp vault", types.ModuleContractName{Category: "datastore", Provider: "gcp", Platform: "vault", Subplatform: "gce"}, true},
		{"azure vault", types.ModuleContractName{Category: "datastore", Provider: "azure", Platform: "vault"}, true},
	}
	for _, tt := range tests {
		var r Resolver = fakeResolver{ws: Workspace{Module: "nullstone/x", Contract: tt.contract, Outputs: tlsCluster}}
		ws, _ := r.Workspace(context.Background(), "s", "e", "b")
		err := CheckContract(ws)
		if (err == nil) != tt.ok {
			t.Fatalf("%s: err = %v", tt.name, err)
		}
	}
}

func TestLoadProfile(t *testing.T) {
	for _, k := range []string{"NULLSTONE_ADDR", "NULLSTONE_API_KEY", "NULLSTONE_ORG"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("config", `{"name":"other","address":"https://api.example.test"}`)
	write("key", "not-a-real-key\n")
	write("org", "acme")

	p, err := loadProfile(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "work" || p.Address != "https://api.example.test" || p.Org != "acme" {
		t.Fatalf("profile = %+v", p)
	}
	cfg, err := p.APIConfig("")
	if err != nil || cfg.OrgName != "acme" {
		t.Fatalf("cfg = %+v, %v", cfg, err)
	}

	t.Setenv("NULLSTONE_ORG", "beta")
	t.Setenv("NULLSTONE_API_KEY", "not-a-real-env-key")
	p, _ = loadProfile(dir, "work")
	if p.Org != "beta" || p.apiKey != "not-a-real-env-key" {
		t.Fatalf("env overrides: %+v", p.Org)
	}

	t.Setenv("NULLSTONE_API_KEY", "")
	empty, _ := loadProfile(t.TempDir(), "default")
	if _, err := empty.APIConfig("acme"); err == nil || strings.Contains(err.Error(), "not-a-real") {
		t.Fatalf("missing key: %v", err)
	}
}
