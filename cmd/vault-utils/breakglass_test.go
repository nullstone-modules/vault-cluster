package main

import "testing"

func TestParseBreakGlassArgs(t *testing.T) {
	got, err := parseBreakGlassArgs([]string{"begin", "--reason", "expired token", "--second", "alex", "--org", "acme", "--stack", "core", "--env", "prod", "--block", "vault"})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "begin" || got.reason != "expired token" || got.second != "alex" || got.stack != "core" || got.env != "prod" || got.block != "vault" || !got.usesNullstone() {
		t.Fatalf("got %+v", got)
	}
	got, err = parseBreakGlassArgs([]string{"end"})
	if err != nil || got.sub != "end" || got.usesNullstone() {
		t.Fatalf("end: %+v %v", got, err)
	}
	got, err = parseBreakGlassArgs([]string{"end", "--stack", "core", "--env", "prod", "--block", "vault", "--internal"})
	if err != nil || !got.usesNullstone() || !got.internal {
		t.Fatalf("end with cluster: %+v %v", got, err)
	}
}

func TestParseBreakGlassArgsRefusals(t *testing.T) {
	cases := map[string][]string{
		"no subcommand":   {},
		"unknown":         {"remint"},
		"missing reason":  {"begin", "--second", "s"},
		"missing second":  {"begin", "--reason", "r"},
		"blank reason":    {"begin", "--reason", "  ", "--second", "s"},
		"partial cluster": {"begin", "--reason", "r", "--second", "s", "--stack", "core"},
		"unknown flag":    {"begin", "--reason", "r", "--second", "s", "--force"},
		"extra argument":  {"end", "now"},
	}
	for name, args := range cases {
		if _, err := parseBreakGlassArgs(args); err == nil {
			t.Errorf("%s: accepted %v", name, args)
		}
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
