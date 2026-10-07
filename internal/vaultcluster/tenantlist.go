package vaultcluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Tenant is one onboarded tenant. The AppRole roles are the record of truth: a tenant exists exactly when
// its roles exist. Reader or Writer is false when that mount has no role, which a failed create can leave.
// Env is set on a shared cluster (role name env.tenant) and empty otherwise.
type Tenant struct {
	Env    string
	ID     string
	Reader bool
	Writer bool
}

// Role is the AppRole role name this tenant logs in with.
func (t Tenant) Role() string {
	return Config{}.TenantRole(t.Env, t.ID)
}

// ListTenants reads the role names on both tenant mounts and merges them by role name.
func (c *Client) ListTenants() ([]Tenant, error) {
	byRole := map[string]*Tenant{}
	for _, kind := range []string{"reader", "writer"} {
		names, err := c.listRoles(c.Cfg.TenantMount(kind))
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			t := byRole[name]
			if t == nil {
				env, id := SplitTenantRole(name)
				t = &Tenant{Env: env, ID: id}
				byRole[name] = t
			}
			if kind == "reader" {
				t.Reader = true
			} else {
				t.Writer = true
			}
		}
	}
	out := make([]Tenant, 0, len(byRole))
	for _, t := range byRole {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Env != out[j].Env {
			return out[i].Env < out[j].Env
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (c *Client) listRoles(mount string) ([]string, error) {
	r, err := c.Do("LIST", "auth/"+mount+"/role", nil)
	if err != nil && r.Status == 0 {
		return nil, err
	}
	if r.Status == 404 {
		return nil, nil
	}
	if r.Status < 200 || r.Status >= 300 {
		return nil, fmt.Errorf("list %s roles failed (HTTP %d): %s", mount, r.Status, strings.TrimSpace(string(r.Body)))
	}
	var listed struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &listed); err != nil {
		return nil, err
	}
	return listed.Data.Keys, nil
}

// PrintTenants writes one line per tenant and marks tenants missing a role on either mount or whose role
// name does not fit the cluster mode. shared selects the env column.
func PrintTenants(w io.Writer, tenants []Tenant, shared bool) {
	if len(tenants) == 0 {
		fmt.Fprintln(w, "no tenants")
		return
	}
	for _, t := range tenants {
		label := t.ID
		if shared {
			label = t.Env + "\t" + t.ID
		}
		var notes []string
		switch {
		case t.Reader && t.Writer:
		case t.Reader:
			notes = append(notes, "no writer role; re-run tenants create")
		default:
			notes = append(notes, "no reader role; re-run tenants create")
		}
		if shared && t.Env == "" {
			notes = append(notes, "not env-scoped; this cluster is shared")
		}
		if !shared && t.Env != "" {
			notes = append(notes, "env-scoped; this cluster is not shared")
		}
		if len(notes) == 0 {
			fmt.Fprintln(w, label)
			continue
		}
		fmt.Fprintf(w, "%s\t(%s)\n", label, strings.Join(notes, "; "))
	}
}
