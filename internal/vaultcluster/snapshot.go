package vaultcluster

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (c *Client) SnapshotTake(dir string) (string, error) {
	req := c.API.NewRequest("GET", "/v1/sys/storage/raft/snapshot")
	resp, err := c.API.RawRequest(req)
	if err != nil {
		return "", fmt.Errorf("snapshot failed: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", fmt.Errorf("snapshot is empty; refusing to keep it")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	file := filepath.Join(dir, "vault-"+stamp+".snap")
	if err := os.WriteFile(file, b, 0o600); err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	if err := os.WriteFile(file+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
		return "", err
	}
	_ = chownToDirOwner(dir, file)
	_ = chownToDirOwner(dir, file+".sha256")
	return file, nil
}

func SnapshotVerify(file string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	want, err := os.ReadFile(file + ".sha256")
	if err != nil {
		return fmt.Errorf("no checksum beside %s; integrity cannot be established", file)
	}
	sum := sha256.Sum256(b)
	got := hex.EncodeToString(sum[:])
	if got != strings.TrimSpace(string(want)) {
		return fmt.Errorf("checksum mismatch for %s; this snapshot is corrupt and must not be restored", file)
	}
	return nil
}

func SnapshotList(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".snap") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

func (c *Client) SnapshotRestore(file string) error {
	if _, err := os.Stat(file + ".sha256"); err == nil {
		if err := SnapshotVerify(file); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	req := c.API.NewRequest("POST", "/v1/sys/storage/raft/snapshot-force")
	req.BodyBytes = b
	resp, err := c.API.RawRequest(req)
	if err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("restore failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
