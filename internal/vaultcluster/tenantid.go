package vaultcluster

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var tenantIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$`)

var reservedTenantIDs = map[string]struct{}{
	"sys": {}, "auth": {}, "identity": {}, "cubbyhole": {}, "root": {},
	"default": {}, "admin": {}, "data": {}, "metadata": {}, "delete": {},
	"undelete": {}, "destroy": {}, "config": {}, "subkeys": {}, "tenant": {},
	"customers": {},
}

func ValidateTenantID(id string) error {
	if id == "" {
		return fmt.Errorf("tenant ID is empty")
	}
	for _, r := range id {
		if unicode.IsSpace(r) {
			return fmt.Errorf("tenant ID contains whitespace")
		}
		if r > unicode.MaxASCII {
			return fmt.Errorf("tenant ID contains non-ASCII characters")
		}
	}
	switch {
	case strings.Contains(id, "/"):
		return fmt.Errorf("tenant ID contains a path separator")
	case strings.Contains(id, ".."):
		return fmt.Errorf("tenant ID contains '..'")
	case strings.Contains(id, "*"):
		return fmt.Errorf("tenant ID contains the ACL wildcard '*'")
	case strings.Contains(id, "+"):
		return fmt.Errorf("tenant ID contains '+'")
	case strings.ContainsAny(id, "{}"):
		return fmt.Errorf("tenant ID contains a brace")
	case strings.Contains(id, `\`):
		return fmt.Errorf("tenant ID contains a backslash")
	case strings.ContainsAny(id, `"'`):
		return fmt.Errorf("tenant ID contains a quote")
	case strings.ContainsAny(id, "$`"):
		return fmt.Errorf("tenant ID contains a shell metacharacter")
	case strings.Contains(id, "%"):
		return fmt.Errorf("tenant ID contains '%%'")
	}
	if !tenantIDPattern.MatchString(id) {
		return fmt.Errorf("tenant ID %q does not match %s", id, tenantIDPattern)
	}
	if _, ok := reservedTenantIDs[id]; ok {
		return fmt.Errorf("tenant ID %q is reserved", id)
	}
	return nil
}
