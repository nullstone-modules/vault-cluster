package vaultcluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/vault/api"
)

// Tenant secret IDs are minted by apps right before login, so they live seconds and work once.
const (
	tenantSecretIDTTL  = "60s"
	tenantSecretIDUses = 1
)

// CreateTenant writes the tenant's reader and writer AppRole roles and, with credentials enabled, its
// database roles. Tenant policies are static, so nothing here writes a policy. No credentials are issued:
// apps mint their own at login.
//
// env is empty on an unshared cluster. On a shared cluster it is required: the roles are named env.tenant
// and each gets an entity carrying env and tenant as metadata, which the tenant policies template.
func (c *Client) CreateTenant(env, tenantID string) error {
	if err := ValidateTenantID(tenantID); err != nil {
		return err
	}
	if err := c.Cfg.checkEnv(env); err != nil {
		return err
	}
	role := c.Cfg.TenantRole(env, tenantID)
	if err := c.writeAppRole("reader", role, []string{c.Cfg.TenantPolicy("reader")}); err != nil {
		return err
	}
	writerPolicies := []string{c.Cfg.TenantPolicy("writer")}
	if c.Cfg.EnableCredentials {
		writerPolicies = append(writerPolicies, c.Cfg.TenantPolicy("database"))
	}
	if err := c.writeAppRole("writer", role, writerPolicies); err != nil {
		return err
	}
	if c.Cfg.SharedEnvs {
		for _, kind := range []string{"reader", "writer"} {
			if err := c.bindTenantEntity(kind, env, tenantID); err != nil {
				return err
			}
		}
	}

	if c.Cfg.EnableCredentials {
		if err := c.writeDBRole(role, "readonly", "app_readonly"); err != nil {
			return err
		}
		if err := c.writeDBRole(role, "readwrite", "app_readwrite"); err != nil {
			return err
		}
	}

	log.Printf("tenant %s onboarded", role)
	return nil
}

func (c *Client) writeAppRole(kind, role string, policies []string) error {
	_, err := c.API.Logical().Write("auth/"+c.Cfg.TenantMount(kind)+"/role/"+role, map[string]any{
		"token_policies":     policies,
		"token_ttl":          c.Cfg.TokenTTL,
		"token_max_ttl":      c.Cfg.TokenMaxTTL,
		"token_type":         "service",
		"secret_id_ttl":      tenantSecretIDTTL,
		"secret_id_num_uses": tenantSecretIDUses,
		"bind_secret_id":     true,
	})
	return err
}

// tenantEntityName is the identity entity for one tenant role on one mount.
func (c *Client) tenantEntityName(kind, env, tenantID string) string {
	return c.Cfg.TenantMount(kind) + "/" + c.Cfg.TenantRole(env, tenantID)
}

// bindTenantEntity gives the tenant role on one mount an entity with env and tenant metadata, aliased by the
// role's role_id so every login on that role lands on it. Re-runs are no-ops; an entity Vault auto-created for
// an earlier login is unlinked so the metadata-bearing one wins.
func (c *Client) bindTenantEntity(kind, env, tenantID string) error {
	mount := c.Cfg.TenantMount(kind)
	role := c.Cfg.TenantRole(env, tenantID)
	rid, err := c.API.Logical().Read("auth/" + mount + "/role/" + role + "/role-id")
	if err != nil {
		return err
	}
	if rid == nil || rid.Data["role_id"] == nil {
		return fmt.Errorf("role %s/%s has no role_id", mount, role)
	}
	accessor, err := c.authAccessor(mount)
	if err != nil {
		return err
	}
	return c.ensureEntityAlias(c.tenantEntityName(kind, env, tenantID),
		map[string]any{entityMetaEnv: env, entityMetaTenant: tenantID},
		fmt.Sprint(rid.Data["role_id"]), accessor)
}

func (c *Client) authAccessor(mount string) (string, error) {
	auths, err := c.API.Sys().ListAuth()
	if err != nil {
		return "", err
	}
	a := auths[mount+"/"]
	if a == nil || a.Accessor == "" {
		return "", fmt.Errorf("auth mount %s has no accessor", mount)
	}
	return a.Accessor, nil
}

// ensureEntityAlias upserts the entity by name with metadata and points the (aliasName, accessor) alias at it.
func (c *Client) ensureEntityAlias(name string, metadata map[string]any, aliasName, accessor string) error {
	if _, err := c.API.Logical().Write("identity/entity/name/"+name, map[string]any{"metadata": metadata}); err != nil {
		return fmt.Errorf("write entity %s: %w", name, err)
	}
	ent, err := c.API.Logical().Read("identity/entity/name/" + name)
	if err != nil {
		return err
	}
	if ent == nil || ent.Data["id"] == nil {
		return fmt.Errorf("entity %s was not created", name)
	}
	entityID := fmt.Sprint(ent.Data["id"])

	existing, err := c.API.Logical().Write("identity/lookup/entity", map[string]any{
		"alias_name": aliasName, "alias_mount_accessor": accessor,
	})
	if err != nil {
		return fmt.Errorf("lookup alias: %w", err)
	}
	if existing != nil && existing.Data != nil {
		if fmt.Sprint(existing.Data["id"]) == entityID {
			return nil
		}
		for _, a := range aliasList(existing.Data["aliases"]) {
			if a["name"] == aliasName && a["mount_accessor"] == accessor {
				if err := c.deleteMissingOK("identity/entity-alias/id/" + fmt.Sprint(a["id"])); err != nil {
					return fmt.Errorf("unlink stale alias: %w", err)
				}
			}
		}
	}
	_, err = c.API.Logical().Write("identity/entity-alias", map[string]any{
		"name": aliasName, "canonical_id": entityID, "mount_accessor": accessor,
	})
	if err != nil {
		return fmt.Errorf("write alias for %s: %w", name, err)
	}
	return nil
}

func aliasList(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// dbCreationStatements are the only statements a tenant database role may run. The provisioning
// policy pins creation_statements to these values, so a role cannot grant more than the app groups.
func dbCreationStatements() []string {
	return []string{
		`CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';`,
		`GRANT app_readonly TO "{{name}}";`,
		`GRANT app_readwrite TO "{{name}}";`,
	}
}

func (c *Client) writeDBRole(tenantRole, suffix, group string) error {
	role := fmt.Sprintf("tenant-%s-%s", tenantRole, suffix)
	stmts := dbCreationStatements()
	grant := fmt.Sprintf(`GRANT %s TO "{{name}}";`, group)
	found := false
	for _, s := range stmts[1:] {
		found = found || s == grant
	}
	if !found {
		return fmt.Errorf("unknown database group %q", group)
	}
	_, err := c.API.Logical().Write(c.Cfg.DatabaseMount+"/roles/"+role, map[string]any{
		"db_name":             c.Cfg.DatabaseConnName,
		"creation_statements": []string{stmts[0], grant},
		"default_ttl":         c.Cfg.DatabaseTTL,
		"max_ttl":             c.Cfg.DatabaseMaxTTL,
	})
	return err
}

// LoginAppRole logs in to role on an AppRole mount with a fresh secret ID, as an app does for one tenant.
func (c *Client) LoginAppRole(mount, role string) (string, error) {
	path := "auth/" + mount + "/role/" + role
	s, err := c.API.Logical().Read(path + "/role-id")
	if err != nil {
		return "", err
	}
	if s == nil {
		return "", fmt.Errorf("approle role %s/%s not found", mount, role)
	}
	sec, err := c.API.Logical().Write(path+"/secret-id", map[string]any{})
	if err != nil {
		return "", err
	}
	login, err := c.API.Logical().Write("auth/"+mount+"/login", map[string]any{
		"role_id":   s.Data["role_id"],
		"secret_id": sec.Data["secret_id"],
	})
	if err != nil {
		return "", err
	}
	if login == nil || login.Auth == nil {
		return "", fmt.Errorf("approle login returned no auth")
	}
	return login.Auth.ClientToken, nil
}

func (c *Client) OffboardTenant(env, tenantID string, purge bool) error {
	if err := ValidateTenantID(tenantID); err != nil {
		return err
	}
	if err := c.Cfg.checkEnv(env); err != nil {
		return err
	}
	role := c.Cfg.TenantRole(env, tenantID)
	for _, kind := range []string{"reader", "writer"} {
		if c.Cfg.SharedEnvs {
			if err := c.deleteMissingOK("identity/entity/name/" + c.tenantEntityName(kind, env, tenantID)); err != nil {
				return fmt.Errorf("delete %s entity %s: %w", kind, role, err)
			}
		}
		if err := c.deleteMissingOK("auth/" + c.Cfg.TenantMount(kind) + "/role/" + role); err != nil {
			return fmt.Errorf("delete %s role %s: %w", kind, role, err)
		}
	}
	if c.Cfg.EnableCredentials {
		for _, suffix := range []string{"readonly", "readwrite"} {
			dbRole := fmt.Sprintf("tenant-%s-%s", role, suffix)
			_ = c.API.Sys().RevokePrefix(c.Cfg.DatabaseMount + "/creds/" + dbRole)
			if err := c.deleteMissingOK(c.Cfg.DatabaseMount + "/roles/" + dbRole); err != nil {
				return fmt.Errorf("delete database role %s: %w", dbRole, err)
			}
		}
	}
	if purge {
		if err := c.purgeKVPrefix(c.Cfg.KVMetaPath(env, tenantID, "")); err != nil {
			return err
		}
	}
	log.Printf("tenant %s offboarded", role)
	return nil
}

// purgeKVPrefix destroys every secret under one KV v2 metadata path, recursing into folders.
func (c *Client) purgeKVPrefix(meta string) error {
	r, err := c.Do("GET", meta+"?list=true", nil)
	if err != nil && r.Status == 0 {
		return err
	}
	if r.Status == 403 {
		return fmt.Errorf("permission denied listing %s (provisioning cannot purge tenant data)", meta)
	}
	if r.Status == 404 {
		return nil
	}
	if r.Status < 200 || r.Status >= 300 {
		return fmt.Errorf("list %s failed (HTTP %d)", meta, r.Status)
	}
	var wrap struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &wrap); err != nil {
		return err
	}
	for _, key := range wrap.Data.Keys {
		if strings.HasSuffix(key, "/") {
			if err := c.purgeKVPrefix(meta + "/" + strings.TrimSuffix(key, "/")); err != nil {
				return err
			}
			continue
		}
		path := meta + "/" + key
		if err := c.deleteMissingOK(path); err != nil {
			return fmt.Errorf("destroy %s: %w", path, err)
		}
	}
	return nil
}

func (c *Client) deleteMissingOK(path string) error {
	r, err := c.Do("DELETE", path, nil)
	if err != nil && r.Status == 0 {
		return err
	}
	if r.Status == 404 || (r.Status >= 200 && r.Status < 300) {
		return nil
	}
	return fmt.Errorf("DELETE %s failed (HTTP %d): %s", path, r.Status, strings.TrimSpace(string(r.Body)))
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var re *api.ResponseError
	if errors.As(err, &re) && re.StatusCode == 404 {
		return true
	}
	return strings.Contains(err.Error(), "404")
}
