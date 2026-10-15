package vaultcluster

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ReadPasswordFromTerminal reads one line from in without echo. It refuses anything that is not a terminal:
// a pipe or a here-string would carry the password through a shell, and PowerShell 5.1 prefixes a BOM.
// The prompt goes to prompt (stderr), never stdout.
func ReadPasswordFromTerminal(in *os.File, prompt io.Writer) (string, error) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("the password must be typed at a terminal; stdin is not one")
	}
	fmt.Fprint(prompt, "Password for the first admin (not echoed): ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(prompt)
	if err != nil {
		return "", err
	}
	return TrimPassword(string(b)), nil
}
