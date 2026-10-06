package vaultcluster

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster/policies"
)

// TenantAccessors are the mount accessors of the tenant AppRole mounts. Static tenant policies
// template the tenant ID from the login's role_name alias metadata on these mounts.
type TenantAccessors struct {
	Reader string
	Writer string
}

func RenderPolicy(templateName string, cfg Config, acc TenantAccessors) (string, error) {
	name := templateName + ".hcl.tpl"
	raw, err := policies.Templates.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("no such template %q: %w", templateName, err)
	}
	tpl, err := template.New(name).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", templateName, err)
	}
	if strings.HasPrefix(templateName, "tenant-") && (acc.Reader == "" || acc.Writer == "") {
		return "", fmt.Errorf("render template %q: tenant mount accessors are required", templateName)
	}
	var stmts []string
	for _, s := range dbCreationStatements() {
		stmts = append(stmts, strconv.Quote(s))
	}
	data := struct {
		KVMount              string
		TenantPrefix         string
		DatabaseMount        string
		ReaderMount          string
		WriterMount          string
		ReaderTenant         string
		WriterTenant         string
		DBCreationStatements string
	}{
		KVMount:              cfg.KVMount,
		TenantPrefix:         cfg.TenantPrefix,
		DatabaseMount:        cfg.DatabaseMount,
		ReaderMount:          cfg.TenantMount("reader"),
		WriterMount:          cfg.TenantMount("writer"),
		ReaderTenant:         roleNameTemplate(acc.Reader),
		WriterTenant:         roleNameTemplate(acc.Writer),
		DBCreationStatements: strings.Join(stmts, ", "),
	}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", templateName, err)
	}
	return b.String(), nil
}

// roleNameTemplate is the Vault ACL template for the AppRole role name of a login on the mount with accessor.
func roleNameTemplate(accessor string) string {
	return "{{identity.entity.aliases." + accessor + ".metadata.role_name}}"
}
