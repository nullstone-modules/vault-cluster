package vaultcluster

import (
	"fmt"
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
			if IsRootToken(info) {
				n++
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

func TestBreakGlassBeginEnd(t *testing.T) {
	c, store, init := breakGlassCluster(t)
	rootsBefore := rootAccessors(t, c)
	opTok, _ := store.LoadToken("operator")

	logs := &logBuf{}
	bg := BreakGlass{Store: store, Reason: "provisioning expired", Logf: logs.logf}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	token, err := bg.Begin(anon)
	if err != nil {
		t.Fatalf("%v\n%s", err, logs.text())
	}
	if !strings.HasPrefix(token, "hvs.") {
		t.Fatalf("unexpected token shape (len %d)", len(token))
	}
	sec, err := c.WithToken(token).API.Auth().Token().LookupSelf()
	if err != nil || !IsRootToken(sec) {
		t.Fatalf("minted token is not a working root: %v %v", sec, err)
	}
	if got := rootAccessors(t, c); got != rootsBefore+1 {
		t.Fatalf("root accessors %d, want %d", got, rootsBefore+1)
	}
	noAttemptInProgress(t, c)

	text := logs.text()
	mustNotContain(t, text, opTok, token, init.KeysB64[0], c.Cfg.Token)
	for _, want := range []string{"nonce", "required 1", "progress 1/1", `reason="provisioning expired"`, "break-glass begin", "root token ready"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log lacks %q:\n%s", want, text)
		}
	}

	// End refuses the operator token and revokes the root.
	if err := End(anon, opTok, logs.logf); err == nil || !strings.Contains(err.Error(), "not a root token") {
		t.Fatalf("End revoked a non-root token: %v", err)
	}
	if _, err := c.WithToken(opTok).API.Auth().Token().LookupSelf(); err != nil {
		t.Fatalf("operator token was revoked: %v", err)
	}
	if err := End(anon, token, logs.logf); err != nil {
		t.Fatal(err)
	}
	if _, err := c.WithToken(token).API.Auth().Token().LookupSelf(); err == nil {
		t.Fatal("root token still works after End")
	}
	if got := rootAccessors(t, c); got != rootsBefore {
		t.Fatalf("root accessors %d, want %d", got, rootsBefore)
	}
	if err := End(anon, token, logs.logf); err == nil {
		t.Fatal("End of a revoked token succeeded")
	}
	mustNotContain(t, logs.text(), opTok, token)
}

func TestBreakGlassCancelsStaleAttempt(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	opTok, _ := store.LoadToken("operator")
	stale, err := c.WithToken(opTok).API.Sys().GenerateRootInit("", "")
	if err != nil {
		t.Fatal(err)
	}
	logs := &logBuf{}
	bg := BreakGlass{Store: store, Reason: "drill", Logf: logs.logf}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	token, err := bg.Begin(anon)
	if err != nil {
		t.Fatalf("%v\n%s", err, logs.text())
	}
	defer End(anon, token, logs.logf)
	if !strings.Contains(logs.text(), "cancelling an in-progress generate-root attempt (nonce "+stale.Nonce) {
		t.Fatalf("stale attempt not reported:\n%s", logs.text())
	}
	mustNotContain(t, logs.text(), stale.OTP, opTok, token)
	noAttemptInProgress(t, c)
}

func TestBreakGlassBeginFailsCleanly(t *testing.T) {
	c, store, _ := breakGlassCluster(t)
	rootsBefore := rootAccessors(t, c)
	// Break the shares: generate-root rejects them and the attempt must be cancelled.
	if err := store.SaveInit(&api.InitResponse{KeysB64: []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}); err != nil {
		t.Fatal(err)
	}
	logs := &logBuf{}
	bg := BreakGlass{Store: store, Reason: "drill", Logf: logs.logf}
	anon, _ := New(Config{Addr: c.Cfg.Addr, HTTPTimeout: c.Cfg.HTTPTimeout})
	if _, err := bg.Begin(anon); err == nil {
		t.Fatal("bad share accepted")
	}
	if !strings.Contains(logs.text(), "cancelled generate-root attempt") {
		t.Fatalf("attempt not cancelled:\n%s", logs.text())
	}
	noAttemptInProgress(t, c)
	if got := rootAccessors(t, c); got != rootsBefore {
		t.Fatalf("root accessors %d, want %d", got, rootsBefore)
	}
}
