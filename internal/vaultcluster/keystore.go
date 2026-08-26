package vaultcluster

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/hashicorp/vault/api"
)

type KeyStore interface {
	SaveInit(resp *api.InitResponse) error
	LoadInit() (*api.InitResponse, error)
	SaveToken(name, token string) error
	LoadToken(name string) (string, error)
}

type FileKeyStore struct {
	Dir string
}

func (s FileKeyStore) SaveInit(resp *api.InitResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return s.write(filepath.Join(s.Dir, "vault-init.json"), b)
}

func (s FileKeyStore) LoadInit() (*api.InitResponse, error) {
	raw, err := os.ReadFile(filepath.Join(s.Dir, "vault-init.json"))
	if err != nil {
		return nil, err
	}
	var resp api.InitResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s FileKeyStore) SaveToken(name, token string) error {
	return s.write(filepath.Join(s.Dir, name+".token"), []byte(token))
}

func (s FileKeyStore) LoadToken(name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, name+".token"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s FileKeyStore) write(path string, b []byte) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return chownToDirOwner(s.Dir, path)
}

func chownToDirOwner(dir, path string) error {
	if os.Getuid() != 0 {
		return nil
	}
	uid, gid := 0, 0
	if fi, err := os.Stat(dir); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			uid = int(st.Uid)
			gid = int(st.Gid)
		}
	}
	if v := os.Getenv("BOOTSTRAP_UID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			uid = n
		}
	}
	if v := os.Getenv("BOOTSTRAP_GID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			gid = n
		}
	}
	_ = os.Chown(dir, uid, gid)
	return os.Chown(path, uid, gid)
}
