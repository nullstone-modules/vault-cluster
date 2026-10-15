package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

func TestParseBreakGlassArgs(t *testing.T) {
	got, err := parseBreakGlassArgs([]string{"--reason", "expired token", "--second", "alex", "--org", "acme", "--stack", "core", "--env", "prod", "--block", "vault", "remint", "provisioning"})
	if err != nil {
		t.Fatal(err)
	}
	if got.reason != "expired token" || got.second != "alex" || got.stack != "core" || got.env != "prod" || got.block != "vault" || got.action != "remint" || len(got.actionArgs) != 1 || got.actionArgs[0] != "provisioning" {
		t.Fatalf("got %+v", got)
	}
	if !got.usesNullstone() {
		t.Fatal("stack/env/block should resolve through Nullstone")
	}

	got, err = parseBreakGlassArgs([]string{"--reason=r", "--second=s", "exec", "--", "vault", "token", "lookup", "-format=json"})
	if err != nil {
		t.Fatal(err)
	}
	if got.usesNullstone() || got.action != "exec" || strings.Join(got.actionArgs, " ") != "-- vault token lookup -format=json" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseBreakGlassArgsRefusals(t *testing.T) {
	cases := map[string][]string{
		"missing reason":     {"--second", "s", "remint", "provisioning"},
		"missing second":     {"--reason", "r", "remint", "provisioning"},
		"blank reason":       {"--reason", "  ", "--second", "s", "remint", "provisioning"},
		"missing action":     {"--reason", "r", "--second", "s"},
		"partial cluster":    {"--reason", "r", "--second", "s", "--stack", "core", "remint", "provisioning"},
		"unknown flag":       {"--reason", "r", "--second", "s", "--force", "remint", "provisioning"},
		"flags after action": {"--reason", "r", "remint", "provisioning", "--second", "s"},
	}
	for name, args := range cases {
		if _, err := parseBreakGlassArgs(args); err == nil {
			t.Errorf("%s: accepted %v", name, args)
		}
	}
}

func TestBuildActionRefusals(t *testing.T) {
	bg := vaultcluster.BreakGlass{Reason: "r", Second: "s", Store: vaultcluster.FileKeyStore{Dir: t.TempDir()}}
	policy := filepath.Join(t.TempDir(), "p.hcl")
	if err := os.WriteFile(policy, []byte(`path "kv/data/x" { capabilities = ["read"] }`), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := map[string]breakGlassArgs{
		"remint operator":       {action: "remint", actionArgs: []string{"operator"}},
		"remint unknown":        {action: "remint", actionArgs: []string{"root"}},
		"remint no token":       {action: "remint"},
		"policy reserved":       {action: "policy", actionArgs: []string{"write", "operator", policy}},
		"policy root":           {action: "policy", actionArgs: []string{"write", "root", policy}},
		"policy missing file":   {action: "policy", actionArgs: []string{"write", "admin", filepath.Join(t.TempDir(), "none.hcl")}},
		"policy read not write": {action: "policy", actionArgs: []string{"read", "admin"}},
		"userpass disable":      {action: "userpass", actionArgs: []string{"disable"}},
		"userpass empty admin":  {action: "userpass", actionArgs: []string{"enable", "--first-admin"}},
		"exec nothing":          {action: "exec", actionArgs: []string{"--"}},
		"unknown action":        {action: "restore"},
	}
	for name, a := range refused {
		if _, err := buildAction(a, bg, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, name := range vaultcluster.PlatformPolicies {
		if _, err := buildAction(breakGlassArgs{action: "policy", actionArgs: []string{"write", name, policy}}, bg, nil); err == nil {
			t.Errorf("platform policy %s: accepted", name)
		}
	}

	act, err := buildAction(breakGlassArgs{action: "policy", actionArgs: []string{"write", "admin", policy}}, bg, nil)
	if err != nil || act.Name() != "policy write admin" {
		t.Fatalf("admin policy: %v %v", act, err)
	}
	act, err = buildAction(breakGlassArgs{action: "userpass", actionArgs: []string{"enable", "--first-admin=alex"}}, bg, nil)
	if err != nil || act.Name() != "userpass enable --first-admin alex" {
		t.Fatalf("userpass: %v %v", act, err)
	}
	act, err = buildAction(breakGlassArgs{action: "exec", actionArgs: []string{"--", "vault", "status"}}, bg, nil)
	if err != nil || act.Name() != "exec vault status" {
		t.Fatalf("exec: %v %v", act, err)
	}
}

func TestSecretARNsFromOutputs(t *testing.T) {
	arns, err := secretARNsFromOutputs(map[string]any{
		"operator_secret_arn":     "arn:aws:secretsmanager:us-east-1:1:secret:vault/vault/operator/abc",
		"provisioning_secret_arn": "arn:aws:secretsmanager:us-east-1:1:secret:vault/vault/provisioning/abc",
	})
	if err != nil || arns.init != "" || arns.appsAuth != "" || arns.operator == "" {
		t.Fatalf("legacy outputs: %+v %v", arns, err)
	}
	if _, err := secretARNsFromOutputs(map[string]any{"vault_addr": "http://x"}); err == nil {
		t.Fatal("missing operator ARN accepted")
	}
}
