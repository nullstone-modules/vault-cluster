package vaultcluster

import (
	"fmt"
	"strings"

	"github.com/nullstone-modules/vault-cluster/config/policies"
)

func RenderPolicy(templateName, tenantID string, cfg Config) (string, error) {
	if tenantID != "" {
		if err := ValidateTenantID(tenantID); err != nil {
			return "", err
		}
	}
	name := templateName + ".hcl.tpl"
	raw, err := policies.Templates.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("no such template %q: %w", templateName, err)
	}
	out := string(raw)
	repl := map[string]string{
		"@@TENANT_ID@@":      tenantID,
		"@@KV_MOUNT@@":       cfg.KVMount,
		"@@TENANT_PREFIX@@":  cfg.TenantPrefix,
		"@@DATABASE_MOUNT@@": cfg.DatabaseMount,
		"@@AUTH_MOUNT@@":     cfg.AuthMount,
	}
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	if i := strings.Index(out, "@@"); i >= 0 {
		end := strings.Index(out[i+2:], "@@")
		if end >= 0 {
			return "", fmt.Errorf("unsubstituted placeholder %s", out[i:i+4+end])
		}
		return "", fmt.Errorf("unsubstituted placeholder remaining")
	}
	return out, nil
}
