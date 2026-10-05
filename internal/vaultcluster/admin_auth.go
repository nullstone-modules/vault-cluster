package vaultcluster

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// adminAuth is one Vault auth method that humans log in with. The mount path is the method name.
type adminAuth interface {
	validatePrincipal(principal string) error
	roleBody(b AdminBinding, cfg AdminConfig) map[string]any
	boundPrincipals(roleData json.RawMessage, role string) ([]string, error)
	configure(c *Client, cfg AdminConfig) error
}

var adminAuths = map[string]adminAuth{
	"aws":  awsAdminAuth{},
	"gcp":  gcpAdminAuth{},
	"oidc": oidcAdminAuth{},
}

func AdminMethods() []string {
	out := make([]string, 0, len(adminAuths))
	for m := range adminAuths {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// aws: IAM auth. A trailing * matches a prefix; Vault then resolves the caller's full ARN with iam:GetRole/GetUser.
type awsAdminAuth struct{}

var awsPrincipal = regexp.MustCompile(`^arn:aws[a-z-]*:iam::[0-9]{12}:(user|role)/[A-Za-z0-9+=,.@_/-]+\*?$`)

func (awsAdminAuth) validatePrincipal(p string) error {
	if !awsPrincipal.MatchString(p) {
		return fmt.Errorf("aws principal must be an IAM user or role ARN")
	}
	return nil
}

func (awsAdminAuth) roleBody(b AdminBinding, _ AdminConfig) map[string]any {
	return map[string]any{
		"auth_type":               "iam",
		"bound_iam_principal_arn": []string{b.Principal},
	}
}

func (awsAdminAuth) boundPrincipals(raw json.RawMessage, _ string) ([]string, error) {
	var role struct {
		Bound []string `json:"bound_iam_principal_arn"`
	}
	err := json.Unmarshal(raw, &role)
	return role.Bound, err
}

func (awsAdminAuth) configure(*Client, AdminConfig) error { return nil }

// gcp: IAM auth. Humans sign the login JWT by impersonating the bound service account
// (roles/iam.serviceAccountTokenCreator), so Vault's audit log names the service account, not the person.
type gcpAdminAuth struct{}

var gcpPrincipal = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]@[a-z0-9.-]+\.iam\.gserviceaccount\.com$`)

func (gcpAdminAuth) validatePrincipal(p string) error {
	if !gcpPrincipal.MatchString(p) {
		return fmt.Errorf("gcp principal must be a service account email")
	}
	return nil
}

func (gcpAdminAuth) roleBody(b AdminBinding, _ AdminConfig) map[string]any {
	return map[string]any{
		"type":                   "iam",
		"bound_service_accounts": []string{b.Principal},
	}
}

func (gcpAdminAuth) boundPrincipals(raw json.RawMessage, _ string) ([]string, error) {
	var role struct {
		Bound []string `json:"bound_service_accounts"`
	}
	err := json.Unmarshal(raw, &role)
	return role.Bound, err
}

func (gcpAdminAuth) configure(*Client, AdminConfig) error { return nil }

// oidc: any OIDC IdP (Entra ID, Google Workspace, Okta). The principal is an IdP group, matched on GroupsClaim.
// The audit log names the person through UserClaim.
type oidcAdminAuth struct{}

// The Vault CLI's `vault login -method=oidc` listener.
const oidcCLIRedirect = "http://localhost:8250/oidc/callback"

type OIDCConfig struct {
	DiscoveryURL string `json:"discovery_url"`
	ClientID     string `json:"client_id"`
	// ClientSecretFile is a path on the node. Empty means a public client.
	ClientSecretFile string   `json:"client_secret_file,omitempty"`
	UserClaim        string   `json:"user_claim,omitempty"`
	GroupsClaim      string   `json:"groups_claim,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	RedirectURIs     []string `json:"redirect_uris,omitempty"`
}

func (o OIDCConfig) userClaim() string {
	if o.UserClaim == "" {
		return "email"
	}
	return o.UserClaim
}

func (o OIDCConfig) groupsClaim() string {
	if o.GroupsClaim == "" {
		return "groups"
	}
	return o.GroupsClaim
}

func (o OIDCConfig) validate() error {
	if !strings.HasPrefix(o.DiscoveryURL, "https://") {
		return fmt.Errorf("oidc discovery_url must be https")
	}
	if o.ClientID == "" {
		return fmt.Errorf("oidc client_id is required")
	}
	return nil
}

func (oidcAdminAuth) validatePrincipal(p string) error {
	if p == "" || len(p) > 256 || strings.ContainsAny(p, "*\n") {
		return fmt.Errorf("oidc principal must be one IdP group name or ID, without wildcards")
	}
	return nil
}

func (oidcAdminAuth) roleBody(b AdminBinding, cfg AdminConfig) map[string]any {
	o := *cfg.OIDC
	body := map[string]any{
		"role_type":             "oidc",
		"user_claim":            o.userClaim(),
		"groups_claim":          o.groupsClaim(),
		"bound_claims":          map[string]any{o.groupsClaim(): []string{b.Principal}},
		"allowed_redirect_uris": append([]string{oidcCLIRedirect}, o.RedirectURIs...),
	}
	if len(o.Scopes) > 0 {
		body["oidc_scopes"] = o.Scopes
	}
	return body
}

func (oidcAdminAuth) boundPrincipals(raw json.RawMessage, _ string) ([]string, error) {
	var role struct {
		Bound map[string]any `json:"bound_claims"`
	}
	if err := json.Unmarshal(raw, &role); err != nil {
		return nil, err
	}
	var out []string
	for _, v := range role.Bound {
		switch vv := v.(type) {
		case string:
			out = append(out, vv)
		case []any:
			for _, s := range vv {
				if str, ok := s.(string); ok {
					out = append(out, str)
				}
			}
		}
	}
	return out, nil
}

func (oidcAdminAuth) configure(c *Client, cfg AdminConfig) error {
	o := cfg.OIDC
	body := map[string]any{
		"oidc_discovery_url": o.DiscoveryURL,
		"oidc_client_id":     o.ClientID,
	}
	if o.ClientSecretFile != "" {
		raw, err := os.ReadFile(o.ClientSecretFile)
		if err != nil {
			return fmt.Errorf("oidc client secret: %w", err)
		}
		body["oidc_client_secret"] = strings.TrimSpace(string(raw))
	}
	_, err := c.Must("POST", "auth/oidc/config", body)
	return err
}
