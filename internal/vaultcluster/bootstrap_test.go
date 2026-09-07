package vaultcluster

import "testing"

func TestInitRequestAutoUnsealUsesRecovery(t *testing.T) {
	req := initRequest(BootstrapOptions{Shares: 1, Threshold: 1, AutoUnseal: true})
	if req.RecoveryShares != 1 || req.RecoveryThreshold != 1 {
		t.Fatalf("recovery: %+v", req)
	}
	if req.SecretShares != 0 || req.SecretThreshold != 0 {
		t.Fatalf("shamir should be unset: %+v", req)
	}
}

func TestInitRequestLocalUsesShamir(t *testing.T) {
	req := initRequest(BootstrapOptions{Shares: 5, Threshold: 3})
	if req.SecretShares != 5 || req.SecretThreshold != 3 {
		t.Fatalf("shamir: %+v", req)
	}
	if req.RecoveryShares != 0 {
		t.Fatalf("recovery should be unset: %+v", req)
	}
}
