package vaultcluster

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
)

const vaultTestImage = "hashicorp/vault:1.21.4@sha256:4e33b126a59c0c333b76fb4e894722462659a6bec7c48c9ee8cea56fccfd2569"

func TestIsolationMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping docker isolation")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon is not running")
	}

	cfgJSON := `{"disable_mlock":true,"listener":{"tcp":{"address":"0.0.0.0:8200","tls_disable":true}},"storage":{"inmem":{}}}`
	run := exec.Command("docker", "run", "-d", "-p", "127.0.0.1::8200",
		"-e", "VAULT_LOCAL_CONFIG="+cfgJSON,
		"--cap-add", "IPC_LOCK",
		vaultTestImage, "server")
	out, err := run.Output()
	if err != nil {
		stderr := []byte(nil)
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("docker run: %v\n%s\n%s", err, out, stderr)
	}
	id := lastLine(string(out))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })

	var portOut []byte
	var portErr error
	for i := 0; i < 20; i++ {
		portOut, portErr = exec.Command("docker", "port", id, "8200").CombinedOutput()
		if portErr == nil && strings.Contains(string(portOut), ":") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if portErr != nil || !strings.Contains(string(portOut), ":") {
		logs, _ := exec.Command("docker", "logs", id).CombinedOutput()
		t.Fatalf("docker port %s: %v\n%s\nlogs:\n%s", id, portErr, portOut, logs)
	}
	line := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	hostPort := line
	if i := strings.LastIndex(line, ":"); i >= 0 {
		hostPort = line[i+1:]
	}
	addr := "http://127.0.0.1:" + hostPort

	cfg := Config{
		Addr:          addr,
		KVMount:       "kv",
		TenantPrefix:  "customers",
		AuthMount:     "approle",
		DatabaseMount: "database",
		EnableAudit:   false,
		HTTPTimeout:   10 * time.Second,
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := c.API.Sys().SealStatus(); err == nil && st != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	resp, err := c.API.Sys().Init(&api.InitRequest{SecretShares: 1, SecretThreshold: 1})
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	if len(resp.KeysB64) > 0 {
		key = resp.KeysB64[0]
	} else if len(resp.Keys) > 0 {
		key = resp.Keys[0]
	}
	if _, err := c.API.Sys().Unseal(key); err != nil {
		t.Fatal(err)
	}
	c.API.SetToken(resp.RootToken)
	c.Cfg.Token = resp.RootToken
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-a", false); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-b", false); err != nil {
		t.Fatal(err)
	}

	tokA, err := c.LoginAppRole(cfg.TenantRole("reader", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	tokB, err := c.LoginAppRole(cfg.TenantRole("reader", "tenant-b"))
	if err != nil {
		t.Fatal(err)
	}
	tokAW, err := c.LoginAppRole(cfg.TenantRole("writer", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	tokBW, err := c.LoginAppRole(cfg.TenantRole("writer", "tenant-b"))
	if err != nil {
		t.Fatal(err)
	}

	provSec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{
		Policies:    []string{"provisioning"},
		Period:      "1h",
		NoParent:    true,
		DisplayName: "provisioning",
	})
	if err != nil || provSec == nil || provSec.Auth == nil {
		t.Fatalf("provisioning token: %v", err)
	}
	tokProv := provSec.Auth.ClientToken

	mustWrite := func(token, path, val string) {
		t.Helper()
		cl := c.WithToken(token)
		if _, err := cl.API.Logical().Write(path, map[string]any{"data": map[string]any{"value": val}}); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite(tokAW, cfg.KVDataPath("tenant-a", "fixture"), "secret-a")
	mustWrite(tokBW, cfg.KVDataPath("tenant-b", "fixture"), "secret-b")

	assertStatus := func(token, method, path string, body any, want int) {
		t.Helper()
		cl := c.WithToken(token)
		r, err := cl.Do(method, path, body)
		if err != nil && r.Status == 0 {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		if r.Status != want {
			t.Fatalf("%s %s: got HTTP %d want %d body %s", method, path, r.Status, want, r.Body)
		}
	}
	deny := func(token, method, path string, body any) {
		t.Helper()
		assertStatus(token, method, path, body, 403)
	}

	aData := cfg.KVDataPath("tenant-a", "fixture")
	bData := cfg.KVDataPath("tenant-b", "fixture")
	aMeta := cfg.KVMetaPath("tenant-a", "fixture")
	bMeta := cfg.KVMetaPath("tenant-b", "fixture")
	crossWrite := map[string]any{"data": map[string]any{"value": "FAKE-cross-tenant-write"}}

	assertStatus(tokA, "GET", aData, nil, 200)
	assertStatus(tokB, "GET", bData, nil, 200)
	assertStatus(tokA, "GET", "auth/token/lookup-self", nil, 200)

	deny(tokA, "GET", bData, nil)
	deny(tokB, "GET", aData, nil)
	deny(tokAW, "GET", bData, nil)
	deny(tokBW, "GET", aData, nil)
	deny(tokAW, "POST", bData, crossWrite)
	deny(tokBW, "POST", aData, crossWrite)
	deny(tokAW, "POST", cfg.KVDataPath("tenant-b", "newly-planted-secret"), map[string]any{
		"data": map[string]any{"value": "FAKE-planted"},
	})
	deny(tokAW, "DELETE", bData, nil)
	deny(tokAW, "POST", cfg.KVMount+"/destroy/"+cfg.TenantPrefix+"/tenant-b/fixture", map[string]any{"versions": []int{1}})
	deny(tokAW, "DELETE", bMeta, nil)
	deny(tokA, "GET", bMeta, nil)
	deny(tokB, "GET", aMeta, nil)
	deny(tokA, "GET", cfg.KVMount+"/metadata/"+cfg.TenantPrefix+"?list=true", nil)
	deny(tokB, "GET", cfg.KVMount+"/metadata/"+cfg.TenantPrefix+"?list=true", nil)
	deny(tokA, "GET", cfg.KVMetaPath("tenant-b", "")+"?list=true", nil)

	deny(tokA, "GET", cfg.KVMount+"/data/"+cfg.TenantPrefix, nil)
	deny(tokA, "GET", cfg.KVMount+"/data", nil)
	deny(tokA, "GET", cfg.KVMount+"/metadata/"+cfg.TenantPrefix, nil)
	deny(tokAW, "POST", cfg.KVMount+"/data/"+cfg.TenantPrefix+"/shared-secret", map[string]any{
		"data": map[string]any{"value": "FAKE-planted-at-parent"},
	})
	deny(tokA, "GET", cfg.KVMount+"/data/platform/root-credentials", nil)
	deny(tokAW, "POST", cfg.KVMount+"/data/platform/root-credentials", map[string]any{
		"data": map[string]any{"value": "FAKE-planted-sibling"},
	})
	deny(tokA, "GET", cfg.KVDataPath("tenant-a-extended", "secret"), nil)
	deny(tokA, "GET", cfg.KVDataPath("tenant-ax", "secret"), nil)
	deny(tokA, "GET", cfg.KVMount+"/data/"+cfg.TenantPrefix+"/*", nil)

	deny(tokA, "GET", "sys/mounts", nil)
	deny(tokA, "GET", "sys/auth", nil)
	deny(tokAW, "POST", "sys/mounts/rogue", map[string]any{"type": "kv", "options": map[string]any{"version": "2"}})
	deny(tokAW, "DELETE", "sys/mounts/"+cfg.KVMount, nil)
	deny(tokA, "GET", "sys/audit", nil)
	deny(tokA, "GET", "sys/policies/acl?list=true", nil)
	deny(tokA, "GET", "sys/policies/acl/"+cfg.TenantPolicy("reader", "tenant-b"), nil)
	deny(tokAW, "PUT", "sys/policies/acl/"+cfg.TenantPolicy("reader", "tenant-a"), map[string]any{
		"policy": `path "kv/data/*" { capabilities = ["read", "list"] }`,
	})
	deny(tokA, "GET", "auth/"+cfg.AuthMount+"/role?list=true", nil)
	deny(tokA, "GET", "auth/"+cfg.AuthMount+"/role/"+cfg.TenantRole("reader", "tenant-b")+"/role-id", nil)
	deny(tokAW, "POST", "auth/"+cfg.AuthMount+"/role/"+cfg.TenantRole("reader", "tenant-b")+"/secret-id", map[string]any{})
	deny(tokAW, "POST", "auth/token/create", map[string]any{"policies": []string{"operator"}})
	deny(tokA, "GET", "auth/token/accessors?list=true", nil)

	deny(tokProv, "GET", aData, nil)
	deny(tokProv, "GET", aMeta, nil)
	deny(tokProv, "POST", aData, map[string]any{"data": map[string]any{"value": "FAKE-written-by-provisioning"}})
	deny(tokProv, "PUT", "sys/policies/acl/provisioning", map[string]any{
		"policy": `path "kv/data/*" { capabilities = ["read"] }`,
	})
	assertStatus(tokProv, "GET", "auth/"+cfg.AuthMount+"/role/"+cfg.TenantRole("reader", "tenant-a")+"/role-id", nil, 200)

	for _, traversal := range []string{
		fmt.Sprintf("%s/data/%s/tenant-a/../tenant-b/fixture", cfg.KVMount, cfg.TenantPrefix),
		fmt.Sprintf("%s/data/%s/tenant-a/..%%2ftenant-b/fixture", cfg.KVMount, cfg.TenantPrefix),
	} {
		cl := c.WithToken(tokA)
		r, err := cl.Do("GET", traversal, nil)
		if err != nil && r.Status == 0 {
			t.Fatalf("traversal %s: %v", traversal, err)
		}
		if r.Status >= 200 && r.Status < 300 {
			t.Fatalf("traversal granted HTTP %d: %s", r.Status, traversal)
		}
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
