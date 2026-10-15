package vaultcluster

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/vault/api"
)

// breakGlassCluster is a configured Shamir cluster whose init material and operator token sit in a
// FileKeyStore, as `bootstrap local` leaves them. The returned client holds the initial root token.
func breakGlassCluster(t *testing.T) (*Client, FileKeyStore, *api.InitResponse) {
	t.Helper()
	c, init := startVaultInmemInit(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.EnvPrefix = "envs"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	store := FileKeyStore{Dir: t.TempDir()}
	init.RootToken = revokedRootMarker
	if err := store.SaveInit(init); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operator", "provisioning"} {
		tok, err := c.issueOrphanToken(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveToken(name, tok); err != nil {
			t.Fatal(err)
		}
	}
	return c, store, init
}

// rootAccessors counts live tokens holding the root policy. The test's own initial root is one.
func rootAccessors(t *testing.T, c *Client) int {
	t.Helper()
	sec, err := c.API.Logical().List("auth/token/accessors")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	if sec != nil {
		for _, k := range sec.Data["keys"].([]any) {
			info, err := c.API.Auth().Token().LookupAccessor(k.(string))
			if err != nil {
				continue
			}
			for _, p := range info.Data["policies"].([]any) {
				if p == "root" {
					n++
				}
			}
		}
	}
	return n
}

func noAttemptInProgress(t *testing.T, c *Client) {
	t.Helper()
	st, err := c.API.Sys().GenerateRootStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Started {
		t.Fatalf("generate-root attempt left in progress: nonce %s", st.Nonce)
	}
}

type logBuf struct{ lines []string }

func (l *logBuf) logf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}
func (l *logBuf) text() string { return strings.Join(l.lines, "\n") }

func mustNotContain(t *testing.T, text string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(text, s) {
			t.Fatalf("log leaks a secret: %q", s)
		}
	}
}

func TestBreakGlassRemintProvisioning(t *testing.T) {
	c, store, init := breakGlassCluster(t)
	rootsBefore := rootAccessors(t, c)
	oldTok, _ := store.LoadToken("provisioning")
	opTok, _ := store.LoadToken("operator")

	logs := &logBuf{}
	bg := BreakGlass{Store: store, Reason: "provisioning expired", Second: "alex", Logf: logs.logf}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	if err := bg.Run(anon, RemintAction{Token: "provisioning", BG: bg}); err != nil {
		t.Fatalf("%v\n%s", err, logs.text())
	}

	newTok, err := store.LoadToken("provisioning")
	if err != nil || newTok == oldTok || newTok == "" {
		t.Fatalf("provisioning token not replaced: %q %v", newTok, err)
	}
	sec, err := c.WithToken(newTok).API.Auth().Token().LookupSelf()
	if err != nil {
		t.Fatalf("new provisioning token does not work: %v", err)
	}
	if sec.Data["display_name"] != "token-provisioning" || fmt.Sprint(sec.Data["period"]) != "86400" {
		t.Fatalf("token parameters differ from bootstrap: display_name=%v period=%v", sec.Data["display_name"], sec.Data["period"])
	}
	meta, _ := sec.Data["meta"].(map[string]any)
	if meta["break_glass_reason"] != "provisioning expired" || meta["break_glass_second"] != "alex" {
		t.Fatalf("token metadata lacks the record: %v", meta)
	}
	if _, err := c.WithToken(oldTok).API.Auth().Token().LookupSelf(); err != nil {
		t.Fatalf("remint must not revoke the old token (it may still be in use): %v", err)
	}
	if got := rootAccessors(t, c); got != rootsBefore {
		t.Fatalf("root accessors %d, want %d: a root token is still alive", got, rootsBefore)
	}
	noAttemptInProgress(t, c)

	text := logs.text()
	mustNotContain(t, text, opTok, newTok, oldTok, init.KeysB64[0], c.Cfg.Token)
	for _, want := range []string{"nonce", "required 1", "progress 1/1", "revoked the root token", `reason="provisioning expired"`, `second="alex"`, "break-glass start", "break-glass end"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log lacks %q:\n%s", want, text)
		}
	}
}

func TestBreakGlassPolicyWriteRoundTrip(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	bg := BreakGlass{Store: store, Reason: "first admin policy", Second: "alex", Logf: func(string, ...any) {}}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	hcl := "path \"sys/health\" {\n  capabilities = [\"read\"]\n}\n\npath \"kv/data/customers/*\" {\n  capabilities = [\"read\", \"list\"]\n}\n"
	act := &PolicyWriteAction{Policy: "admin", HCL: hcl}
	if err := bg.Run(anon, act); err != nil {
		t.Fatal(err)
	}
	if act.Paths != 2 {
		t.Fatalf("paths read back = %d", act.Paths)
	}
	stored, err := c.API.Sys().GetPolicy("admin")
	if err != nil || strings.TrimSpace(stored) != strings.TrimSpace(hcl) {
		t.Fatalf("stored policy differs: %q %v", stored, err)
	}
	if err := bg.Run(anon, &PolicyWriteAction{Policy: "operator", HCL: hcl}); err == nil {
		t.Fatal("platform policy written")
	}
	noAttemptInProgress(t, c)
}

func TestBreakGlassUserpass(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	rootsBefore := rootAccessors(t, c)
	bg := BreakGlass{Store: store, Reason: "first admin", Second: "alex", Logf: func(string, ...any) {}}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})

	// No admin policy yet: the action fails clearly and the cluster is left clean.
	err := bg.Run(anon, UserpassAction{FirstAdmin: "alex", ReadPassword: func() (string, error) { return "hunter2\n", nil }})
	if err == nil || !strings.Contains(err.Error(), `policy "admin" does not exist`) {
		t.Fatalf("missing admin policy: %v", err)
	}
	noAttemptInProgress(t, c)

	if err := c.API.Sys().PutPolicy("admin", `path "sys/health" { capabilities = ["read"] }`); err != nil {
		t.Fatal(err)
	}

	// Piped password: the terminal reader refuses before any user is written.
	err = bg.Run(anon, UserpassAction{FirstAdmin: "alex", ReadPassword: pipedPasswordReader(t, "hunter2\n")})
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("piped password: %v", err)
	}
	if sec, _ := c.API.Logical().Read("auth/userpass/users/alex"); sec != nil {
		t.Fatal("user created from a piped password")
	}

	// Terminal password (a pty when the platform has one, else the reader is injected).
	err = bg.Run(anon, UserpassAction{FirstAdmin: "alex", ReadPassword: terminalPasswordReader(t, "hunter2")})
	if err != nil {
		t.Fatal(err)
	}
	login, err := c.API.Logical().Write("auth/userpass/login/alex", map[string]any{"password": "hunter2"})
	if err != nil || login == nil || login.Auth == nil {
		t.Fatalf("login as alex: %v", err)
	}
	if strings.Join(login.Auth.Policies, ",") != "admin,default" || login.Auth.LeaseDuration != 8*3600 {
		t.Fatalf("admin token: policies=%v ttl=%d", login.Auth.Policies, login.Auth.LeaseDuration)
	}
	if _, err := c.API.Logical().Write("auth/userpass/login/alex", map[string]any{"password": "hunter2\n"}); err == nil {
		t.Fatal("password stored with a trailing newline")
	}

	// Enable is idempotent and works without an admin.
	if err := bg.Run(anon, UserpassAction{}); err != nil {
		t.Fatal(err)
	}
	if got := rootAccessors(t, c); got != rootsBefore {
		t.Fatalf("root accessors %d, want %d", got, rootsBefore)
	}
	noAttemptInProgress(t, c)
}

type failingAction struct{ ran *bool }

func (failingAction) Name() string { return "fail" }
func (a failingAction) Run(root *Client) error {
	*a.ran = true
	if _, err := root.API.Sys().GetPolicy("operator"); err != nil {
		return fmt.Errorf("root did not work: %w", err)
	}
	return errors.New("boom")
}

func TestBreakGlassFailingActionRevokesRoot(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	rootsBefore := rootAccessors(t, c)
	logs := &logBuf{}
	bg := BreakGlass{Store: store, Reason: "drill", Second: "alex", Logf: logs.logf}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	ran := false
	err := bg.Run(anon, failingAction{ran: &ran})
	if err == nil || !strings.Contains(err.Error(), "boom") || !ran {
		t.Fatalf("err = %v ran = %t", err, ran)
	}
	if got := rootAccessors(t, c); got != rootsBefore {
		t.Fatalf("root accessors %d, want %d: the root survived a failed action", got, rootsBefore)
	}
	noAttemptInProgress(t, c)
	if !strings.Contains(logs.text(), "revoked the root token") || !strings.Contains(logs.text(), "ok=false") {
		t.Fatalf("log:\n%s", logs.text())
	}
}

func TestBreakGlassCancelsStaleAttempt(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	opTok, _ := store.LoadToken("operator")
	stale, err := c.WithToken(opTok).API.Sys().GenerateRootInit("", "")
	if err != nil {
		t.Fatal(err)
	}
	logs := &logBuf{}
	bg := BreakGlass{Store: store, Reason: "drill", Second: "alex", Logf: logs.logf}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	if err := bg.Run(anon, UserpassAction{}); err != nil {
		t.Fatalf("%v\n%s", err, logs.text())
	}
	if !strings.Contains(logs.text(), "cancelling an in-progress generate-root attempt (nonce "+stale.Nonce) {
		t.Fatalf("stale attempt not reported:\n%s", logs.text())
	}
	mustNotContain(t, logs.text(), stale.OTP, opTok)
	noAttemptInProgress(t, c)
}

func TestBreakGlassExecPassesRootInEnvOnly(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	rootsBefore := rootAccessors(t, c)
	bg := BreakGlass{Store: store, Reason: "drill", Second: "alex", Logf: func(string, ...any) {}}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	out := &strings.Builder{}
	act := ExecAction{Argv: []string{os.Args[0], "-test.run=TestBreakGlassExecHelper"}, Stdout: out, Stderr: os.Stderr, Env: []string{"GO_BREAK_GLASS_HELPER=1", "VAULT_TOKEN=stale"}}
	if err := bg.Run(anon, act); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if !strings.HasPrefix(got, "root ") || strings.Contains(got, "stale") {
		t.Fatalf("child saw %q", got)
	}
	if got := rootAccessors(t, c); got != rootsBefore {
		t.Fatalf("root accessors %d, want %d", got, rootsBefore)
	}
}

// TestBreakGlassExecHelper is the child of TestBreakGlassExecPassesRootInEnvOnly: it proves the token it got
// is root by reading a policy, and prints only "root <policies>".
func TestBreakGlassExecHelper(t *testing.T) {
	if os.Getenv("GO_BREAK_GLASS_HELPER") != "1" {
		t.Skip("helper process")
	}
	c, err := New(Config{Addr: os.Getenv("VAULT_ADDR"), Token: os.Getenv("VAULT_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	sec, err := c.API.Auth().Token().LookupSelf()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("root %v\n", sec.Data["policies"])
}
