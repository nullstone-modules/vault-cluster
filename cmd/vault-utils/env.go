package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/nullstone-modules/vault-cluster/internal/nsenv"
)

const reachTimeout = 3 * time.Second

func runEnv(args []string) error {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	org := fs.String("org", "", "Nullstone org (default: profile org or NULLSTONE_ORG)")
	stack := fs.String("stack", "", "Nullstone stack")
	env := fs.String("env", "", "Nullstone environment")
	block := fs.String("block", "", "Vault cluster block")
	profile := fs.String("profile", "", "nullstone CLI profile (default: NULLSTONE_PROFILE or default)")
	shell := fs.String("shell", defaultShell(), "bash, zsh, fish, or powershell")
	internal := fs.Bool("internal", false, "Use vault.internal instead of the user-facing name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stack == "" || *env == "" || *block == "" {
		return fmt.Errorf("usage: vault-utils env --org <org> --stack <stack> --env <env> --block <block> [--profile <p>] [--shell bash|zsh|fish|powershell] [--internal]")
	}
	p, err := nsenv.LoadProfile(*profile)
	if err != nil {
		return err
	}
	cfg, err := p.APIConfig(*org)
	if err != nil {
		return err
	}
	ctx := context.Background()
	ws, err := nsenv.APIResolver{Config: cfg}.Workspace(ctx, *stack, *env, *block)
	if err != nil {
		return err
	}
	return printEnv(ctx, os.Stdout, os.Stderr, ws, *shell, *internal)
}

func printEnv(ctx context.Context, stdout, stderr io.Writer, ws nsenv.Workspace, shell string, internal bool) error {
	if err := nsenv.CheckContract(ws); err != nil {
		return err
	}
	s, err := nsenv.SettingsFromOutputs(ws, internal)
	if err != nil {
		return err
	}
	out, err := nsenv.Render(shell, s)
	if err != nil {
		return err
	}
	if s.Note != "" {
		fmt.Fprintf(stderr, "note: %s\n", s.Note)
	}
	if err := nsenv.CheckReachable(ctx, s, reachTimeout); err != nil {
		fmt.Fprintf(stderr, "warning: %s is unreachable (%v).\n", s.Addr, err)
		fmt.Fprintf(stderr, "warning: the Vault load balancer is private to the cluster network; connect through a VPN, a Tailscale subnet router, or similar.\n")
	}
	_, err = io.WriteString(stdout, out)
	return err
}

func defaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}
