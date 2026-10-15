//go:build !linux

package vaultcluster

import "testing"

// terminalPasswordReader stands in for a pty where the test cannot open one: the Vault side of the action
// is still exercised, the no-echo terminal read is not (it is on Linux, see breakglass_pty_linux_test.go).
func terminalPasswordReader(t *testing.T, password string) func() (string, error) {
	t.Helper()
	t.Logf("no pty on %s: terminal password read skipped, reader injected", runtimeGOOS)
	return func() (string, error) { return password + "\n", nil }
}
