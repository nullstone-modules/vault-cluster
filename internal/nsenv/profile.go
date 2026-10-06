// Package nsenv resolves a Nullstone Vault cluster workspace into Vault CLI settings.
package nsenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	api "gopkg.in/nullstone-io/go-api-client.v0"
	"gopkg.in/nullstone-io/go-api-client.v0/auth"
)

// Profile follows the nullstone CLI: ~/.nullstone/<name>/{config,key,org}, overridden by
// NULLSTONE_API_KEY, NULLSTONE_ORG, and NULLSTONE_ADDR.
type Profile struct {
	Name    string
	Address string
	Org     string
	apiKey  string
}

func LoadProfile(name string) (Profile, error) {
	if name == "" {
		name = os.Getenv("NULLSTONE_PROFILE")
	}
	if name == "" {
		name = "default"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Profile{}, err
	}
	return loadProfile(filepath.Join(home, ".nullstone", name), name)
}

func loadProfile(dir, name string) (Profile, error) {
	p := Profile{Name: name, Address: api.DefaultAddress}
	if raw, err := os.ReadFile(filepath.Join(dir, "config")); err == nil {
		var cfg struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return p, fmt.Errorf("invalid nullstone profile %s: %w", name, err)
		}
		if cfg.Address != "" {
			p.Address = cfg.Address
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return p, err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "key")); err == nil {
		p.apiKey = strings.TrimSpace(string(raw))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return p, err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "org")); err == nil {
		p.Org = strings.TrimSpace(string(raw))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return p, err
	}
	if v := os.Getenv("NULLSTONE_ADDR"); v != "" {
		p.Address = v
	}
	if v := os.Getenv("NULLSTONE_API_KEY"); v != "" {
		p.apiKey = strings.TrimSpace(v)
	}
	if v := os.Getenv("NULLSTONE_ORG"); v != "" {
		p.Org = v
	}
	return p, nil
}

// APIConfig never exposes the key except to the client's auth header.
func (p Profile) APIConfig(org string) (api.Config, error) {
	if org == "" {
		org = p.Org
	}
	if org == "" {
		return api.Config{}, fmt.Errorf("no nullstone org: pass --org or set NULLSTONE_ORG")
	}
	if p.apiKey == "" {
		return api.Config{}, fmt.Errorf("no nullstone api key: run `nullstone set-profile` or set NULLSTONE_API_KEY")
	}
	return api.Config{
		BaseAddress:       p.Address,
		OrgName:           org,
		AccessTokenSource: auth.RawAccessTokenSource{AccessToken: p.apiKey},
	}, nil
}
