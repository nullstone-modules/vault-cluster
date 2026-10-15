package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/nullstone-modules/vault-cluster/internal/aws/secretsmanager"
	"github.com/nullstone-modules/vault-cluster/internal/nsenv"
	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

const breakGlassUsage = `usage: vault-utils break-glass --reason <text> --second <name> [cluster flags] <action> [args]

Cluster flags: --org --stack --env --block [--profile] [--internal], or none when VAULT_ADDR and the
VAULT_*_SECRET_ARN variables (AWS) or BOOTSTRAP_DIR (local) are set.

Actions (one per run):
  remint provisioning|apps-auth        Re-issue the platform token and store it
  policy write <name> <file>           Write an ACL policy (platform policies refused)
  userpass enable [--first-admin <n>]  Enable userpass; create the first admin (password from the terminal)
  exec -- <command> [args]             Run a command with the root token in its environment`

// breakGlassArgs is the parsed command line. Action words follow the flags; nothing in here is secret.
type breakGlassArgs struct {
	reason, second                  string
	org, stack, env, block, profile string
	internal                        bool
	action                          string
	actionArgs                      []string
}

func parseBreakGlassArgs(args []string) (breakGlassArgs, error) {
	var out breakGlassArgs
	fs := flag.NewFlagSet("break-glass", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&out.reason, "reason", "", "why root is needed (recorded)")
	fs.StringVar(&out.second, "second", "", "the second person (recorded)")
	fs.StringVar(&out.org, "org", "", "Nullstone org")
	fs.StringVar(&out.stack, "stack", "", "Nullstone stack")
	fs.StringVar(&out.env, "env", "", "Nullstone environment")
	fs.StringVar(&out.block, "block", "", "Vault cluster block")
	fs.StringVar(&out.profile, "profile", "", "nullstone CLI profile")
	fs.BoolVar(&out.internal, "internal", false, "use vault.internal instead of the user-facing name")
	if err := fs.Parse(args); err != nil {
		return out, fmt.Errorf("%w\n%s", err, breakGlassUsage)
	}
	if strings.TrimSpace(out.reason) == "" || strings.TrimSpace(out.second) == "" {
		return out, fmt.Errorf("--reason and --second are required\n%s", breakGlassUsage)
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return out, fmt.Errorf("an action is required\n%s", breakGlassUsage)
	}
	out.action, out.actionArgs = rest[0], rest[1:]
	if (out.stack != "" || out.env != "" || out.block != "") && (out.stack == "" || out.env == "" || out.block == "") {
		return out, fmt.Errorf("--stack, --env, and --block go together\n%s", breakGlassUsage)
	}
	return out, nil
}

func (a breakGlassArgs) usesNullstone() bool { return a.stack != "" }

// buildAction turns the action words into an Action. Everything it can refuse without a cluster, it refuses here.
func buildAction(a breakGlassArgs, bg vaultcluster.BreakGlass, readPassword func() (string, error)) (vaultcluster.Action, error) {
	args := a.actionArgs
	switch a.action {
	case "remint":
		if len(args) != 1 {
			return nil, fmt.Errorf("usage: break-glass ... remint provisioning|apps-auth")
		}
		act := vaultcluster.RemintAction{Token: args[0], BG: bg}
		return act, act.Check()
	case "policy":
		if len(args) != 3 || args[0] != "write" {
			return nil, fmt.Errorf("usage: break-glass ... policy write <name> <file>")
		}
		hcl, err := os.ReadFile(args[2])
		if err != nil {
			return nil, err
		}
		act := &vaultcluster.PolicyWriteAction{Policy: args[1], HCL: string(hcl), Logf: bg.Logf}
		return act, act.Check()
	case "userpass":
		if len(args) < 1 || args[0] != "enable" {
			return nil, fmt.Errorf("usage: break-glass ... userpass enable [--first-admin <name>]")
		}
		act := vaultcluster.UserpassAction{Logf: bg.Logf, ReadPassword: readPassword}
		switch {
		case len(args) == 1:
		case len(args) == 3 && args[1] == "--first-admin" && args[2] != "":
			act.FirstAdmin = args[2]
		case len(args) == 2 && strings.HasPrefix(args[1], "--first-admin=") && len(args[1]) > len("--first-admin="):
			act.FirstAdmin = strings.TrimPrefix(args[1], "--first-admin=")
		default:
			return nil, fmt.Errorf("usage: break-glass ... userpass enable [--first-admin <name>]")
		}
		return act, nil
	case "exec":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: break-glass ... exec -- <command> [args]")
		}
		return vaultcluster.ExecAction{Argv: args, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, nil
	default:
		return nil, fmt.Errorf("unknown action %q\n%s", a.action, breakGlassUsage)
	}
}

func runBreakGlass(args []string) error {
	a, err := parseBreakGlassArgs(args)
	if err != nil {
		return err
	}
	// Refuse a bad action before any network call.
	if _, err := buildAction(a, vaultcluster.BreakGlass{}, nil); err != nil {
		return err
	}
	cfg, store, err := resolveBreakGlassTarget(context.Background(), a)
	if err != nil {
		return err
	}
	bg := vaultcluster.BreakGlass{Store: store, Reason: a.reason, Second: a.second, Logf: log.Printf}
	action, err := buildAction(a, bg, readPasswordFromTerminal)
	if err != nil {
		return err
	}
	c, err := vaultcluster.New(cfg)
	if err != nil {
		return err
	}
	return bg.Run(c, action)
}

// resolveBreakGlassTarget finds the cluster address and key store. With --stack/--env/--block it reads the
// Nullstone workspace outputs; otherwise VAULT_ADDR plus the secret ARNs (AWS) or BOOTSTRAP_DIR (local).
// VAULT_TOKEN is ignored: break-glass always starts from the stored operator token.
func resolveBreakGlassTarget(ctx context.Context, a breakGlassArgs) (vaultcluster.Config, vaultcluster.KeyStore, error) {
	cfg := vaultcluster.ConfigFromEnv()
	cfg.Token = ""
	if !a.usesNullstone() {
		if cfg.Addr == "" {
			return cfg, nil, fmt.Errorf("set VAULT_ADDR, or pass --org --stack --env --block")
		}
		store, err := keyStore()
		return cfg, store, err
	}
	p, err := nsenv.LoadProfile(a.profile)
	if err != nil {
		return cfg, nil, err
	}
	apiCfg, err := p.APIConfig(a.org)
	if err != nil {
		return cfg, nil, err
	}
	ws, err := nsenv.APIResolver{Config: apiCfg}.Workspace(ctx, a.stack, a.env, a.block)
	if err != nil {
		return cfg, nil, err
	}
	if err := nsenv.CheckContract(ws); err != nil {
		return cfg, nil, err
	}
	s, err := nsenv.SettingsFromOutputs(ws, a.internal)
	if err != nil {
		return cfg, nil, err
	}
	if s.Note != "" {
		log.Printf("note: %s", s.Note)
	}
	cfg.Addr, cfg.TLSServerName, cfg.SharedEnvs = s.Addr, s.TLSServerName, s.SharedEnvs
	arns, err := secretARNsFromOutputs(ws.Outputs)
	if err != nil {
		return cfg, nil, err
	}
	store, err := breakGlassKeyStore(ctx, a.block, arns)
	return cfg, store, err
}

// secretARNs are the platform secrets by token name, as the cluster workspace exports them.
type secretARNs struct {
	init, provisioning, operator, appsAuth string
}

func secretARNsFromOutputs(out map[string]any) (secretARNs, error) {
	str := func(k string) string {
		v, _ := out[k].(string)
		return strings.TrimSpace(v)
	}
	arns := secretARNs{
		init:         str("init_secret_arn"),
		provisioning: str("provisioning_secret_arn"),
		operator:     str("operator_secret_arn"),
		appsAuth:     str("apps_auth_secret_arn"),
	}
	if arns.operator == "" || arns.provisioning == "" {
		return arns, fmt.Errorf("workspace has no operator_secret_arn / provisioning_secret_arn output; is it an AWS cluster that has been applied?")
	}
	return arns, nil
}

// breakGlassKeyStore builds the Secrets Manager store. Clusters applied before the module exported
// init_secret_arn and apps_auth_secret_arn are found by name prefix (<block>/vault/<name>/) in the
// operator secret's region.
func breakGlassKeyStore(ctx context.Context, block string, arns secretARNs) (vaultcluster.KeyStore, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("AWS credentials: %w", err)
	}
	region, err := secretsmanager.RegionOf(arns.operator)
	if err != nil {
		return nil, err
	}
	lister := secretsmanager.NewLister(awsCfg, region)
	if arns.init == "" {
		if arns.init, err = secretsmanager.FindByPrefix(ctx, lister, block+"/vault/init/"); err != nil {
			return nil, fmt.Errorf("init secret (apply the cluster to export init_secret_arn): %w", err)
		}
		log.Printf("init secret found by name: %s", arns.init)
	}
	if arns.appsAuth == "" {
		if arns.appsAuth, err = secretsmanager.FindByPrefix(ctx, lister, block+"/vault/apps-auth/"); err != nil {
			log.Printf("apps-auth secret not found by name; remint apps-auth needs apps_auth_secret_arn: %v", err)
		}
	}
	return &secretsmanager.KeyStore{
		Secrets:         secretsmanager.NewClient(awsCfg),
		InitARN:         arns.init,
		ProvisioningARN: arns.provisioning,
		OperatorARN:     arns.operator,
		AppsAuthARN:     arns.appsAuth,
	}, nil
}

func readPasswordFromTerminal() (string, error) {
	return vaultcluster.ReadPasswordFromTerminal(os.Stdin, os.Stderr)
}
