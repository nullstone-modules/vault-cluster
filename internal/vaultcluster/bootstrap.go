package vaultcluster

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/vault/api"
)

type BootstrapOptions struct {
	Shares     int
	Threshold  int
	KeepRoot   bool
	AutoUnseal bool
	ClaimInit  func() (bool, error)
}

const revokedRootMarker = "revoked-at-bootstrap"

// On a first boot the KMS seal waits for instance credentials before Vault opens its
// listener; about a minute was measured on a freshly created instance profile.
const readyTimeout = 3 * time.Minute

const initWaitTimeout = 5 * time.Minute

func (c *Client) RunBootstrap(store KeyStore, opts BootstrapOptions) error {
	if err := c.WaitReady(readyTimeout); err != nil {
		return err
	}

	leader, err := c.ensureInitialized(store, opts)
	if err != nil {
		return err
	}

	if opts.AutoUnseal {
		if err := c.waitUnsealed(); err != nil {
			return err
		}
	} else if err := c.unseal(store, opts.Threshold); err != nil {
		return err
	}

	if !leader {
		return c.waitPeerBootstrap(store, initWaitTimeout)
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

func (c *Client) ensureInitialized(store KeyStore, opts BootstrapOptions) (bool, error) {
	st, err := c.API.Sys().SealStatus()
	if err != nil {
		return false, err
	}
	if _, err := store.LoadInit(); err == nil {
		if !st.Initialized {
			if err := c.waitVaultInitialized(initWaitTimeout); err != nil {
				return false, err
			}
		}
		if opts.ClaimInit != nil {
			ok, err := opts.ClaimInit()
			if err != nil {
				return false, err
			}
			return ok, nil
		}
		return true, nil
	}
	if st.Initialized {
		if err := c.waitInitMaterial(store, initWaitTimeout); err != nil {
			return false, fmt.Errorf("Vault is initialized but key material is missing: %w", err)
		}
		return false, nil
	}
	claimed := true
	if opts.ClaimInit != nil {
		ok, err := opts.ClaimInit()
		if err != nil {
			return false, err
		}
		claimed = ok
	}
	if !claimed {
		if err := c.waitInitMaterial(store, initWaitTimeout); err != nil {
			if err := c.initVault(store, opts); err != nil {
				return false, err
			}
			return true, nil
		}
		if err := c.waitVaultInitialized(initWaitTimeout); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := c.initVault(store, opts); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Client) waitInitMaterial(store KeyStore, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := store.LoadInit(); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for init material")
}

func (c *Client) waitVaultInitialized(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := c.API.Sys().SealStatus()
		if err == nil && st != nil && st.Initialized {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for Vault to initialize")
}

func (c *Client) waitPeerBootstrap(store KeyStore, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tok, err := store.LoadToken("provisioning")
		if err == nil {
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
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for the bootstrap leader")
}

func initRequest(opts BootstrapOptions) *api.InitRequest {
	if opts.AutoUnseal {
		return &api.InitRequest{
			RecoveryShares:    opts.Shares,
			RecoveryThreshold: opts.Threshold,
		}
	}
	return &api.InitRequest{
		SecretShares:    opts.Shares,
		SecretThreshold: opts.Threshold,
	}
}

func (c *Client) initVault(store KeyStore, opts BootstrapOptions) error {
	if opts.AutoUnseal {
		log.Printf("initializing Vault (recovery %d/%d, auto-unseal)", opts.Shares, opts.Threshold)
	} else {
		log.Printf("initializing Vault (%d/%d Shamir)", opts.Shares, opts.Threshold)
	}
	resp, err := c.API.Sys().Init(initRequest(opts))
	if err != nil {
		st, stErr := c.API.Sys().SealStatus()
		if stErr == nil && st.Initialized {
			return nil
		}
		return err
	}
	if err := store.SaveInit(resp); err != nil {
		return err
	}
	log.Printf("initialized; key material saved (not logged)")
	return nil
}

func (c *Client) waitUnsealed() error {
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
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("Vault remained sealed (auto-unseal failed)")
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
