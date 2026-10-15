package vaultcluster

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/hashicorp/vault/api"
)

// PlatformPolicies are the policies bootstrap writes and owns. Break-glass refuses to overwrite them: they
// are rendered from templates in this repo and a hand edit would be lost at the next bootstrap.
var PlatformPolicies = []string{"provisioning", "operator", "apps-auth", "apps-reader", "apps-writer", "tenant-reader", "tenant-writer", "tenant-database"}

// ReservedPolicyName reports whether break-glass refuses to write name.
func ReservedPolicyName(name string) bool {
	switch name {
	case "root", "default":
		return true
	}
	for _, p := range PlatformPolicies {
		if name == p {
			return true
		}
	}
	return false
}

// RemintAction re-issues one platform token with the parameters bootstrap uses and stores it.
// The operator token is refused: the nodes hold it and rotating it needs a node restart.
type RemintAction struct {
	Token string
	BG    BreakGlass
}

func (a RemintAction) Name() string { return "remint " + a.Token }

// Check validates the token name without a cluster.
func (a RemintAction) Check() error {
	switch a.Token {
	case "provisioning", "apps-auth":
		return nil
	case "operator":
		return fmt.Errorf("remint operator is refused: the nodes hold that token; rebuild the cluster nodes instead")
	default:
		return fmt.Errorf("unknown token %q (provisioning, apps-auth)", a.Token)
	}
}

func (a RemintAction) Run(root *Client) error {
	if err := a.Check(); err != nil {
		return err
	}
	tok, err := root.issueOrphanTokenWithMeta(a.Token, a.BG.TokenMeta())
	if err != nil {
		return err
	}
	if err := a.BG.Store.SaveToken(a.Token, tok); err != nil {
		return fmt.Errorf("the new %s token was issued but not saved; it is orphaned and expires in 24h unless renewed: %w", a.Token, err)
	}
	sec, err := root.WithToken(tok).API.Auth().Token().LookupSelf()
	if err != nil {
		return fmt.Errorf("the new %s token was saved but lookup-self failed: %w", a.Token, err)
	}
	a.BG.logf("%s token re-minted and saved: display_name=%v policies=%v period=%v", a.Token, sec.Data["display_name"], sec.Data["policies"], sec.Data["period"])
	return nil
}

// PolicyWriteAction writes one ACL policy and reports the path count read back.
type PolicyWriteAction struct {
	Policy string
	HCL    string
	Logf   func(string, ...any)
	// Paths is set after Run: the number of path stanzas Vault stored.
	Paths int
}

func (a *PolicyWriteAction) Name() string { return "policy write " + a.Policy }

func (a *PolicyWriteAction) Check() error {
	if a.Policy == "" {
		return fmt.Errorf("policy name is required")
	}
	if ReservedPolicyName(a.Policy) {
		return fmt.Errorf("policy %q is owned by bootstrap and cannot be written by break-glass", a.Policy)
	}
	if strings.TrimSpace(a.HCL) == "" {
		return fmt.Errorf("policy %q is empty", a.Policy)
	}
	return nil
}

var policyPathStanza = regexp.MustCompile(`(?m)^\s*path\s+"`)

func (a *PolicyWriteAction) Run(root *Client) error {
	if err := a.Check(); err != nil {
		return err
	}
	if err := root.API.Sys().PutPolicy(a.Policy, a.HCL); err != nil {
		return err
	}
	stored, err := root.API.Sys().GetPolicy(a.Policy)
	if err != nil {
		return fmt.Errorf("policy %s written but not readable: %w", a.Policy, err)
	}
	a.Paths = len(policyPathStanza.FindAllStringIndex(stored, -1))
	if a.Logf != nil {
		a.Logf("policy %s written: %d path stanzas", a.Policy, a.Paths)
	}
	return nil
}

// UserpassAction enables the userpass auth method (idempotent) and optionally creates the first admin.
// The password comes from ReadPassword, which must not echo and must refuse a pipe.
type UserpassAction struct {
	Mount        string
	FirstAdmin   string
	AdminPolicy  string
	ReadPassword func() (string, error)
	Logf         func(string, ...any)
}

func (a UserpassAction) Name() string {
	if a.FirstAdmin != "" {
		return "userpass enable --first-admin " + a.FirstAdmin
	}
	return "userpass enable"
}

func (a UserpassAction) mount() string {
	if a.Mount == "" {
		return "userpass"
	}
	return a.Mount
}

func (a UserpassAction) adminPolicy() string {
	if a.AdminPolicy == "" {
		return "admin"
	}
	return a.AdminPolicy
}

func (a UserpassAction) Run(root *Client) error {
	if a.FirstAdmin != "" && a.ReadPassword == nil {
		return fmt.Errorf("no password reader")
	}
	auths, err := root.API.Sys().ListAuth()
	if err != nil {
		return err
	}
	if _, ok := auths[a.mount()+"/"]; ok {
		a.logf("auth method %s already enabled", a.mount())
	} else {
		if err := root.API.Sys().EnableAuthWithOptions(a.mount(), &api.EnableAuthOptions{Type: "userpass", Description: "Human admins"}); err != nil {
			return err
		}
		a.logf("enabled auth method %s", a.mount())
	}
	if a.FirstAdmin == "" {
		return nil
	}
	pol, err := root.API.Sys().GetPolicy(a.adminPolicy())
	if err != nil {
		return fmt.Errorf("read policy %s: %w", a.adminPolicy(), err)
	}
	if strings.TrimSpace(pol) == "" {
		return fmt.Errorf("policy %q does not exist; write it first with: break-glass ... policy write %s <file>", a.adminPolicy(), a.adminPolicy())
	}
	password, err := a.ReadPassword()
	if err != nil {
		return err
	}
	password = TrimPassword(password)
	if password == "" {
		return fmt.Errorf("empty password")
	}
	_, err = root.API.Logical().Write(fmt.Sprintf("auth/%s/users/%s", a.mount(), a.FirstAdmin), map[string]any{
		"password":       password,
		"token_policies": a.adminPolicy(),
		"token_ttl":      "8h",
		"token_max_ttl":  "24h",
	})
	if err != nil {
		return err
	}
	a.logf("user %s created on %s with token_policies=%s token_ttl=8h token_max_ttl=24h", a.FirstAdmin, a.mount(), a.adminPolicy())
	return nil
}

func (a UserpassAction) logf(format string, args ...any) {
	if a.Logf != nil {
		a.Logf(format, args...)
	}
}

// TrimPassword drops one trailing line ending, which a terminal read includes and a shell here-string adds.
func TrimPassword(p string) string {
	p = strings.TrimSuffix(p, "\n")
	p = strings.TrimSuffix(p, "\r")
	return p
}

// ExecAction runs a child process with VAULT_ADDR, VAULT_TOKEN (root), and VAULT_TLS_SERVER_NAME in its
// environment. The token reaches the child only through that environment, which anything the child spawns
// inherits; it dies when break-glass revokes root after the child exits. The audit log is the record.
type ExecAction struct {
	Argv   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string
}

func (a ExecAction) Name() string { return "exec " + strings.Join(a.Argv, " ") }

func (a ExecAction) Run(root *Client) error {
	if len(a.Argv) == 0 {
		return fmt.Errorf("exec needs a command: break-glass ... exec -- <command> [args]")
	}
	cmd := exec.Command(a.Argv[0], a.Argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.Stdin, a.Stdout, a.Stderr
	base := a.Env
	if base == nil {
		base = os.Environ()
	}
	cmd.Env = append(withoutVaultVars(base),
		"VAULT_ADDR="+root.Cfg.Addr,
		"VAULT_TOKEN="+root.Cfg.Token,
		"VAULT_TLS_SERVER_NAME="+root.Cfg.TLSServerName,
	)
	return cmd.Run()
}

func withoutVaultVars(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "VAULT_ADDR="), strings.HasPrefix(kv, "VAULT_TOKEN="), strings.HasPrefix(kv, "VAULT_TLS_SERVER_NAME="):
			continue
		}
		out = append(out, kv)
	}
	return out
}
