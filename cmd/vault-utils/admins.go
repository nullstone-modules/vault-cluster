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

// reconcileAdmins writes VAULT_ADMINS_FILE with the aws-auth token (VAULT_TOKEN wins when set).
// Without VAULT_ADMINS_FILE it does nothing, so older user-data never revokes admins.
func reconcileAdmins(c *vaultcluster.Client, store vaultcluster.KeyStore) error {
	path := os.Getenv("VAULT_ADMINS_FILE")
	if path == "" {
		return nil
	}
	bindings, err := vaultcluster.LoadAdminBindings(path)
	if err != nil {
		return err
	}
	cl := c
	if store != nil {
		tok, err := store.LoadToken("aws-auth")
		if err != nil {
			return fmt.Errorf("aws-auth token: %w", err)
		}
		cl = c.WithToken(tok)
	}
	if err := cl.ReconcileAdmins(bindings); err != nil {
		return err
	}
	log.Printf("admin roles reconciled (%d)", len(bindings))
	return nil
}

// renewPlatformTokens keeps the 24h periodic provisioning and aws-auth tokens alive.
// Nothing else renews them, and the app-facing function and admin reconcile depend on aws-auth.
func renewPlatformTokens(c *vaultcluster.Client) {
	store, err := keyStore()
	if err != nil {
		log.Printf("platform token renewal disabled: %v", err)
		return
	}
	for _, name := range []string{"provisioning", "aws-auth"} {
		tok, err := store.LoadToken(name)
		if err != nil {
			log.Printf("%s token renewal disabled: %v", name, err)
			continue
		}
		go c.WithToken(tok).RenewToken(tokenRenewInterval, nil)
	}
}
