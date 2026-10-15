package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/nullstone-modules/vault-cluster/internal/aws/secretsmanager"
	"github.com/nullstone-modules/vault-cluster/internal/nsenv"
	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

const breakGlassUsage = `usage: vault-utils break-glass begin --reason <text> --second <name> [cluster flags]
       vault-utils break-glass end [cluster flags]

begin mints a root token from the operator token and the stored recovery shares and saves it where
"vault login" would (the token helper, else ~/.vault-token). end revokes it and removes it.

Cluster flags: --org --stack --env --block [--profile] [--internal], or none when VAULT_ADDR and the
VAULT_*_SECRET_ARN variables (AWS) or BOOTSTRAP_DIR (local) are set.`

// breakGlassArgs is the parsed command line. Nothing in here is secret.
type breakGlassArgs struct {
	sub                             string
	reason, second                  string
	org, stack, env, block, profile string
	internal                        bool
}

func parseBreakGlassArgs(args []string) (breakGlassArgs, error) {
	var out breakGlassArgs
	if len(args) == 0 {
		return out, fmt.Errorf("begin or end is required\n%s", breakGlassUsage)
	}
	out.sub = args[0]
	if out.sub != "begin" && out.sub != "end" {
		return out, fmt.Errorf("unknown subcommand %q\n%s", out.sub, breakGlassUsage)
	}
	fs := flag.NewFlagSet("break-glass "+out.sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&out.reason, "reason", "", "why root is needed (recorded)")
	fs.StringVar(&out.second, "second", "", "the second person (recorded)")
	fs.StringVar(&out.org, "org", "", "Nullstone org")
	fs.StringVar(&out.stack, "stack", "", "Nullstone stack")
	fs.StringVar(&out.env, "env", "", "Nullstone environment")
	fs.StringVar(&out.block, "block", "", "Vault cluster block")
	fs.StringVar(&out.profile, "profile", "", "nullstone CLI profile")
	fs.BoolVar(&out.internal, "internal", false, "use vault.internal instead of the user-facing name")
	if err := fs.Parse(args[1:]); err != nil {
		return out, fmt.Errorf("%w\n%s", err, breakGlassUsage)
	}
	if fs.NArg() > 0 {
		return out, fmt.Errorf("unexpected argument %q\n%s", fs.Arg(0), breakGlassUsage)
	}
	if out.sub == "begin" && (strings.TrimSpace(out.reason) == "" || strings.TrimSpace(out.second) == "") {
		return out, fmt.Errorf("--reason and --second are required\n%s", breakGlassUsage)
	}
	if (out.stack != "" || out.env != "" || out.block != "") && (out.stack == "" || out.env == "" || out.block == "") {
		return out, fmt.Errorf("--stack, --env, and --block go together\n%s", breakGlassUsage)
	}
	return out, nil
}

func (a breakGlassArgs) usesNullstone() bool { return a.stack != "" }

func runBreakGlass(args []string) error {
	a, err := parseBreakGlassArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if a.sub == "end" {
		cfg, _, err := resolveBreakGlassTarget(ctx, a, false)
		if err != nil {
			return err
		}
		return breakGlassEnd(cfg)
	}
	cfg, store, err := resolveBreakGlassTarget(ctx, a, true)
	if err != nil {
		return err
	}
	return breakGlassBegin(cfg, store, a)
}

func breakGlassBegin(cfg vaultcluster.Config, store vaultcluster.KeyStore, a breakGlassArgs) error {
	c, err := vaultcluster.New(cfg)
	if err != nil {
		return err
	}
	if saved, err := vaultcluster.HelperToken(); err != nil {
		return err
	} else if saved != "" {
		sec, err := c.WithToken(saved).API.Auth().Token().LookupSelf()
		if err == nil && vaultcluster.IsRootToken(sec) {
			return fmt.Errorf("a break-glass root token is already saved; run `vault-utils break-glass end` first")
		}
		log.Printf("replacing the saved vault login token (it is not revoked)")
	}
	bg := vaultcluster.BreakGlass{Store: store, Reason: a.reason, Second: a.second, Logf: log.Printf}
	token, err := bg.Begin(c)
	if err != nil {
		return err
	}
	if err := vaultcluster.StoreHelperToken(token); err != nil {
		if rerr := c.WithToken(token).API.Auth().Token().RevokeSelf(""); rerr != nil {
			return fmt.Errorf("saving the root token failed (%v) and revoking it failed too: %w", err, rerr)
		}
		return fmt.Errorf("saving the root token failed; it was revoked: %w", err)
	}
	log.Printf("root token saved for the vault CLI (unset VAULT_TOKEN to use it); revoke with `vault-utils break-glass end`")
	return nil
}

func breakGlassEnd(cfg vaultcluster.Config) error {
	c, err := vaultcluster.New(cfg)
	if err != nil {
		return err
	}
	token, err := vaultcluster.HelperToken()
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("no saved token; nothing to revoke")
	}
	if err := vaultcluster.End(c, token, log.Printf); err != nil {
		return err
	}
	return vaultcluster.EraseHelperToken()
}

// resolveBreakGlassTarget finds the cluster address and, for begin, the key store. With --stack/--env/--block
// it reads the Nullstone workspace outputs; otherwise VAULT_ADDR plus the secret ARNs (AWS) or BOOTSTRAP_DIR
// (local). VAULT_TOKEN is ignored: begin starts from the stored operator token, end from the saved root.
func resolveBreakGlassTarget(ctx context.Context, a breakGlassArgs, needStore bool) (vaultcluster.Config, vaultcluster.KeyStore, error) {
	cfg := vaultcluster.ConfigFromEnv()
	cfg.Token = ""
	if !a.usesNullstone() {
		if cfg.Addr == "" {
			return cfg, nil, fmt.Errorf("set VAULT_ADDR, or pass --org --stack --env --block")
		}
		if !needStore {
			return cfg, nil, nil
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
	if !needStore {
		return cfg, nil, nil
	}
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

// breakGlassKeyStore builds the Secrets Manager store. A cluster applied before the module exported
// init_secret_arn is found by name prefix (<block>/vault/init/) in the operator secret's region.
func breakGlassKeyStore(ctx context.Context, block string, arns secretARNs) (vaultcluster.KeyStore, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("AWS credentials: %w", err)
	}
	if arns.init == "" {
		region, err := secretsmanager.RegionOf(arns.operator)
		if err != nil {
			return nil, err
		}
		arns.init, err = secretsmanager.FindByPrefix(ctx, secretsmanager.NewLister(awsCfg, region), block+"/vault/init/")
		if err != nil {
			return nil, fmt.Errorf("init secret (apply the cluster to export init_secret_arn): %w", err)
		}
		log.Printf("init secret found by name: %s", arns.init)
	}
	return &secretsmanager.KeyStore{
		Secrets:         secretsmanager.NewClient(awsCfg),
		InitARN:         arns.init,
		ProvisioningARN: arns.provisioning,
		OperatorARN:     arns.operator,
		AppsAuthARN:     arns.appsAuth,
	}, nil
}
