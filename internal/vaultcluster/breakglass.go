package vaultcluster

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
)

// BreakGlass mints a root token from the operator token and the stored recovery (or unseal) shares,
// runs exactly one action with it, and revokes it. The generate-root attempt is cancelled and the root
// revoked on every exit path, including a panic in the action. Nothing here logs a token, a share, the
// OTP, or the encoded token.
type BreakGlass struct {
	Store  KeyStore
	Reason string
	Second string
	// Logf receives the audit trail (nonce, required, progress, revocation). Defaults to log.Printf.
	Logf func(format string, args ...any)
}

// Action is one recorded break-glass step. Name is what the log records; Run gets a client holding the root token.
type Action interface {
	Name() string
	Run(root *Client) error
}

func (bg BreakGlass) logf(format string, args ...any) {
	if bg.Logf != nil {
		bg.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// Validate enforces the two-person rule before any Vault call.
func (bg BreakGlass) Validate() error {
	if strings.TrimSpace(bg.Reason) == "" {
		return fmt.Errorf("--reason is required: record why before touching root")
	}
	if strings.TrimSpace(bg.Second) == "" {
		return fmt.Errorf("--second is required: name the second person")
	}
	if bg.Store == nil {
		return fmt.Errorf("no key store")
	}
	return nil
}

// TokenMeta is stamped on any token an action creates.
func (bg BreakGlass) TokenMeta() map[string]string {
	return map[string]string{
		"break_glass_reason": bg.Reason,
		"break_glass_second": bg.Second,
		"break_glass_at":     time.Now().UTC().Format(time.RFC3339),
	}
}

// Run performs the whole flow on c, which needs only an address. Any token on c is ignored.
func (bg BreakGlass) Run(c *Client, action Action) (err error) {
	if err := bg.Validate(); err != nil {
		return err
	}
	if action == nil {
		return fmt.Errorf("no action")
	}
	start := time.Now().UTC()
	bg.logf("break-glass start %s action=%s reason=%q second=%q", start.Format(time.RFC3339), action.Name(), bg.Reason, bg.Second)
	defer func() {
		bg.logf("break-glass end %s action=%s reason=%q second=%q ok=%t", time.Now().UTC().Format(time.RFC3339), action.Name(), bg.Reason, bg.Second, err == nil)
	}()

	operatorToken, err := bg.Store.LoadToken("operator")
	if err != nil {
		return fmt.Errorf("operator token: %w", err)
	}
	op := c.WithToken(operatorToken)

	initResp, err := bg.Store.LoadInit()
	if err != nil {
		return fmt.Errorf("init material: %w", err)
	}
	shares := initShares(initResp)
	if len(shares) == 0 {
		return fmt.Errorf("init material holds no recovery or unseal shares")
	}

	if err := cancelStaleAttempt(op, bg.logf); err != nil {
		return err
	}

	rootToken, err := generateRoot(op, shares, bg.logf)
	if err != nil {
		return err
	}
	root := c.WithToken(rootToken)
	defer func() {
		if rerr := root.API.Auth().Token().RevokeSelf(""); rerr != nil {
			bg.logf("root token revocation FAILED: %v", rerr)
			if err == nil {
				err = fmt.Errorf("root token revocation failed: %w", rerr)
			}
			return
		}
		bg.logf("revoked the root token")
	}()
	if _, err := root.API.Auth().Token().LookupSelf(); err != nil {
		return fmt.Errorf("root token does not work: %w", err)
	}

	bg.logf("running action %s", action.Name())
	if err := action.Run(root); err != nil {
		return fmt.Errorf("action %s: %w", action.Name(), err)
	}
	bg.logf("action %s done", action.Name())
	return nil
}

func cancelStaleAttempt(op *Client, logf func(string, ...any)) error {
	st, err := op.API.Sys().GenerateRootStatus()
	if err != nil {
		return fmt.Errorf("generate-root status: %w", err)
	}
	if !st.Started {
		return nil
	}
	logf("cancelling an in-progress generate-root attempt (nonce %s, progress %d/%d)", st.Nonce, st.Progress, st.Required)
	if err := op.API.Sys().GenerateRootCancel(); err != nil {
		return fmt.Errorf("generate-root cancel: %w", err)
	}
	return nil
}

// generateRoot starts an attempt, submits shares until it completes, and decodes the token. The attempt is
// cancelled unless it completed.
func generateRoot(op *Client, shares []string, logf func(string, ...any)) (token string, err error) {
	initSt, err := op.API.Sys().GenerateRootInit("", "")
	if err != nil {
		return "", fmt.Errorf("generate-root init: %w", err)
	}
	if initSt.OTP == "" {
		_ = op.API.Sys().GenerateRootCancel()
		return "", fmt.Errorf("generate-root init returned no OTP")
	}
	logf("generate-root started: nonce %s, required %d, shares on hand %d", initSt.Nonce, initSt.Required, len(shares))
	complete := false
	defer func() {
		if complete {
			return
		}
		if cerr := op.API.Sys().GenerateRootCancel(); cerr != nil {
			logf("generate-root cancel failed: %v", cerr)
		} else {
			logf("cancelled generate-root attempt %s", initSt.Nonce)
		}
	}()

	var st *api.GenerateRootStatusResponse
	for i, share := range selectShares(shares, initSt.Required) {
		st, err = op.API.Sys().GenerateRootUpdate(share, initSt.Nonce)
		if err != nil {
			return "", fmt.Errorf("generate-root share %d: %w", i+1, err)
		}
		logf("generate-root progress %d/%d", st.Progress, st.Required)
		if st.Complete {
			break
		}
	}
	if st == nil || !st.Complete {
		return "", fmt.Errorf("generate-root needs %d shares; the key store holds %d", initSt.Required, len(shares))
	}
	encoded := st.EncodedToken
	if encoded == "" {
		encoded = st.EncodedRootToken
	}
	token, err = DecodeRootToken(encoded, initSt.OTP)
	if err != nil {
		return "", err
	}
	complete = true
	return token, nil
}

// initShares returns the shares that satisfy generate-root: recovery shares on an auto-unsealed cluster,
// unseal shares on a Shamir one. Base64 is preferred; hex works too.
func initShares(resp *api.InitResponse) []string {
	for _, keys := range [][]string{resp.RecoveryKeysB64, resp.RecoveryKeys, resp.KeysB64, resp.Keys} {
		if len(keys) > 0 {
			return keys
		}
	}
	return nil
}

// selectShares returns the first required shares, or all of them when fewer are held (the attempt then fails
// with a clear count instead of a Vault error).
func selectShares(shares []string, required int) []string {
	if required <= 0 || required >= len(shares) {
		return shares
	}
	return shares[:required]
}

// DecodeRootToken reverses what Vault does to the token: it is XORed with the server-generated OTP and
// base64-encoded without padding. Same as `vault operator generate-root -decode -otp`.
func DecodeRootToken(encoded, otp string) (string, error) {
	if encoded == "" || otp == "" {
		return "", fmt.Errorf("decode root token: encoded token and OTP are required")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil {
		return "", fmt.Errorf("decode root token: %w", err)
	}
	if len(raw) != len(otp) {
		return "", fmt.Errorf("decode root token: OTP length %d does not match token length %d", len(otp), len(raw))
	}
	out := make([]byte, len(raw))
	for i := range raw {
		out[i] = raw[i] ^ otp[i]
	}
	return string(out), nil
}

// issueOrphanTokenWithMeta is issueOrphanToken with token metadata (the break-glass record).
func (c *Client) issueOrphanTokenWithMeta(policy string, meta map[string]string) (string, error) {
	sec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{
		Policies:    []string{policy},
		Period:      "24h",
		Renewable:   boolPtr(true),
		DisplayName: policy,
		NoParent:    true,
		Metadata:    meta,
	})
	if err != nil {
		return "", err
	}
	if sec == nil || sec.Auth == nil {
		return "", fmt.Errorf("failed to issue %s token", policy)
	}
	return sec.Auth.ClientToken, nil
}
