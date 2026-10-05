package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

func runAdmins(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 || args[0] != "reconcile" {
		return fmt.Errorf("usage: vault-utils admins reconcile")
	}
	if os.Getenv("VAULT_ADMINS_FILE") == "" {
		return fmt.Errorf("VAULT_ADMINS_FILE is not set")
	}
	var store vaultcluster.KeyStore
	if c.Cfg.Token == "" {
		s, err := keyStore()
		if err != nil {
			return err
		}
		store = s
	}
	return reconcileAdmins(c, store)
}

// reconcileAdmins applies VAULT_ADMINS_FILE with the admin-auth token, or VAULT_TOKEN when store is nil.
// Without VAULT_ADMINS_FILE it does nothing, so older node config never revokes admins.
func reconcileAdmins(c *vaultcluster.Client, store vaultcluster.KeyStore) error {
	path := os.Getenv("VAULT_ADMINS_FILE")
	if path == "" {
		return nil
	}
	cfg, err := vaultcluster.LoadAdminConfig(path)
	if err != nil {
		return err
	}
	cl := c
	if store != nil {
		tok, err := adminToken(store, cfg)
		if err != nil {
			return err
		}
		cl = c.WithToken(tok)
	}
	if err := cl.ReconcileAdmins(cfg); err != nil {
		return err
	}
	log.Printf("admin roles reconciled (%d)", len(cfg.Bindings))
	return nil
}

// adminToken prefers admin-auth. AWS clusters bootstrapped before it existed fall back to aws-auth,
// which can write only aws roles.
func adminToken(store vaultcluster.KeyStore, cfg vaultcluster.AdminConfig) (string, error) {
	tok, err := store.LoadToken("admin-auth")
	if err == nil && tok != "" {
		return tok, nil
	}
	for _, b := range cfg.Bindings {
		if b.Method != "aws" {
			return "", fmt.Errorf("admin-auth token: %v (needed for %s bindings)", err, b.Method)
		}
	}
	log.Printf("admin-auth token unavailable (%v); using aws-auth", err)
	tok, err = store.LoadToken("aws-auth")
	if err != nil {
		return "", fmt.Errorf("aws-auth token: %w", err)
	}
	return tok, nil
}

// renewPlatformTokens keeps the 24h periodic provisioning, aws-auth, and admin-auth tokens alive.
// Nothing else renews them.
func renewPlatformTokens(c *vaultcluster.Client) {
	store, err := keyStore()
	if err != nil {
		log.Printf("platform token renewal disabled: %v", err)
		return
	}
	for _, name := range []string{"provisioning", "aws-auth", "admin-auth"} {
		tok, err := store.LoadToken(name)
		if err != nil || tok == "" {
			log.Printf("%s token renewal disabled: %v", name, err)
			continue
		}
		go c.WithToken(tok).RenewToken(tokenRenewInterval, nil)
	}
}
