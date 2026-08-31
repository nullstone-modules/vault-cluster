package vaultcluster

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCredentialsMatrix(t *testing.T) {
	pg, c := credentialsStack(t)
	c.Cfg.EnableCredentials = true
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-a", false); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-b", false); err != nil {
		t.Fatal(err)
	}

	tokA, err := c.LoginAppRole(c.Cfg.TenantRole("writer", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	tokB, err := c.LoginAppRole(c.Cfg.TenantRole("writer", "tenant-b"))
	if err != nil {
		t.Fatal(err)
	}
	tokAR, err := c.LoginAppRole(c.Cfg.TenantRole("reader", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}

	a := c.WithToken(tokA)
	ro, err := a.API.Logical().Read(c.Cfg.DatabaseMount + "/creds/tenant-tenant-a-readonly")
	if err != nil || ro == nil || ro.Data == nil {
		t.Fatalf("issue readonly: %v %#v", err, ro)
	}
	roUser := fmt.Sprint(ro.Data["username"])
	roPass := fmt.Sprint(ro.Data["password"])

	out, err := pgQuery(pg, roUser, roPass, "SELECT count(*) FROM app.items;")
	if err != nil {
		t.Fatalf("readonly SELECT: %v %s", err, out)
	}
	if strings.TrimSpace(out) != "1" {
		t.Fatalf("readonly SELECT count=%q", out)
	}
	if _, err := pgQuery(pg, roUser, roPass, "INSERT INTO app.items (name) VALUES ('nope');"); err == nil {
		t.Fatal("readonly INSERT succeeded")
	}

	rw, err := a.API.Logical().Read(c.Cfg.DatabaseMount + "/creds/tenant-tenant-a-readwrite")
	if err != nil || rw == nil || rw.Data == nil {
		t.Fatalf("issue readwrite: %v", err)
	}
	rwUser := fmt.Sprint(rw.Data["username"])
	rwPass := fmt.Sprint(rw.Data["password"])
	if _, err := pgQuery(pg, rwUser, rwPass, "INSERT INTO app.items (name) VALUES ('ok');"); err != nil {
		t.Fatalf("readwrite INSERT: %v", err)
	}

	b := c.WithToken(tokB)
	r, err := b.Do("GET", c.Cfg.DatabaseMount+"/creds/tenant-tenant-a-readonly", nil)
	if err != nil && r.Status == 0 {
		t.Fatal(err)
	}
	if r.Status != 403 {
		t.Fatalf("cross-tenant creds: HTTP %d want 403", r.Status)
	}
	r, err = c.WithToken(tokAR).Do("GET", c.Cfg.DatabaseMount+"/creds/tenant-tenant-a-readonly", nil)
	if err != nil && r.Status == 0 {
		t.Fatal(err)
	}
	if r.Status != 403 {
		t.Fatalf("reader creds: HTTP %d want 403", r.Status)
	}

	if err := c.API.Sys().Revoke(ro.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pgQuery(pg, roUser, roPass, "SELECT 1;"); err == nil {
		t.Fatal("revoked readonly still connects")
	}
}

func TestCredentialsAfterIsolation(t *testing.T) {
	pg, c := credentialsStack(t)
	c.Cfg.EnableCredentials = false
	if err := c.Configure(); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTenant("tenant-a", false); err != nil {
		t.Fatal(err)
	}

	store := FileKeyStore{Dir: t.TempDir()}
	for _, name := range []string{"operator", "provisioning"} {
		tok, err := c.issueOrphanToken(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveToken(name, tok); err != nil {
			t.Fatal(err)
		}
	}
	provTok, err := store.LoadToken("provisioning")
	if err != nil {
		t.Fatal(err)
	}
	prov := c.WithToken(provTok)
	prov.Cfg.EnableCredentials = true
	if err := prov.enableCredentialsIfNeeded(store); err != nil {
		t.Fatal(err)
	}
	if err := prov.CreateTenant("tenant-a", false); err != nil {
		t.Fatal(err)
	}

	tokA, err := c.LoginAppRole(c.Cfg.TenantRole("writer", "tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	ro, err := c.WithToken(tokA).API.Logical().Read(c.Cfg.DatabaseMount + "/creds/tenant-tenant-a-readonly")
	if err != nil || ro == nil || ro.Data == nil {
		t.Fatalf("issue readonly after isolation: %v %#v", err, ro)
	}
	roUser := fmt.Sprint(ro.Data["username"])
	roPass := fmt.Sprint(ro.Data["password"])
	out, err := pgQuery(pg, roUser, roPass, "SELECT count(*) FROM app.items;")
	if err != nil {
		t.Fatalf("readonly SELECT: %v %s", err, out)
	}
	if strings.TrimSpace(out) != "1" {
		t.Fatalf("readonly SELECT count=%q", out)
	}
}

func credentialsStack(t *testing.T) (string, *Client) {
	t.Helper()
	requireDocker(t)

	net := fmt.Sprintf("vctest-%d", time.Now().UnixNano())
	if err := exec.Command("docker", "network", "create", net).Run(); err != nil {
		t.Fatalf("network create: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "network", "rm", net).Run() })

	pgPass := "local-dev-only-not-a-real-secret"
	pg := dockerRun(t,
		"--name", fmt.Sprintf("vctest-pg-%d", time.Now().UnixNano()),
		"--network", net, "--network-alias", "pg",
		"-e", "POSTGRES_PASSWORD="+pgPass,
		"-e", "POSTGRES_DB=appdb",
		postgresTestImage,
	)
	deadline := time.Now().Add(45 * time.Second)
	var last string
	ready := false
	for time.Now().Before(deadline) {
		out, err := pgQuery(pg, "postgres", pgPass, "SELECT 1;")
		if err == nil {
			ready = true
			break
		}
		last = strings.TrimSpace(fmt.Sprintf("%v %s", err, out))
		time.Sleep(250 * time.Millisecond)
	}
	if !ready {
		logs, _ := exec.Command("docker", "logs", pg).CombinedOutput()
		t.Fatalf("postgres did not become ready: %s\n%s", last, logs)
	}

	mustPG(t, pg, "postgres", pgPass, `
CREATE ROLE app_readonly NOLOGIN;
CREATE ROLE app_readwrite NOLOGIN;
CREATE ROLE vault_admin LOGIN CREATEROLE PASSWORD 'local-dev-only-vault-admin-not-a-real-secret';
GRANT app_readonly TO vault_admin WITH ADMIN OPTION;
GRANT app_readwrite TO vault_admin WITH ADMIN OPTION;
CREATE SCHEMA app;
CREATE TABLE app.items (id serial PRIMARY KEY, name text NOT NULL);
INSERT INTO app.items (name) VALUES ('row-a');
GRANT USAGE ON SCHEMA app TO app_readonly, app_readwrite;
GRANT SELECT ON ALL TABLES IN SCHEMA app TO app_readonly;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO app_readwrite;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA app TO app_readwrite;
`)

	c := startVaultInmem(t, "--network", net)
	c.Cfg.KVMount = "kv"
	c.Cfg.TenantPrefix = "customers"
	c.Cfg.AuthMount = "approle"
	c.Cfg.DatabaseMount = "database"
	c.Cfg.EnableAudit = false
	c.Cfg.DatabaseURL = "postgresql://{{username}}:{{password}}@pg:5432/appdb?sslmode=disable"
	c.Cfg.DatabaseUsername = "vault_admin"
	c.Cfg.DatabasePassword = "local-dev-only-vault-admin-not-a-real-secret"
	c.Cfg.DatabaseConnName = "app"
	c.Cfg.DatabaseTTL = "1h"
	c.Cfg.DatabaseMaxTTL = "24h"
	c.Cfg.TokenTTL = "1h"
	c.Cfg.TokenMaxTTL = "24h"
	return pg, c
}

func mustPG(t *testing.T, id, user, pass, sql string) {
	t.Helper()
	out, err := pgQuery(id, user, pass, sql)
	if err != nil {
		t.Fatalf("psql: %v\n%s", err, out)
	}
}

func pgQuery(id, user, pass, sql string) (string, error) {
	cmd := exec.Command("docker", "exec", "-e", "PGPASSWORD="+pass, id,
		"psql", "-U", user, "-d", "appdb", "-v", "ON_ERROR_STOP=1", "-tAc", sql)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
