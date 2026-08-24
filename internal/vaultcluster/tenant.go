package vaultcluster

import (
	"fmt"
	"log"
)

func (c *Client) CreateTenant(tenantID string, issueCreds bool) error {
	if err := ValidateTenantID(tenantID); err != nil {
		return err
	}
	readerP := c.Cfg.TenantPolicy("reader", tenantID)
	writerP := c.Cfg.TenantPolicy("writer", tenantID)
	dbP := c.Cfg.TenantPolicy("database", tenantID)
	readerR := c.Cfg.TenantRole("reader", tenantID)
	writerR := c.Cfg.TenantRole("writer", tenantID)

	apply := func(tmpl, name string) error {
		hcl, err := RenderPolicy(tmpl, tenantID, c.Cfg)
		if err != nil {
			return err
		}
		if err := LintOrError(name, hcl, c.Cfg); err != nil {
			return err
		}
		return c.API.Sys().PutPolicy(name, hcl)
	}
	if err := apply("tenant-reader", readerP); err != nil {
		return err
	}
	if err := apply("tenant-writer", writerP); err != nil {
		return err
	}
	if c.Cfg.EnableCredentials {
		if err := apply("tenant-database", dbP); err != nil {
			return err
		}
	}

	if err := c.writeAppRole(readerR, []string{readerP}); err != nil {
		return err
	}
	writerPolicies := []string{writerP}
	if c.Cfg.EnableCredentials {
		writerPolicies = append(writerPolicies, dbP)
	}
	if err := c.writeAppRole(writerR, writerPolicies); err != nil {
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

	if !issueCreds {
		log.Printf("tenant %s onboarded (no credentials issued)", tenantID)
		return nil
	}
	if err := c.printAppRoleCreds(readerR); err != nil {
		return err
	}
	return c.printAppRoleCreds(writerR)
}

func (c *Client) writeAppRole(role string, policies []string) error {
	_, err := c.API.Logical().Write("auth/"+c.Cfg.AuthMount+"/role/"+role, map[string]any{
		"token_policies":     policies,
		"token_ttl":          c.Cfg.TokenTTL,
		"token_max_ttl":      c.Cfg.TokenMaxTTL,
		"token_type":         "service",
		"secret_id_ttl":      "24h",
		"secret_id_num_uses": 0,
		"bind_secret_id":     true,
	})
	return err
}

func (c *Client) writeDBRole(tenantID, suffix, group string) error {
	role := fmt.Sprintf("tenant-%s-%s", tenantID, suffix)
	stmt1 := `CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';`
	stmt2 := fmt.Sprintf(`GRANT %s TO "{{name}}";`, group)
	_, err := c.API.Logical().Write(c.Cfg.DatabaseMount+"/roles/"+role, map[string]any{
		"db_name":             c.Cfg.DatabaseConnName,
		"creation_statements": []string{stmt1, stmt2},
		"default_ttl":         c.Cfg.DatabaseTTL,
		"max_ttl":             c.Cfg.DatabaseMaxTTL,
	})
	return err
}

func (c *Client) printAppRoleCreds(role string) error {
	s, err := c.API.Logical().Read("auth/" + c.Cfg.AuthMount + "/role/" + role + "/role-id")
	if err != nil {
		return err
	}
	sec, err := c.API.Logical().Write("auth/"+c.Cfg.AuthMount+"/role/"+role+"/secret-id", map[string]any{})
	if err != nil {
		return err
	}
	fmt.Printf("----------------------------------------------------------\n")
	fmt.Printf("role       %s\n", role)
	fmt.Printf("role_id    %s\n", s.Data["role_id"])
	fmt.Printf("secret_id  %s\n", sec.Data["secret_id"])
	return nil
}

func (c *Client) LoginAppRole(role string) (string, error) {
	s, err := c.API.Logical().Read("auth/" + c.Cfg.AuthMount + "/role/" + role + "/role-id")
	if err != nil {
		return "", err
	}
	sec, err := c.API.Logical().Write("auth/"+c.Cfg.AuthMount+"/role/"+role+"/secret-id", map[string]any{})
	if err != nil {
		return "", err
	}
	login, err := c.API.Logical().Write("auth/"+c.Cfg.AuthMount+"/login", map[string]any{
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
