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
type Tenant struct {
	ID     string
	Reader bool
	Writer bool
}

// ListTenants reads the role names on both tenant mounts and merges them by tenant ID.
func (c *Client) ListTenants() ([]Tenant, error) {
	byID := map[string]*Tenant{}
	for _, kind := range []string{"reader", "writer"} {
		names, err := c.listRoles(c.Cfg.TenantMount(kind))
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			t := byID[name]
			if t == nil {
				t = &Tenant{ID: name}
				byID[name] = t
			}
			if kind == "reader" {
				t.Reader = true
			} else {
				t.Writer = true
			}
		}
	}
	out := make([]Tenant, 0, len(byID))
	for _, t := range byID {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
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

// PrintTenants writes one line per tenant and marks tenants missing a role on either mount.
func PrintTenants(w io.Writer, tenants []Tenant) {
	if len(tenants) == 0 {
		fmt.Fprintln(w, "no tenants")
		return
	}
	for _, t := range tenants {
		switch {
		case t.Reader && t.Writer:
			fmt.Fprintln(w, t.ID)
		case t.Reader:
			fmt.Fprintf(w, "%s\t(no writer role; re-run tenants create)\n", t.ID)
		default:
			fmt.Fprintf(w, "%s\t(no reader role; re-run tenants create)\n", t.ID)
		}
	}
}
