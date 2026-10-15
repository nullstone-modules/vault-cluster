package vaultcluster

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hashicorp/vault/api"
)

func TestBreakGlassValidate(t *testing.T) {
	store := FileKeyStore{Dir: t.TempDir()}
	for name, bg := range map[string]BreakGlass{
		"no reason": {Second: "s", Store: store},
		"no second": {Reason: "r", Store: store},
		"blank":     {Reason: " ", Second: "\t", Store: store},
		"no store":  {Reason: "r", Second: "s"},
	} {
		if err := bg.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := (BreakGlass{Reason: "r", Second: "s", Store: store}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (BreakGlass{Store: store}).Run(&Client{}, nil); err == nil {
		t.Fatal("Run without reason/second touched the cluster")
	}
}

func TestSelectShares(t *testing.T) {
	shares := []string{"a", "b", "c", "d", "e"}
	if got := selectShares(shares, 3); strings.Join(got, "") != "abc" {
		t.Fatalf("required < held: %v", got)
	}
	if got := selectShares(shares, 5); len(got) != 5 {
		t.Fatalf("required == held: %v", got)
	}
	if got := selectShares(shares, 7); len(got) != 5 {
		t.Fatalf("required > held: %v", got)
	}
	if got := selectShares(shares, 0); len(got) != 5 {
		t.Fatalf("required unknown: %v", got)
	}
}

func TestInitSharesPrefersRecovery(t *testing.T) {
	if got := initShares(&api.InitResponse{RecoveryKeysB64: []string{"r"}, KeysB64: []string{"k"}}); len(got) != 1 || got[0] != "r" {
		t.Fatalf("auto-unseal cluster: %v", got)
	}
	if got := initShares(&api.InitResponse{KeysB64: []string{"k1", "k2"}, Keys: []string{"h1", "h2"}}); len(got) != 2 || got[0] != "k1" {
		t.Fatalf("shamir cluster: %v", got)
	}
	if got := initShares(&api.InitResponse{Keys: []string{"h1"}}); len(got) != 1 || got[0] != "h1" {
		t.Fatalf("hex only: %v", got)
	}
	if got := initShares(&api.InitResponse{RootToken: "x"}); got != nil {
		t.Fatalf("no shares: %v", got)
	}
}

func TestDecodeRootToken(t *testing.T) {
	token := "hvs.CAESIJ1234567890abcdefghij"
	otp := strings.Repeat("ZsDZoHfm6LcAM7mFsSV9mtPoC5", 2)[:len(token)]
	x := make([]byte, len(token))
	for i := range x {
		x[i] = token[i] ^ otp[i]
	}
	encoded := base64.RawStdEncoding.EncodeToString(x)
	got, err := DecodeRootToken(encoded, otp)
	if err != nil || got != token {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := DecodeRootToken(encoded+"==", otp); err != nil || got != token {
		t.Fatalf("padded: %q, %v", got, err)
	}
	if _, err := DecodeRootToken(encoded, otp[:5]); err == nil {
		t.Fatal("short OTP accepted")
	}
	if _, err := DecodeRootToken("", otp); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, err := DecodeRootToken("!!!", otp); err == nil {
		t.Fatal("bad base64 accepted")
	}
}

func TestTrimPassword(t *testing.T) {
	for in, want := range map[string]string{
		"hunter2\n":   "hunter2",
		"hunter2\r\n": "hunter2",
		"hunter2":     "hunter2",
		"hunter2\n\n": "hunter2\n",
		" hunter2 ":   " hunter2 ",
	} {
		if got := TrimPassword(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestReservedPolicyName(t *testing.T) {
	for _, n := range append([]string{"root", "default"}, PlatformPolicies...) {
		if !ReservedPolicyName(n) {
			t.Errorf("%s not reserved", n)
		}
	}
	for _, n := range []string{"admin", "readonly", "operator2"} {
		if ReservedPolicyName(n) {
			t.Errorf("%s reserved", n)
		}
	}
	if err := (&PolicyWriteAction{Policy: "operator", HCL: "x"}).Check(); err == nil {
		t.Fatal("platform policy accepted")
	}
	if err := (&PolicyWriteAction{Policy: "admin", HCL: " "}).Check(); err == nil {
		t.Fatal("empty policy accepted")
	}
}

func TestRemintActionCheck(t *testing.T) {
	if err := (RemintAction{Token: "operator"}).Check(); err == nil {
		t.Fatal("operator accepted")
	}
	if err := (RemintAction{Token: "root"}).Check(); err == nil {
		t.Fatal("root accepted")
	}
	for _, n := range []string{"provisioning", "apps-auth"} {
		if err := (RemintAction{Token: n}).Check(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecActionEnv(t *testing.T) {
	env := withoutVaultVars([]string{"HOME=/h", "VAULT_TOKEN=leak", "VAULT_ADDR=http://old", "VAULT_TLS_SERVER_NAME=old", "PATH=/bin"})
	if strings.Join(env, " ") != "HOME=/h PATH=/bin" {
		t.Fatalf("env = %v", env)
	}
	if err := (ExecAction{}).Run(&Client{}); err == nil {
		t.Fatal("exec without a command accepted")
	}
}
