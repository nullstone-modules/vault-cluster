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

// AdminRolePrefix marks AWS auth roles owned by the cluster. Apps cannot write them.
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

var (
	adminRoleName     = regexp.MustCompile(`^admin-[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)
	adminPrincipalARN = regexp.MustCompile(`^arn:aws[a-z-]*:iam::[0-9]{12}:(user|role)/[A-Za-z0-9+=,.@_/-]+\*?$`)
)

type AdminBinding struct {
	Name         string   `json:"name"`
	PrincipalARN string   `json:"principal_arn"`
	Access       []string `json:"access"`
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

func ValidateAdminBindings(bindings []AdminBinding) error {
	names := map[string]bool{}
	principals := map[string]string{}
	for _, b := range bindings {
		if !adminRoleName.MatchString(b.Name) {
			return fmt.Errorf("invalid admin role name %q", b.Name)
		}
		if names[b.Name] {
			return fmt.Errorf("duplicate admin role %q", b.Name)
		}
		names[b.Name] = true
		if !adminPrincipalARN.MatchString(b.PrincipalARN) {
			return fmt.Errorf("%s: principal_arn must be an IAM user or role ARN", b.Name)
		}
		if other, ok := principals[b.PrincipalARN]; ok {
			return fmt.Errorf("%s: principal is already bound to %s", b.Name, other)
		}
		principals[b.PrincipalARN] = b.Name
		if _, err := AdminPolicies(b.Access); err != nil {
			return fmt.Errorf("%s: %w", b.Name, err)
		}
	}
	return nil
}

func LoadAdminBindings(path string) ([]AdminBinding, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bindings []AdminBinding
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return bindings, ValidateAdminBindings(bindings)
}

// ReconcileAdmins makes the admin-* AWS auth roles match bindings: writes each, deletes the rest.
// One principal maps to one role across the whole mount. A failed binding does not stop the others.
func (c *Client) ReconcileAdmins(bindings []AdminBinding) error {
	if err := ValidateAdminBindings(bindings); err != nil {
		return err
	}
	if err := c.enableAWSAuth(); err != nil {
		return err
	}
	bound, err := c.awsRoleBindings()
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
			if err := c.deleteMissingOK("auth/aws/role/" + role); err != nil {
				errs = append(errs, err)
				continue
			}
			delete(bound, role)
			log.Printf("removed admin role %s", role)
		}
	}
	for _, b := range bindings {
		if other := boundElsewhere(bound, b.Name, b.PrincipalARN); other != "" {
			errs = append(errs, fmt.Errorf("%s: principal is already bound to vault role %s", b.Name, other))
			continue
		}
		policies, _ := AdminPolicies(b.Access)
		if _, err := c.Must("POST", "auth/aws/role/"+b.Name, map[string]any{
			"auth_type":               "iam",
			"bound_iam_principal_arn": []string{b.PrincipalARN},
			"token_policies":          policies,
			"token_ttl":               AdminTokenTTL,
			"token_max_ttl":           AdminTokenMaxTTL,
			"token_type":              "service",
		}); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name, err))
			continue
		}
		bound[b.Name] = []string{b.PrincipalARN}
		log.Printf("wrote admin role %s (%s)", b.Name, strings.Join(policies, ", "))
	}
	return errors.Join(errs...)
}

func boundElsewhere(bound map[string][]string, role, principal string) string {
	for other, arns := range bound {
		if other == role {
			continue
		}
		for _, arn := range arns {
			if arn == principal {
				return other
			}
		}
	}
	return ""
}

func (c *Client) enableAWSAuth() error {
	r, err := c.Do("POST", "sys/auth/aws", map[string]string{"type": "aws"})
	if err != nil && r.Status == 0 {
		return err
	}
	if r.Status >= 400 && !strings.Contains(string(r.Body), "already in use") {
		return fmt.Errorf("enable aws auth failed (HTTP %d)", r.Status)
	}
	return nil
}

func (c *Client) awsRoleBindings() (map[string][]string, error) {
	out := map[string][]string{}
	r, err := c.Do("LIST", "auth/aws/role", nil)
	if err != nil && r.Status == 0 {
		return nil, err
	}
	if r.Status == 404 {
		return out, nil
	}
	if r.Status >= 300 {
		return nil, fmt.Errorf("list aws auth roles failed (HTTP %d)", r.Status)
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
		res, err := c.Must("GET", "auth/aws/role/"+key, nil)
		if err != nil {
			return nil, err
		}
		var role struct {
			Data struct {
				Bound []string `json:"bound_iam_principal_arn"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body, &role); err != nil {
			return nil, err
		}
		out[key] = role.Data.Bound
	}
	return out, nil
}
