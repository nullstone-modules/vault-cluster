package vaultcluster

import "testing"

func TestValidateTenantID_accepts(t *testing.T) {
	for _, id := range []string{"tenant-a", "tenant-b", "acme-corp"} {
		if err := ValidateTenantID(id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

func TestValidateTenantID_rejects(t *testing.T) {
	for _, id := range []string{
		"a/b", "../etc", "tenant-*", "TENANT", "ab", "sys", "data",
		"a b", "a_b", "-x", "x-", "",
	} {
		if err := ValidateTenantID(id); err == nil {
			t.Errorf("%q should have been rejected", id)
		}
	}
}
