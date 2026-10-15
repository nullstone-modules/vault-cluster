package vaultcluster

import (
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

const runtimeGOOS = runtime.GOOS

// pipedPasswordReader feeds password through an OS pipe, which ReadPasswordFromTerminal must refuse.
func pipedPasswordReader(t *testing.T, password string) func() (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if _, err := io.WriteString(w, password); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return func() (string, error) { return ReadPasswordFromTerminal(r, io.Discard) }
}

func TestReadPasswordRefusesPipe(t *testing.T) {
	_, err := pipedPasswordReader(t, "hunter2\n")()
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("piped password accepted: %v", err)
	}
}

func TestReadPasswordFromTerminal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("no pty on %s: terminal password read not exercised", runtime.GOOS)
	}
	got, err := terminalPasswordReader(t, "hunter2")()
	if err != nil || got != "hunter2" {
		t.Fatalf("got %q, %v", got, err)
	}
}
