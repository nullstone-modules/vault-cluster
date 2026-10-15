package vaultcluster

import (
	"fmt"
	"log"

	"github.com/hashicorp/vault/api"
)

func (c *Client) Configure() error {
	if c.Cfg.Token == "" {
		return fmt.Errorf("VAULT_TOKEN is not set")
	}
	log.Printf("configuring platform at %s", c.Cfg.Addr)
	if err := c.enableAudit(); err != nil {
		return err
	}
	if err := c.mountKV(); err != nil {
		return err
	}
	acc, err := c.enableTenantAppRoles()
	if err != nil {
		return err
	}
	for _, name := range []string{"provisioning", "operator", "apps-auth", "apps-reader", "apps-writer", "tenant-reader", "tenant-writer", "tenant-database"} {
		hcl, err := RenderPolicy(name, c.Cfg, acc)
		if err != nil {
			return err
		}
		if err := LintOrError(name, hcl, c.Cfg); err != nil {
			return err
		}
		if err := c.API.Sys().PutPolicy(name, hcl); err != nil {
			return fmt.Errorf("apply policy %s: %w", name, err)
		}
		log.Printf("applied policy %s", name)
	}
	if err := c.enableAppsAuth(); err != nil {
		return err
	}
	if err := c.mountDatabase(); err != nil {
		return err
	}
	log.Printf("platform configuration complete")
	return nil
}

func (c *Client) enableAudit() error {
	if !c.Cfg.EnableAudit {
		log.Printf("ENABLE_AUDIT is false; skipping")
		return nil
	}
	if c.Cfg.AuditPath == "" {
		return fmt.Errorf("AUDIT_LOG_PATH is required")
	}
	audits, err := c.API.Sys().ListAudit()
	if err != nil {
		return err
	}
	if _, ok := audits[c.Cfg.AuditDevice+"/"]; ok {
		log.Printf("audit device %s already enabled", c.Cfg.AuditDevice)
		return nil
	}
	return c.API.Sys().EnableAuditWithOptions(c.Cfg.AuditDevice, &api.EnableAuditOptions{
		Type:        "file",
		Description: "Platform audit device",
		Options: map[string]string{
			"file_path":     c.Cfg.AuditPath,
			"log_raw":       "false",
			"hmac_accessor": "true",
			"mode":          "0600",
			"format":        "json",
		},
	})
}

func (c *Client) mountKV() error {
	mounts, err := c.API.Sys().ListMounts()
	if err != nil {
		return err
	}
	if _, ok := mounts[c.Cfg.KVMount+"/"]; !ok {
		if err := c.API.Sys().Mount(c.Cfg.KVMount, &api.MountInput{
			Type:        "kv",
			Description: "Multi-tenant secret store",
			Options:     map[string]string{"version": "2"},
		}); err != nil {
			return err
		}
	}
	tune, err := c.API.Logical().Read("sys/mounts/" + c.Cfg.KVMount + "/tune")
	if err != nil {
		return err
	}
	ver := ""
	if tune != nil {
		if opts, ok := tune.Data["options"].(map[string]any); ok {
			if v, ok := opts["version"].(string); ok {
				ver = v
			}
		}
	}
	if ver != "2" {
		return fmt.Errorf("%s/ is not KV v2", c.Cfg.KVMount)
	}
	_, err = c.API.Logical().Write(c.Cfg.KVMount+"/config", map[string]any{
		"max_versions":         10,
		"cas_required":         false,
		"delete_version_after": "0s",
	})
	return err
}

// enableTenantAppRoles mounts one AppRole backend per tenant access kind and returns their accessors.
func (c *Client) enableTenantAppRoles() (TenantAccessors, error) {
	var acc TenantAccessors
	for _, kind := range []string{"reader", "writer"} {
		mount := c.Cfg.TenantMount(kind)
		auths, err := c.API.Sys().ListAuth()
		if err != nil {
			return acc, err
		}
		if _, ok := auths[mount+"/"]; !ok {
			if err := c.API.Sys().EnableAuthWithOptions(mount, &api.EnableAuthOptions{
				Type:        "approle",
				Description: "Tenant " + kind + " identities (role name = tenant ID)",
			}); err != nil {
				return acc, err
			}
			if auths, err = c.API.Sys().ListAuth(); err != nil {
				return acc, err
			}
		}
		if err := c.API.Sys().TuneMount("auth/"+mount, api.MountConfigInput{
			DefaultLeaseTTL: c.Cfg.TokenTTL,
			MaxLeaseTTL:     c.Cfg.TokenMaxTTL,
		}); err != nil {
			return acc, err
		}
		a := auths[mount+"/"]
		if a == nil || a.Accessor == "" {
			return acc, fmt.Errorf("auth mount %s has no accessor", mount)
		}
		if kind == "reader" {
			acc.Reader = a.Accessor
		} else {
			acc.Writer = a.Accessor
		}
	}
	return acc, nil
}

func (c *Client) mountDatabase() error {
	if !c.Cfg.EnableCredentials {
		log.Printf("dynamic credentials disabled")
		return nil
	}
	if c.Cfg.DatabaseURL == "" || c.Cfg.DatabaseUsername == "" || c.Cfg.DatabasePassword == "" {
		return fmt.Errorf("database connection settings are required when credentials are enabled")
	}
	mounts, err := c.API.Sys().ListMounts()
	if err != nil {
		return err
	}
	if _, ok := mounts[c.Cfg.DatabaseMount+"/"]; !ok {
		if err := c.API.Sys().Mount(c.Cfg.DatabaseMount, &api.MountInput{
			Type:        "database",
			Description: "Dynamic database credentials",
			Config: api.MountConfigInput{
				DefaultLeaseTTL: c.Cfg.DatabaseTTL,
				MaxLeaseTTL:     c.Cfg.DatabaseMaxTTL,
			},
		}); err != nil {
			return err
		}
	}
	_, err = c.API.Logical().Write(c.Cfg.DatabaseMount+"/config/"+c.Cfg.DatabaseConnName, map[string]any{
		"plugin_name":             "postgresql-database-plugin",
		"connection_url":          c.Cfg.DatabaseURL,
		"username":                c.Cfg.DatabaseUsername,
		"password":                c.Cfg.DatabasePassword,
		"allowed_roles":           []string{"tenant-*"},
		"max_open_connections":    8,
		"max_idle_connections":    2,
		"max_connection_lifetime": "5m",
		"verify_connection":       true,
		"password_authentication": "password",
	})
	return err
}

// AppsAuthRole is the auth/aws role the cluster function logs in with. It carries the apps-auth policy.
const AppsAuthRole = "apps-auth"

// enableAppsAuth mounts auth/aws and binds the cluster function's execution role to the apps-auth policy.
// The function holds no token of its own: it logs in per invoke and revokes the token when done.
// Without AdminRoleARN (local stack) the mount and role are left alone.
func (c *Client) enableAppsAuth() error {
	if c.Cfg.AdminRoleARN == "" {
		log.Printf("VAULT_ADMIN_ROLE_ARN is not set; skipping the apps-auth login role")
		return nil
	}
	auths, err := c.API.Sys().ListAuth()
	if err != nil {
		return err
	}
	if _, ok := auths["aws/"]; !ok {
		if err := c.API.Sys().EnableAuthWithOptions("aws", &api.EnableAuthOptions{
			Type:        "aws",
			Description: "IAM logins: the cluster function and app brokers",
		}); err != nil {
			return fmt.Errorf("enable auth/aws: %w", err)
		}
	}
	if _, err := c.API.Logical().Write("auth/aws/role/"+AppsAuthRole, map[string]any{
		"auth_type":               "iam",
		"bound_iam_principal_arn": []string{c.Cfg.AdminRoleARN},
		"token_policies":          []string{AppsAuthRole},
		"token_ttl":               "5m",
		"token_max_ttl":           "5m",
	}); err != nil {
		return fmt.Errorf("write auth/aws/role/%s: %w", AppsAuthRole, err)
	}
	log.Printf("bound %s to auth/aws/role/%s", c.Cfg.AdminRoleARN, AppsAuthRole)
	return nil
}
