//go:build linux

package vaultcluster

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// terminalPasswordReader types password on a real pseudo-terminal and reads it back through
// ReadPasswordFromTerminal, so the no-echo terminal path is exercised end to end.
func terminalPasswordReader(t *testing.T, password string) func() (string, error) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pty on this host (%v); the terminal password path is not exercised here", err)
	}
	t.Cleanup(func() { master.Close() })
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("pty unlock failed (%v); the terminal password path is not exercised here", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Skipf("pty number (%v); the terminal password path is not exercised here", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("pty slave (%v); the terminal password path is not exercised here", err)
	}
	t.Cleanup(func() { slave.Close() })
	return func() (string, error) {
		if _, err := io.WriteString(master, password+"\n"); err != nil {
			return "", err
		}
		return ReadPasswordFromTerminal(slave, io.Discard)
	}
}
