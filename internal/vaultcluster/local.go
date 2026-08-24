package vaultcluster

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hashicorp/vault/api"
)

type LocalBootstrap struct {
	Dir       string
	InitFile  string
	Shares    int
	Threshold int
	KeepRoot  bool
	TenantIDs []string
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

func (c *Client) RunLocalBootstrap(opts LocalBootstrap) error {
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return err
	}
	if opts.InitFile == "" {
		opts.InitFile = filepath.Join(opts.Dir, "vault-init.json")
	}
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
		b, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if err := writePrivate(opts.InitFile, b); err != nil {
			return err
		}
		log.Printf("initialized; recovery material written (not logged)")
	} else if _, err := os.Stat(opts.InitFile); err != nil {
		return fmt.Errorf("Vault is initialized but %s is missing", opts.InitFile)
	}

	if err := c.unsealFromFile(opts.InitFile, opts.Threshold); err != nil {
		return err
	}

	provFile := filepath.Join(opts.Dir, "provisioning.token")
	if b, err := os.ReadFile(provFile); err == nil {
		tok := string(b)
		c.API.SetToken(tok)
		if _, err := c.API.Auth().Token().LookupSelf(); err == nil {
			c.Cfg.Token = tok
			for _, id := range opts.TenantIDs {
				if err := c.CreateTenant(id, false); err != nil {
					return err
				}
			}
			log.Printf("platform already bootstrapped")
			return chownBootstrap(opts.Dir)
		}
	}

	initRaw, err := os.ReadFile(opts.InitFile)
	if err != nil {
		return err
	}
	var initResp api.InitResponse
	if err := json.Unmarshal(initRaw, &initResp); err != nil {
		return err
	}
	root := initResp.RootToken
	if root == "" || root == "revoked-at-bootstrap" {
		return fmt.Errorf("root token in %s is missing or already revoked; restore keys or keep-root", opts.InitFile)
	}
	c.API.SetToken(root)
	c.Cfg.Token = root

	if err := c.Configure(); err != nil {
		return err
	}

	for _, id := range opts.TenantIDs {
		if err := c.CreateTenant(id, false); err != nil {
			return err
		}
	}

	if err := c.issueOrphanToken("provisioning", filepath.Join(opts.Dir, "provisioning.token")); err != nil {
		return err
	}
	if err := c.issueOrphanToken("operator", filepath.Join(opts.Dir, "operator.token")); err != nil {
		return err
	}

	if opts.KeepRoot {
		log.Printf("keeping the root token active")
		return chownBootstrap(opts.Dir)
	}
	if err := c.API.Auth().Token().RevokeSelf(""); err != nil {
		log.Printf("root token revocation failed: %v", err)
	} else {
		initResp.RootToken = "revoked-at-bootstrap"
		b, _ := json.Marshal(initResp)
		_ = writePrivate(opts.InitFile, b)
		log.Printf("revoked the initial root token")
	}
	return chownBootstrap(opts.Dir)
}

func (c *Client) unsealFromFile(path string, threshold int) error {
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
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var initResp api.InitResponse
		if err := json.Unmarshal(raw, &initResp); err != nil {
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

func (c *Client) issueOrphanToken(policy, outfile string) error {
	sec, err := c.API.Auth().Token().Create(&api.TokenCreateRequest{
		Policies:    []string{policy},
		Period:      "24h",
		Renewable:   boolPtr(true),
		DisplayName: policy,
		NoParent:    true,
	})
	if err != nil {
		return err
	}
	if sec == nil || sec.Auth == nil {
		return fmt.Errorf("failed to issue %s token", policy)
	}
	return writePrivate(outfile, []byte(sec.Auth.ClientToken))
}

func boolPtr(b bool) *bool { return &b }

func writePrivate(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func chownBootstrap(dir string) error {
	if os.Getuid() != 0 {
		return nil
	}
	uid, gid := 0, 0
	if fi, err := os.Stat(dir); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			uid = int(st.Uid)
			gid = int(st.Gid)
		}
	}
	if v := os.Getenv("BOOTSTRAP_UID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			uid = n
		}
	}
	if v := os.Getenv("BOOTSTRAP_GID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			gid = n
		}
	}
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(p, uid, gid)
	})
}
