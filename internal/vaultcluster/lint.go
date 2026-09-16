package vaultcluster

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	pathRE = regexp.MustCompile(`path\s+"([^"]+)"`)
	capsRE = regexp.MustCompile(`capabilities\s*=\s*\[([^\]]*)\]`)
)

var kvV2Segments = map[string]struct{}{
	"data": {}, "metadata": {}, "delete": {}, "undelete": {},
	"destroy": {}, "config": {}, "subkeys": {},
}

var validCaps = map[string]struct{}{
	"create": {}, "read": {}, "update": {}, "delete": {}, "list": {},
	"patch": {}, "sudo": {}, "deny": {}, "recover": {}, "subscribe": {},
}

type Finding struct {
	Policy string
	Msg    string
}

func (f Finding) String() string {
	return f.Policy + ": " + f.Msg
}

func LintPolicy(policyName, src string, cfg Config) []Finding {
	type pair struct{ path, caps string }
	var pairs []pair
	lines := strings.Split(src, "\n")
	current := ""
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		if m := pathRE.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if m := capsRE.FindStringSubmatch(line); m != nil && current != "" {
			caps := strings.ReplaceAll(m[1], `"`, "")
			caps = strings.ReplaceAll(caps, " ", "")
			pairs = append(pairs, pair{current, caps})
			current = ""
		}
	}

	var findings []Finding
	add := func(msg string) {
		findings = append(findings, Finding{Policy: policyName, Msg: msg})
	}
	if len(pairs) == 0 {
		add("no path rules found")
		return findings
	}

	seen := map[string]struct{}{}
	for _, p := range pairs {
		if p.path == "*" || p.path == "/*" {
			if p.caps != "deny" {
				add(fmt.Sprintf("path %q grants [%s] over the entire Vault API", p.path, p.caps))
			}
		}
		if p.caps == "" {
			add(fmt.Sprintf("path %q has an empty capabilities list", p.path))
		}
		for _, c := range strings.Split(p.caps, ",") {
			if c == "" {
				continue
			}
			if _, ok := validCaps[c]; !ok {
				add(fmt.Sprintf("path %q has unknown capability %q", p.path, c))
			}
		}
		if strings.Contains(","+p.caps+",", ",deny,") && p.caps != "deny" {
			add(fmt.Sprintf("path %q mixes deny with [%s]", p.path, p.caps))
		}
		if strings.Contains(","+p.caps+",", ",sudo,") {
			if policyName != "operator" && policyName != "admin" {
				add(fmt.Sprintf("path %q grants sudo", p.path))
			}
		}

		if strings.HasPrefix(p.path, cfg.KVMount+"/") {
			rest := strings.TrimPrefix(p.path, cfg.KVMount+"/")
			second, _, _ := strings.Cut(rest, "/")
			ok := p.caps == "deny" || second == "*"
			if _, hit := kvV2Segments[second]; hit {
				ok = true
			}
			if !ok {
				add(fmt.Sprintf("path %q is not a valid KV v2 path", p.path))
			}
		}

		if strings.HasSuffix(p.path, "/"+cfg.TenantPrefix+"/*") || strings.HasSuffix(p.path, "/"+cfg.TenantPrefix+"/") {
			if strings.HasPrefix(p.path, cfg.KVMount+"/") && p.caps != "deny" {
				segs := strings.Split(p.path, "/")
				if len(segs) == 4 && segs[2] == cfg.TenantPrefix && segs[3] == "*" && p.caps != "deny" {
					add(fmt.Sprintf("path %q grants [%s] across all tenants", p.path, p.caps))
				}
			}
		}

		switch p.path {
		case cfg.KVMount + "/*", cfg.KVMount + "/data/*", cfg.KVMount + "/metadata/*":
			if p.caps != "deny" {
				add(fmt.Sprintf("path %q grants [%s] over the entire KV mount", p.path, p.caps))
			}
		}
		if p.path == cfg.DatabaseMount+"/creds/*" && p.caps != "deny" {
			add(fmt.Sprintf("path %q grants [%s] on every tenant's database credentials", p.path, p.caps))
		}
		if _, ok := seen[p.path]; ok {
			add(fmt.Sprintf("path %q is declared more than once", p.path))
		}
		seen[p.path] = struct{}{}
	}
	return findings
}

func LintOrError(policyName, src string, cfg Config) error {
	fs := LintPolicy(policyName, src, cfg)
	if len(fs) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "policy lint failed (%d):", len(fs))
	for _, f := range fs {
		fmt.Fprintf(&b, "\n  %s", f)
	}
	return fmt.Errorf("%s", b.String())
}
