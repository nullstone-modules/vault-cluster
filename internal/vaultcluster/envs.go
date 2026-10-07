package vaultcluster

import (
	"fmt"
	"log"
	"sort"
)

// ListEnvs returns every env that has at least one tenant role on a shared cluster.
func (c *Client) ListEnvs() ([]string, error) {
	if !c.Cfg.SharedEnvs {
		return nil, fmt.Errorf("this cluster is not shared across envs")
	}
	tenants, err := c.ListTenants()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, t := range tenants {
		if t.Env != "" {
			seen[t.Env] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for env := range seen {
		out = append(out, env)
	}
	sort.Strings(out)
	return out, nil
}

// DestroyEnv offboards every tenant of one env. With purge it also destroys the env's KV subtree, which
// needs a token that can read and delete under kv/metadata/envs/<env> (provisioning cannot).
func (c *Client) DestroyEnv(env string, purge bool) error {
	if !c.Cfg.SharedEnvs {
		return fmt.Errorf("this cluster is not shared across envs")
	}
	if err := ValidateEnvName(env); err != nil {
		return err
	}
	tenants, err := c.ListTenants()
	if err != nil {
		return err
	}
	n := 0
	for _, t := range tenants {
		if t.Env != env {
			continue
		}
		if err := c.OffboardTenant(env, t.ID, false); err != nil {
			return err
		}
		n++
	}
	if purge {
		if err := c.purgeKVPrefix(fmt.Sprintf("%s/metadata/%s/%s", c.Cfg.KVMount, c.Cfg.EnvPrefix, env)); err != nil {
			return err
		}
	}
	log.Printf("env %s destroyed (%d tenants)", env, n)
	return nil
}
