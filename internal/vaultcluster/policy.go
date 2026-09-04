package vaultcluster

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster/policies"
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
	tpl, err := template.New(name).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", templateName, err)
	}
	data := struct {
		TenantID      string
		KVMount       string
		TenantPrefix  string
		DatabaseMount string
		AuthMount     string
	}{tenantID, cfg.KVMount, cfg.TenantPrefix, cfg.DatabaseMount, cfg.AuthMount}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", templateName, err)
	}
	return b.String(), nil
}
