package vaultcluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
)

// AdminRolePrefix marks auth roles owned by the cluster. Apps cannot write them.
const AdminRolePrefix = "admin-"

const (
	AdminTokenTTL    = "1h"
	AdminTokenMaxTTL = "8h"
)

// Admin access levels bind existing platform policies.
var adminAccessPolicies = map[string]string{
	"tenants":  "provisioning",
	"operator": "operator",
}

var adminRoleName = regexp.MustCompile(`^admin-[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// AdminConfig is the cloud-neutral admin file each cluster module renders (VAULT_ADMINS_FILE).
type AdminConfig struct {
	Bindings []AdminBinding `json:"bindings"`
	OIDC     *OIDCConfig    `json:"oidc,omitempty"`
}

// AdminBinding binds one principal of one auth method to the Vault role Name.
// Principal is an IAM ARN (aws), a service account email (gcp), or an IdP group (oidc).
type AdminBinding struct {
	Name      string   `json:"name"`
	Method    string   `json:"method"`
	Principal string   `json:"principal"`
	Access    []string `json:"access"`
}

func AdminPolicies(access []string) ([]string, error) {
	if len(access) == 0 {
		return nil, fmt.Errorf("access is empty")
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range access {
		p, ok := adminAccessPolicies[a]
		if !ok {
			return nil, fmt.Errorf("unknown access level %q (tenants, operator)", a)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func ValidateAdminConfig(cfg AdminConfig) error {
	names := map[string]bool{}
	principals := map[string]string{}
	for _, b := range cfg.Bindings {
		if !adminRoleName.MatchString(b.Name) {
			return fmt.Errorf("invalid admin role name %q", b.Name)
		}
		if names[b.Name] {
			return fmt.Errorf("duplicate admin role %q", b.Name)
		}
		names[b.Name] = true
		auth, ok := adminAuths[b.Method]
		if !ok {
			return fmt.Errorf("%s: unknown method %q (%s)", b.Name, b.Method, strings.Join(AdminMethods(), ", "))
		}
		if err := auth.validatePrincipal(b.Principal); err != nil {
			return fmt.Errorf("%s: %w", b.Name, err)
		}
		key := b.Method + "\x00" + b.Principal
		if other, ok := principals[key]; ok {
			return fmt.Errorf("%s: principal is already bound to %s", b.Name, other)
		}
		principals[key] = b.Name
		if _, err := AdminPolicies(b.Access); err != nil {
			return fmt.Errorf("%s: %w", b.Name, err)
		}
		if b.Method == "oidc" && cfg.OIDC == nil {
			return fmt.Errorf("%s: oidc bindings need the oidc settings", b.Name)
		}
	}
	if cfg.OIDC != nil {
		return cfg.OIDC.validate()
	}
	return nil
}

func LoadAdminConfig(path string) (AdminConfig, error) {
	var cfg AdminConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, ValidateAdminConfig(cfg)
}

// ReconcileAdmins makes the admin-* roles on every supported auth mount match cfg: writes each binding,
// deletes the rest. A mount with no bindings is left disabled, or emptied of admin-* roles if enabled.
// One principal maps to one role per mount. A failed binding does not stop the others.
func (c *Client) ReconcileAdmins(cfg AdminConfig) error {
	if err := ValidateAdminConfig(cfg); err != nil {
		return err
	}
	enabled, err := c.enabledAuthMounts()
	if err != nil {
		return err
	}
	var errs []error
	for _, method := range AdminMethods() {
		var bindings []AdminBinding
		for _, b := range cfg.Bindings {
			if b.Method == method {
				bindings = append(bindings, b)
			}
		}
		if len(bindings) == 0 && !enabled[method] {
			continue
		}
		if err := c.reconcileAdminMount(method, adminAuths[method], bindings, cfg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Client) reconcileAdminMount(method string, auth adminAuth, bindings []AdminBinding, cfg AdminConfig) error {
	if len(bindings) > 0 {
		if err := c.enableAuthMount(method); err != nil {
			return err
		}
		if err := auth.configure(c, cfg); err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
	}
	bound, err := c.authRoleBindings(method, auth)
	if err != nil {
		return err
	}
	desired := map[string]bool{}
	for _, b := range bindings {
		desired[b.Name] = true
	}

	var errs []error
	for role := range bound {
		if strings.HasPrefix(role, AdminRolePrefix) && !desired[role] {
			if err := c.deleteMissingOK("auth/" + method + "/role/" + role); err != nil {
				errs = append(errs, err)
				continue
			}
			delete(bound, role)
			log.Printf("removed %s admin role %s", method, role)
		}
	}
	for _, b := range bindings {
		if other := boundElsewhere(bound, b.Name, b.Principal); other != "" {
			errs = append(errs, fmt.Errorf("%s: principal is already bound to %s role %s", b.Name, method, other))
			continue
		}
		policies, _ := AdminPolicies(b.Access)
		body := auth.roleBody(b, cfg)
		body["token_policies"] = policies
		body["token_ttl"] = AdminTokenTTL
		body["token_max_ttl"] = AdminTokenMaxTTL
		body["token_type"] = "service"
		if _, err := c.Must("POST", "auth/"+method+"/role/"+b.Name, body); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name, err))
			continue
		}
		bound[b.Name] = []string{b.Principal}
		log.Printf("wrote %s admin role %s (%s)", method, b.Name, strings.Join(policies, ", "))
	}
	return errors.Join(errs...)
}

func boundElsewhere(bound map[string][]string, role, principal string) string {
	for other, principals := range bound {
		if other == role {
			continue
		}
		for _, p := range principals {
			if p == principal {
				return other
			}
		}
	}
	return ""
}

func (c *Client) enabledAuthMounts() (map[string]bool, error) {
	r, err := c.Must("GET", "sys/auth", nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &wrap); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for path := range wrap.Data {
		out[strings.TrimSuffix(path, "/")] = true
	}
	return out, nil
}

func (c *Client) enableAuthMount(method string) error {
	r, err := c.Do("POST", "sys/auth/"+method, map[string]string{"type": method})
	if err != nil && r.Status == 0 {
		return err
	}
	if r.Status >= 400 && !strings.Contains(string(r.Body), "already in use") {
		return fmt.Errorf("enable %s auth failed (HTTP %d)", method, r.Status)
	}
	return nil
}

func (c *Client) authRoleBindings(method string, auth adminAuth) (map[string][]string, error) {
	out := map[string][]string{}
	r, err := c.Do("LIST", "auth/"+method+"/role", nil)
	if err != nil && r.Status == 0 {
		return nil, err
	}
	if r.Status == 404 {
		return out, nil
	}
	if r.Status >= 300 {
		return nil, fmt.Errorf("list %s auth roles failed (HTTP %d)", method, r.Status)
	}
	var listed struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &listed); err != nil {
		return nil, err
	}
	for _, key := range listed.Data.Keys {
		res, err := c.Must("GET", "auth/"+method+"/role/"+key, nil)
		if err != nil {
			return nil, err
		}
		var role struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(res.Body, &role); err != nil {
			return nil, err
		}
		principals, err := auth.boundPrincipals(role.Data, key)
		if err != nil {
			return nil, fmt.Errorf("%s role %s: %w", method, key, err)
		}
		out[key] = principals
	}
	return out, nil
}
