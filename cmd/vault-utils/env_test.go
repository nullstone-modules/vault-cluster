package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nullstone-modules/vault-cluster/internal/nsenv"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func TestPrintEnvWarnsOnStderrOnly(t *testing.T) {
	ws := nsenv.Workspace{
		Module:   "nullstone/aws-ec2-vault-cluster",
		Contract: types.ModuleContractName{Category: "datastore", Provider: "aws", Platform: "vault", Subplatform: "ec2"},
		Outputs:  map[string]any{"vault_addr": "http://127.0.0.1:1", "vault_fqdn": "vault.internal"},
	}
	var stdout, stderr bytes.Buffer
	if err := printEnv(context.Background(), &stdout, &stderr, ws, "bash", true); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "export VAULT_ADDR='http://127.0.0.1:1'\n") || strings.Contains(stdout.String(), "warning") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unreachable") || !strings.Contains(stderr.String(), "VPN") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPrintEnvRejectsOtherContracts(t *testing.T) {
	ws := nsenv.Workspace{
		Module:   "nullstone/aws-rds-postgres",
		Contract: types.ModuleContractName{Category: "datastore", Provider: "aws", Platform: "postgres", Subplatform: "rds"},
		Outputs:  map[string]any{"vault_addr": "http://127.0.0.1:1"},
	}
	var stdout, stderr bytes.Buffer
	err := printEnv(context.Background(), &stdout, &stderr, ws, "bash", false)
	if err == nil || !strings.Contains(err.Error(), "not a Vault cluster") || stdout.Len() != 0 {
		t.Fatalf("err = %v stdout = %q", err, stdout.String())
	}
}
