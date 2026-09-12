//go:build windows

package vaultcluster

// Ownership fixups only apply inside the Linux bootstrap container.
func chownToDirOwner(dir, path string) error {
	return nil
}
