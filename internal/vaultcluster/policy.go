package vaultcluster

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster/policies"
)

// TenantAccessors are the mount accessors of the tenant AppRole mounts. On an unshared cluster the static
// tenant policies template the tenant ID from the login's role_name alias metadata on these mounts. On a
// shared cluster they are used to attach the per-tenant entity alias instead.
type TenantAccessors struct {
	Reader string
	Writer string
}

// Entity metadata keys written on a shared cluster. Tenant policies template both; broker policies template env.
const (
	entityMetaEnv    = "env"
	entityMetaTenant = "tenant"
)

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
		Shared        bool
		KVMount       string
		TenantPrefix  string
		EnvPrefix     string
		DatabaseMount string
		ReaderMount   string
		WriterMount   string
		// ReaderTenant / WriterTenant name the caller's tenant. Unshared: the role_name alias on that mount.
		// Shared: env and tenant from entity metadata, as one path segment pair.
		ReaderTenant string
		WriterTenant string
		// Env is the caller's env from entity metadata (shared only).
		Env                  string
		DBCreationStatements string
	}{
		Shared:               cfg.SharedEnvs,
		KVMount:              cfg.KVMount,
		TenantPrefix:         cfg.TenantPrefix,
		EnvPrefix:            cfg.EnvPrefix,
		DatabaseMount:        cfg.DatabaseMount,
		ReaderMount:          cfg.TenantMount("reader"),
		WriterMount:          cfg.TenantMount("writer"),
		ReaderTenant:         roleNameTemplate(acc.Reader),
		WriterTenant:         roleNameTemplate(acc.Writer),
		Env:                  entityMetaTemplate(entityMetaEnv),
		DBCreationStatements: strings.Join(stmts, ", "),
	}
	if cfg.SharedEnvs {
		data.ReaderTenant = entityMetaTemplate(entityMetaTenant)
		data.WriterTenant = entityMetaTemplate(entityMetaTenant)
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

// entityMetaTemplate is the Vault ACL template for one entity metadata key.
func entityMetaTemplate(key string) string {
	return "{{identity.entity.metadata." + key + "}}"
}
