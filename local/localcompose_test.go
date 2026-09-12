package local

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

const localComposeFile = "compose.yml"

func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping docker")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon is not running")
	}
}

func dockerOutput(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		stderr := []byte(nil)
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("docker %v: %v\n%s\n%s", args, err, out, stderr)
	}
	return strings.TrimSpace(string(out))
}

// TestLocalComposeStatic lints compose.yml without Docker: images are
// digest-pinned and never latest, no Vault dev mode, and every published port
// binds to loopback only.
func TestLocalComposeStatic(t *testing.T) {
	b, err := os.ReadFile(localComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "image:") {
			if !strings.Contains(trimmed, "@sha256:") {
				t.Errorf("image is not digest-pinned: %s", trimmed)
			}
			if strings.Contains(trimmed, ":latest") {
				t.Errorf("image uses the latest tag: %s", trimmed)
			}
		}
	}

	if strings.Contains(src, "VAULT_DEV_") || regexp.MustCompile(`["'\s]-dev["'\s,\]]`).MatchString(src) {
		t.Error("dev-mode configuration found in compose.yml")
	}

	// Port publications look like - "host:container"; volume mounts contain a
	// path separator in the host part.
	portRe := regexp.MustCompile(`-\s+"([^"/]+):\d+"`)
	found := false
	for _, m := range portRe.FindAllStringSubmatch(src, -1) {
		found = true
		if !strings.HasPrefix(m[1], "127.0.0.1:") {
			t.Errorf("published port is not bound to loopback: %s", m[0])
		}
	}
	if !found {
		t.Error("no published ports found; the loopback check matched nothing")
	}
}

// TestLocalComposeRuntime runs the real local stack in an isolated Compose
// project: health-gated one-shot bootstrap, Shamir over Raft, separated audit
// and Raft volumes, loopback-only exposure, and persistence across a restart
// (Vault reseals, the one-shot unseals, data and tokens survive).
func TestLocalComposeRuntime(t *testing.T) {
	requireDocker(t)
	composeFile, err := filepath.Abs(localComposeFile)
	if err != nil {
		t.Fatal(err)
	}

	project := fmt.Sprintf("vault-rt-%d", time.Now().UnixNano())
	bootstrapDir := t.TempDir()
	env := append(os.Environ(),
		"BOOTSTRAP_HOST_DIR="+bootstrapDir,
		"VAULT_HOST_PORT=0", // ephemeral host port; never collides with a dev stack
	)
	compose := func(args ...string) *exec.Cmd {
		cmd := exec.Command("docker", append([]string{"compose", "-p", project, "-f", composeFile}, args...)...)
		cmd.Env = env
		return cmd
	}
	composeOut := func(args ...string) string {
		t.Helper()
		out, err := compose(args...).Output()
		if err != nil {
			stderr := []byte(nil)
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = ee.Stderr
			}
			t.Fatalf("docker compose %v: %v\n%s\n%s", args, err, out, stderr)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := compose("logs").CombinedOutput()
			t.Logf("compose logs:\n%s", logs)
		}
		_ = compose("down", "--volumes", "--remove-orphans", "--timeout", "5").Run()
	})

	composeOut("up", "-d", "--wait", "vault")
	composeOut("run", "--build", "--rm", "--no-deps", "bootstrap")

	running := composeOut("ps", "--status", "running", "--format", "{{.Service}}")
	if running != "vault" {
		t.Fatalf("expected only vault to stay running (bootstrap is one-shot), got: %q", running)
	}

	addr := "http://" + composeOut("port", "vault", "8200")
	c, err := vaultcluster.New(vaultcluster.Config{Addr: addr, HTTPTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	if res, err := c.Do("GET", "sys/health?standbyok=true", nil); err != nil || res.Status != 200 {
		t.Fatalf("expected health 200 (initialized, unsealed, active): status=%d err=%v", res.Status, err)
	}
	st, err := c.API.Sys().SealStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Type != "shamir" {
		t.Errorf("seal type = %q, want shamir (real unseal flow must be exercised)", st.Type)
	}
	if st.StorageType != "raft" {
		t.Errorf("storage type = %q, want raft", st.StorageType)
	}

	cid := composeOut("ps", "-q", "vault")
	mountName := func(dest string) string {
		return dockerOutput(t, "inspect", "--format",
			`{{range .Mounts}}{{if eq .Destination "`+dest+`"}}{{.Name}}{{end}}{{end}}`, cid)
	}
	raftVol, auditVol := mountName("/vault/file"), mountName("/vault/logs")
	if raftVol == "" || auditVol == "" {
		t.Errorf("raft and audit volumes must both be mounted: raft=%q audit=%q", raftVol, auditVol)
	} else if raftVol == auditVol {
		t.Errorf("audit shares volume %q with raft; audit growth can fill or corrupt Raft storage", raftVol)
	}
	dockerOutput(t, "exec", cid, "sh", "-c", "test -s /vault/logs/audit.log && test -e /vault/file/vault.db")

	hostIPs := dockerOutput(t, "inspect", "--format",
		`{{range $p, $conf := .NetworkSettings.Ports}}{{range $conf}}{{.HostIp}} {{end}}{{end}}`, cid)
	if strings.Contains(hostIPs, "0.0.0.0") || !strings.Contains(hostIPs, "127.0.0.1") {
		t.Errorf("Vault port must be published to 127.0.0.1 only, got bindings: %q", hostIPs)
	}

	// Persistence probe: onboard a tenant, write as its AppRole writer,
	// restart Vault (reseals under Shamir), unseal via the one-shot, and
	// verify the secret and the pre-restart token both survived.
	tokenBytes, err := os.ReadFile(filepath.Join(bootstrapDir, "provisioning.token"))
	if err != nil {
		t.Fatalf("bootstrap did not write provisioning.token: %v", err)
	}
	provToken := strings.TrimSpace(string(tokenBytes))
	composeOut("run", "--rm", "--no-deps", "-e", "VAULT_TOKEN="+provToken,
		"bootstrap", "tenants", "create", "tenant-a")

	prov := c.WithToken(provToken)
	roleResp, err := prov.API.Logical().Read("auth/approle/role/tenant-tenant-a-writer/role-id")
	if err != nil || roleResp == nil {
		t.Fatalf("reading writer role-id: %v", err)
	}
	secResp, err := prov.API.Logical().Write("auth/approle/role/tenant-tenant-a-writer/secret-id", nil)
	if err != nil || secResp == nil {
		t.Fatalf("issuing writer secret-id: %v", err)
	}
	loginResp, err := c.API.Logical().Write("auth/approle/login", map[string]any{
		"role_id":   roleResp.Data["role_id"],
		"secret_id": secResp.Data["secret_id"],
	})
	if err != nil || loginResp == nil || loginResp.Auth == nil {
		t.Fatalf("AppRole login as tenant writer: %v", err)
	}
	writer := c.WithToken(loginResp.Auth.ClientToken)

	probePath := "kv/data/customers/tenant-a/runtime-persistence-probe"
	probeValue := fmt.Sprintf("FAKE-persistence-probe-%d", time.Now().UnixNano())
	if _, err := writer.API.Logical().Write(probePath, map[string]any{
		"data": map[string]any{"probe": probeValue},
	}); err != nil {
		t.Fatalf("writing probe secret: %v", err)
	}

	composeOut("restart", "vault")
	// The ephemeral host port changes on restart; re-resolve and rebuild clients.
	addr = "http://" + composeOut("port", "vault", "8200")
	c, err = vaultcluster.New(vaultcluster.Config{Addr: addr, HTTPTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	writer = c.WithToken(loginResp.Auth.ClientToken)

	sealedAfterRestart := false
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := c.API.Sys().SealStatus(); err == nil && st != nil {
			sealedAfterRestart = st.Sealed
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !sealedAfterRestart {
		t.Error("Vault should reseal after a process restart (expected for Shamir)")
	}

	composeOut("run", "--rm", "--no-deps", "bootstrap")
	if st, err := c.API.Sys().SealStatus(); err != nil || st.Sealed {
		t.Fatalf("one-shot bootstrap should leave Vault unsealed: sealed=%v err=%v", st != nil && st.Sealed, err)
	}

	read, err := writer.API.Logical().Read(probePath)
	if err != nil || read == nil {
		t.Fatalf("reading probe secret after restart: %v", err)
	}
	data, _ := read.Data["data"].(map[string]any)
	if got, _ := data["probe"].(string); got != probeValue {
		t.Errorf("probe secret did not survive the restart: got %q, want %q", got, probeValue)
	}
	if _, err := writer.API.Auth().Token().LookupSelf(); err != nil {
		t.Errorf("pre-restart writer token no longer valid (token storage did not persist): %v", err)
	}
}
