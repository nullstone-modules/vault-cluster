package vaultcluster

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHelperToken(t *testing.T) {
	write := func(t *testing.T, path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("none", func(t *testing.T) {
		got, err := helperToken(t.TempDir(), "")
		if err != nil || got != "" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("token file", func(t *testing.T) {
		home := t.TempDir()
		write(t, filepath.Join(home, ".vault-token"), "not-a-real-token\n", 0o600)
		got, err := helperToken(home, "")
		if err != nil || got != "not-a-real-token" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("config without helper", func(t *testing.T) {
		home := t.TempDir()
		write(t, filepath.Join(home, ".vault"), "# no helper\n", 0o600)
		write(t, filepath.Join(home, ".vault-token"), "not-a-real-token", 0o600)
		got, err := helperToken(home, "")
		if err != nil || got != "not-a-real-token" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("token helper", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("shell helper")
		}
		home := t.TempDir()
		helper := filepath.Join(home, "helper.sh")
		write(t, helper, "#!/bin/sh\n[ \"$1\" = get ] && echo not-a-real-helper-token\n", 0o700)
		cfg := filepath.Join(home, "vault.hcl")
		write(t, cfg, `token_helper = "`+helper+`"`+"\n", 0o600)
		write(t, filepath.Join(home, ".vault-token"), "ignored", 0o600)
		got, err := helperToken(home, cfg)
		if err != nil || got != "not-a-real-helper-token" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}
