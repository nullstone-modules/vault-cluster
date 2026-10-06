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

// CreateTenant writes the tenant's reader and writer AppRole roles (role name = tenant ID) and, with
// credentials enabled, its database roles. Tenant policies are static, so nothing here writes a policy.
// No credentials are issued: apps mint their own at login.
func (c *Client) CreateTenant(tenantID string) error {
	if err := ValidateTenantID(tenantID); err != nil {
		return err
	}
	if err := c.writeAppRole("reader", tenantID, []string{c.Cfg.TenantPolicy("reader")}); err != nil {
		return err
	}
	writerPolicies := []string{c.Cfg.TenantPolicy("writer")}
	if c.Cfg.EnableCredentials {
		writerPolicies = append(writerPolicies, c.Cfg.TenantPolicy("database"))
	}
	if err := c.writeAppRole("writer", tenantID, writerPolicies); err != nil {
		return err
	}

	if c.Cfg.EnableCredentials {
		if err := c.writeDBRole(tenantID, "readonly", "app_readonly"); err != nil {
			return err
		}
		if err := c.writeDBRole(tenantID, "readwrite", "app_readwrite"); err != nil {
			return err
		}
	}

	log.Printf("tenant %s onboarded", tenantID)
	return nil
}

func (c *Client) writeAppRole(kind, tenantID string, policies []string) error {
	_, err := c.API.Logical().Write("auth/"+c.Cfg.TenantMount(kind)+"/role/"+tenantID, map[string]any{
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

// dbCreationStatements are the only statements a tenant database role may run. The provisioning
// policy pins creation_statements to these values, so a role cannot grant more than the app groups.
func dbCreationStatements() []string {
	return []string{
		`CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';`,
		`GRANT app_readonly TO "{{name}}";`,
		`GRANT app_readwrite TO "{{name}}";`,
	}
}

func (c *Client) writeDBRole(tenantID, suffix, group string) error {
	role := fmt.Sprintf("tenant-%s-%s", tenantID, suffix)
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

func (c *Client) OffboardTenant(tenantID string, purge bool) error {
	if err := ValidateTenantID(tenantID); err != nil {
		return err
	}
	for _, kind := range []string{"reader", "writer"} {
		if err := c.deleteMissingOK("auth/" + c.Cfg.TenantMount(kind) + "/role/" + tenantID); err != nil {
			return fmt.Errorf("delete %s role %s: %w", kind, tenantID, err)
		}
	}
	if c.Cfg.EnableCredentials {
		for _, suffix := range []string{"readonly", "readwrite"} {
			dbRole := fmt.Sprintf("tenant-%s-%s", tenantID, suffix)
			_ = c.API.Sys().RevokePrefix(c.Cfg.DatabaseMount + "/creds/" + dbRole)
			if err := c.deleteMissingOK(c.Cfg.DatabaseMount + "/roles/" + dbRole); err != nil {
				return fmt.Errorf("delete database role %s: %w", dbRole, err)
			}
		}
	}
	if purge {
		if err := c.purgeTenantSecrets(tenantID); err != nil {
			return err
		}
	}
	log.Printf("tenant %s offboarded", tenantID)
	return nil
}

func (c *Client) purgeTenantSecrets(tenantID string) error {
	meta := c.Cfg.KVMetaPath(tenantID, "")
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
		path := strings.TrimSuffix(meta+"/"+strings.TrimSuffix(key, "/"), "/")
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
