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
