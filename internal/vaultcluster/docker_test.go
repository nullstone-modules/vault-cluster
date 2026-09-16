package vaultcluster

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
)

const (
	vaultTestImage    = "hashicorp/vault:1.21.4@sha256:4e33b126a59c0c333b76fb4e894722462659a6bec7c48c9ee8cea56fccfd2569"
	postgresTestImage = "postgres:16.15-alpine@sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685"
)

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

func dockerRun(t *testing.T, args ...string) string {
	t.Helper()
	id := lastLine(dockerOutput(t, append([]string{"run", "-d"}, args...)...))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })
	return id
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

func vaultListenAddr(t *testing.T, id string) string {
	t.Helper()
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
	return "http://127.0.0.1:" + hostPort
}

func startVaultInmem(t *testing.T, extraRunArgs ...string) *Client {
	t.Helper()
	requireDocker(t)
	cfgJSON := `{"disable_mlock":true,"listener":{"tcp":{"address":"0.0.0.0:8200","tls_disable":true}},"storage":{"inmem":{}}}`
	args := append([]string{
		"-p", "127.0.0.1::8200",
		"-e", "VAULT_LOCAL_CONFIG=" + cfgJSON,
		"--cap-add", "IPC_LOCK",
	}, extraRunArgs...)
	args = append(args, vaultTestImage, "server")
	id := dockerRun(t, args...)
	c, err := New(Config{Addr: vaultListenAddr(t, id), HTTPTimeout: 10 * time.Second})
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
	return c
}
