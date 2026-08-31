//go:build !windows

package vaultcluster

import (
	"os"
	"strconv"
	"syscall"
)

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
