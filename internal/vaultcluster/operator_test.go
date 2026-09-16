package vaultcluster

import "testing"

func TestOperatorBreakGlass(t *testing.T) {
	c := startVaultInmem(t)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	tok, err := c.issueOrphanToken("operator")
	if err != nil {
		t.Fatal(err)
	}
	op := c.WithToken(tok)

	r, err := op.Do("PUT", "sys/generate-root/attempt", map[string]any{})
	if err != nil && r.Status == 0 {
		t.Fatalf("generate-root attempt: %v", err)
	}
	if r.Status == 403 {
		t.Fatalf("operator should start generate-root, got 403 %s", r.Body)
	}
	if r.Status < 200 || r.Status >= 300 {
		t.Fatalf("generate-root attempt HTTP %d %s", r.Status, r.Body)
	}
	_, _ = op.Do("DELETE", "sys/generate-root/attempt", nil)

	r, err = op.Do("POST", "sys/storage/raft/snapshot-force", nil)
	if err != nil && r.Status == 0 {
		t.Fatalf("snapshot-force: %v", err)
	}
	if r.Status != 403 {
		t.Fatalf("operator snapshot-force: got HTTP %d want 403 body %s", r.Status, r.Body)
	}
}
