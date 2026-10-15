package vaultcluster

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var tokenHelperLine = regexp.MustCompile(`(?m)^\s*token_helper\s*=\s*"([^"]+)"`)

// HelperToken returns the token `vault login` stored: the configured token_helper, else ~/.vault-token.
// It returns "" when there is none.
func HelperToken() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	return helperToken(home, os.Getenv("VAULT_CONFIG_PATH"))
}

func helperToken(home, configPath string) (string, error) {
	if configPath == "" {
		configPath = filepath.Join(home, ".vault")
	}
	if raw, err := os.ReadFile(configPath); err == nil {
		if m := tokenHelperLine.FindSubmatch(raw); m != nil {
			return runTokenHelper(string(m[1]))
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".vault-token"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func runTokenHelper(path string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(path, "get")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("token helper %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// StoreHelperToken saves a token where `vault login` would: the configured token_helper, else ~/.vault-token
// (mode 0600). The vault CLI then uses it without VAULT_TOKEN.
func StoreHelperToken(token string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return storeHelperToken(home, os.Getenv("VAULT_CONFIG_PATH"), token)
}

// EraseHelperToken removes the saved token: `helper erase`, else delete ~/.vault-token.
func EraseHelperToken() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return eraseHelperToken(home, os.Getenv("VAULT_CONFIG_PATH"))
}

func helperPath(home, configPath string) (string, error) {
	if configPath == "" {
		configPath = filepath.Join(home, ".vault")
	}
	raw, err := os.ReadFile(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if m := tokenHelperLine.FindSubmatch(raw); m != nil {
		return string(m[1]), nil
	}
	return "", nil
}

func storeHelperToken(home, configPath, token string) error {
	helper, err := helperPath(home, configPath)
	if err != nil {
		return err
	}
	if helper != "" {
		var stderr bytes.Buffer
		cmd := exec.Command(helper, "store")
		cmd.Stdin = strings.NewReader(token)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("token helper %s store: %w: %s", helper, err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	return os.WriteFile(filepath.Join(home, ".vault-token"), []byte(token), 0o600)
}

func eraseHelperToken(home, configPath string) error {
	helper, err := helperPath(home, configPath)
	if err != nil {
		return err
	}
	if helper != "" {
		var stderr bytes.Buffer
		cmd := exec.Command(helper, "erase")
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("token helper %s erase: %w: %s", helper, err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	err = os.Remove(filepath.Join(home, ".vault-token"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
