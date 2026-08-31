package vaultcluster

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/vault/api"
)

type BootstrapOptions struct {
	Shares    int
	Threshold int
	KeepRoot  bool
}

const revokedRootMarker = "revoked-at-bootstrap"

func (c *Client) RunBootstrap(store KeyStore, opts BootstrapOptions) error {
	if err := c.WaitReady(60 * time.Second); err != nil {
		return err
	}

	st, err := c.API.Sys().SealStatus()
	if err != nil {
		return err
	}
	if !st.Initialized {
		log.Printf("initializing Vault (%d/%d Shamir)", opts.Shares, opts.Threshold)
		resp, err := c.API.Sys().Init(&api.InitRequest{
			SecretShares:    opts.Shares,
			SecretThreshold: opts.Threshold,
		})
		if err != nil {
			return err
		}
		if err := store.SaveInit(resp); err != nil {
			return err
		}
		log.Printf("initialized; key material saved (not logged)")
	} else if _, err := store.LoadInit(); err != nil {
		return fmt.Errorf("Vault is initialized but key material is missing: %w", err)
	}

	if err := c.unseal(store, opts.Threshold); err != nil {
		return err
	}

	if tok, err := store.LoadToken("provisioning"); err == nil {
		c.API.SetToken(tok)
		if _, err := c.API.Auth().Token().LookupSelf(); err == nil {
			c.Cfg.Token = tok
			if err := c.enableCredentialsIfNeeded(store); err != nil {
				return err
			}
			log.Printf("platform already bootstrapped")
			return nil
		}
	}

	initResp, err := store.LoadInit()
	if err != nil {
		return err
	}
	root := initResp.RootToken
	if root == "" || root == revokedRootMarker {
		return fmt.Errorf("root token is missing or already revoked; restore key material or bootstrap with keep-root")
	}
	c.API.SetToken(root)
	c.Cfg.Token = root

	if err := c.Configure(); err != nil {
		return err
	}

	for _, name := range []string{"provisioning", "operator"} {
		tok, err := c.issueOrphanToken(name)
		if err != nil {
			return err
		}
		if err := store.SaveToken(name, tok); err != nil {
			return err
		}
	}

	if opts.KeepRoot {
		log.Printf("keeping the root token active")
		return nil
	}
	if err := c.API.Auth().Token().RevokeSelf(""); err != nil {
		log.Printf("root token revocation failed: %v", err)
	} else {
		initResp.RootToken = revokedRootMarker
		_ = store.SaveInit(initResp)
		log.Printf("revoked the initial root token")
	}
	return nil
}

func (c *Client) WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := c.API.Sys().SealStatus()
		if err == nil && st != nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("Vault did not respond within %s", timeout)
}

func (c *Client) unseal(store KeyStore, threshold int) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		st, err := c.API.Sys().SealStatus()
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if !st.Sealed {
			return nil
		}
		initResp, err := store.LoadInit()
		if err != nil {
			return err
		}
		keys := initResp.KeysB64
		if len(keys) == 0 {
			keys = initResp.Keys
		}
		n := threshold
		if n > len(keys) {
			n = len(keys)
		}
		for i := 0; i < n; i++ {
			if _, err := c.API.Sys().Unseal(keys[i]); err != nil {
				log.Printf("unseal share %d: %v", i, err)
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("Vault remained sealed")
}

func (c *Client) enableCredentialsIfNeeded(store KeyStore) error {
	if !c.Cfg.EnableCredentials || c.databaseMounted() {
		return nil
	}
	tok, err := store.LoadToken("operator")
	if err != nil {
		return fmt.Errorf("database engine is not mounted and the operator token is missing")
	}
	if err := c.WithToken(tok).mountDatabase(); err != nil {
		return fmt.Errorf("enable database engine: %w", err)
	}
	log.Printf("enabled database engine")
	return nil
}

func (c *Client) databaseMounted() bool {
	mounts, err := c.API.Sys().ListMounts()
	if err != nil {
		return false
	}
	_, ok := mounts[c.Cfg.DatabaseMount+"/"]
	return ok
}

func (c *Client) issueOrphanToken(policy string) (string, error) {
	sec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{
		Policies:    []string{policy},
		Period:      "24h",
		Renewable:   boolPtr(true),
		DisplayName: policy,
		NoParent:    true,
	})
	if err != nil {
		return "", err
	}
	if sec == nil || sec.Auth == nil {
		return "", fmt.Errorf("failed to issue %s token", policy)
	}
	return sec.Auth.ClientToken, nil
}

func boolPtr(b bool) *bool { return &b }
