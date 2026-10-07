package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/nullstone-modules/vault-cluster/internal/aws/asg"
	"github.com/nullstone-modules/vault-cluster/internal/aws/s3"
	"github.com/nullstone-modules/vault-cluster/internal/aws/secretsmanager"
	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

// Well inside the 24h token period, so a run of failed renewals is survivable.
const tokenRenewInterval = time.Hour

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `vault-utils <command> [args]

Commands:
  bootstrap local|aws|azure|gcp     Init once, unseal, configure
  tenants create <id> [--env <env>]
  tenants list [--env <env>]
  tenants destroy <id> [--env <env>] --yes [--purge-secrets]
  envs list                         Envs with tenants (shared cluster only)
  envs destroy <env> --yes [--purge-secrets]
  snapshot take                     Write a Raft snapshot
  snapshot list
  snapshot verify <file|s3-uri>
  snapshot restore <file|s3-uri> --yes
  snapshot schedule                 Cron loop (BACKUP_SCHEDULE; empty disables)
  health                            Print seal status
  health serve                      HTTP on :8210 (200 only if this node is a Raft voter and caught up)
  lifecycle                         ASG node: continue the launch hook once a caught-up voter; leave Raft before termination
  env --org --stack --env --block   Print shell settings for a Nullstone Vault cluster workspace

Without VAULT_TOKEN, tenants, envs, snapshot take|restore, and health use the token from "vault login".
On a shared cluster (SHARED_ENVS=true, printed by "vault-utils env") every tenant belongs to an env: --env is
required there and refused elsewhere.

Local key material: BOOTSTRAP_DIR (default .bootstrap).
AWS: VAULT_INIT_SECRET_ARN, VAULT_PROVISIONING_SECRET_ARN, VAULT_OPERATOR_SECRET_ARN.
Optional: SNAPSHOT_BUCKET, SNAPSHOT_PREFIX (default vault-snapshots).
`)
}

func run(cmd string, args []string) error {
	if cmd == "env" {
		return runEnv(args)
	}
	cfg := vaultcluster.ConfigFromEnv()
	if cfg.Token == "" && usesLoginToken(cmd, args) {
		tok, err := vaultcluster.HelperToken()
		if err != nil {
			return err
		}
		cfg.Token = tok
	}
	c, err := vaultcluster.New(cfg)
	if err != nil {
		return err
	}
	switch cmd {
	case "bootstrap":
		return runBootstrap(c, args)
	case "tenants":
		return runTenants(c, args)
	case "envs":
		return runEnvs(c, args)
	case "snapshot":
		return runSnapshot(c, args)
	case "health":
		if len(args) > 0 && args[0] == "serve" {
			return runHealthServe(c)
		}
		return c.Health()
	case "lifecycle":
		return runLifecycle(c)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func runBootstrap(c *vaultcluster.Client, args []string) error {
	platform := ""
	if len(args) > 0 {
		platform = args[0]
	}
	if platform == "" {
		platform = os.Getenv("VAULT_PLATFORM")
	}
	if platform == "" {
		return fmt.Errorf("usage: vault-utils bootstrap local|aws|azure|gcp")
	}
	switch platform {
	case "local":
		shares, _ := strconv.Atoi(getenv("VAULT_INIT_KEY_SHARES", "5"))
		threshold, _ := strconv.Atoi(getenv("VAULT_INIT_KEY_THRESHOLD", "3"))
		return c.RunBootstrap(fileKeyStore(), vaultcluster.BootstrapOptions{
			Shares:    shares,
			Threshold: threshold,
			KeepRoot:  getenv("KEEP_ROOT", "false") == "true",
		})
	case "aws":
		store, err := awsKeyStore()
		if err != nil {
			return err
		}
		shares, _ := strconv.Atoi(getenv("VAULT_INIT_RECOVERY_SHARES", "1"))
		threshold, _ := strconv.Atoi(getenv("VAULT_INIT_RECOVERY_THRESHOLD", "1"))
		return c.RunBootstrap(store, vaultcluster.BootstrapOptions{
			Shares:     shares,
			Threshold:  threshold,
			KeepRoot:   getenv("KEEP_ROOT", "false") == "true",
			AutoUnseal: true,
			ClaimInit:  awsClaimInit,
		})
	case "azure", "gcp":
		return fmt.Errorf("bootstrap %s is not implemented yet", platform)
	default:
		return fmt.Errorf("unknown platform %q (local, aws, azure, gcp)", platform)
	}
}

// tenantArgs parses <id> plus --env <env>, --yes, --purge-secrets. Any other flag is an error.
type tenantArgs struct {
	id, env    string
	yes, purge bool
}

func parseTenantArgs(args []string) (tenantArgs, error) {
	var out tenantArgs
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--yes":
			out.yes = true
		case a == "--purge-secrets":
			out.purge = true
		case a == "--env":
			if i+1 >= len(args) {
				return out, fmt.Errorf("--env needs a value")
			}
			i++
			out.env = args[i]
		case strings.HasPrefix(a, "--env="):
			out.env = strings.TrimPrefix(a, "--env=")
		case strings.HasPrefix(a, "-"):
			return out, fmt.Errorf("unknown flag %q", a)
		default:
			out.id = a
		}
	}
	return out, nil
}

// checkEnvFlag enforces the cluster mode before any Vault call: --env is required on a shared cluster
// (SHARED_ENVS=true) and refused on an unshared one.
func checkEnvFlag(shared bool, env string) error {
	if shared && env == "" {
		return fmt.Errorf("this cluster is shared across envs; pass --env <env>")
	}
	if !shared && env != "" {
		return fmt.Errorf("this cluster is not shared across envs (SHARED_ENVS is not true); drop --env")
	}
	return nil
}

func runTenants(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vault-utils tenants create|list|destroy")
	}
	sub, rest := args[0], args[1:]
	ta, err := parseTenantArgs(rest)
	if err != nil {
		return err
	}
	switch sub {
	case "create":
		if ta.id == "" {
			return fmt.Errorf("usage: vault-utils tenants create <id> [--env <env>]")
		}
		if err := checkEnvFlag(c.Cfg.SharedEnvs, ta.env); err != nil {
			return err
		}
		return c.CreateTenant(ta.env, ta.id)
	case "list":
		tenants, err := c.ListTenants()
		if err != nil {
			return err
		}
		if ta.env != "" {
			var kept []vaultcluster.Tenant
			for _, t := range tenants {
				if t.Env == ta.env {
					kept = append(kept, t)
				}
			}
			tenants = kept
		}
		vaultcluster.PrintTenants(os.Stdout, tenants, c.Cfg.SharedEnvs)
		return nil
	case "destroy":
		if ta.id == "" || !ta.yes {
			return fmt.Errorf("usage: vault-utils tenants destroy <id> [--env <env>] --yes [--purge-secrets]")
		}
		if err := checkEnvFlag(c.Cfg.SharedEnvs, ta.env); err != nil {
			return err
		}
		return c.OffboardTenant(ta.env, ta.id, ta.purge)
	default:
		return fmt.Errorf("unknown subcommand %q (create, list, destroy)", sub)
	}
}

func runEnvs(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vault-utils envs list | destroy <env> --yes [--purge-secrets]")
	}
	sub, rest := args[0], args[1:]
	ta, err := parseTenantArgs(rest)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		envs, err := c.ListEnvs()
		if err != nil {
			return err
		}
		if len(envs) == 0 {
			fmt.Println("no envs")
			return nil
		}
		for _, e := range envs {
			fmt.Println(e)
		}
		return nil
	case "destroy":
		if ta.id == "" || !ta.yes {
			return fmt.Errorf("usage: vault-utils envs destroy <env> --yes [--purge-secrets]")
		}
		return c.DestroyEnv(ta.id, ta.purge)
	default:
		return fmt.Errorf("unknown subcommand %q (list, destroy)", sub)
	}
}

func runSnapshot(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vault-utils snapshot take | list | verify <file|s3-uri> | restore <file|s3-uri> --yes | schedule")
	}
	backupDir := filepath.Join(bootstrapDir(), "backups")
	switch args[0] {
	case "take":
		if err := useOperatorToken(c); err != nil {
			return err
		}
		file, err := takeSnapshot(c, backupDir)
		if err != nil {
			return err
		}
		log.Printf("snapshot written: %s", file)
		log.Printf("this file contains every secret in the cluster; treat it as one")
		return nil
	case "list":
		files, err := listSnapshots(backupDir)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			log.Printf("no snapshots")
			return nil
		}
		for _, f := range files {
			fmt.Println(f)
		}
		return nil
	case "verify":
		if len(args) < 2 {
			return fmt.Errorf("usage: vault-utils snapshot verify <file|s3-uri>")
		}
		if err := verifySnapshot(args[1]); err != nil {
			return err
		}
		log.Printf("checksum OK: %s", args[1])
		return nil
	case "restore":
		if len(args) < 3 || args[2] != "--yes" {
			return fmt.Errorf("restore replaces the entire cluster; re-run with: vault-utils snapshot restore <file|s3-uri> --yes")
		}
		if c.Cfg.Token == "" {
			return fmt.Errorf("restore requires VAULT_TOKEN with sys/storage/raft/snapshot-force (break-glass root); the operator token cannot restore")
		}
		if err := restoreSnapshot(c, args[1]); err != nil {
			return err
		}
		log.Printf("restore submitted; Vault will seal")
		if os.Getenv("VAULT_PLATFORM") == "aws" {
			log.Printf("KMS auto-unseal should bring the node back")
		} else {
			log.Printf("unseal with the key shares that were current when this snapshot was taken")
		}
		return nil
	case "schedule":
		return runSnapshotSchedule(c, backupDir)
	default:
		return fmt.Errorf("unknown subcommand %q (take, list, verify, restore, schedule)", args[0])
	}
}

func runSnapshotSchedule(c *vaultcluster.Client, backupDir string) error {
	sched, err := vaultcluster.ParseBackupSchedule(os.Getenv("BACKUP_SCHEDULE"))
	if err != nil {
		return err
	}
	if sched == nil {
		log.Printf("scheduled snapshots disabled")
		return nil
	}
	if err := useOperatorToken(c); err != nil {
		return err
	}
	go c.RenewToken(tokenRenewInterval, nil)
	nodeID := os.Getenv("VAULT_RAFT_NODE_ID")
	for {
		wait := time.Until(sched.Next(time.Now()))
		if wait > 0 {
			time.Sleep(wait)
		}
		if nodeID != "" {
			data, err := c.RaftAutopilot()
			if err != nil {
				log.Printf("snapshot skipped: %v", err)
				continue
			}
			if !vaultcluster.NodeIsLeader(nodeID, data) {
				log.Printf("snapshot skipped: not raft leader")
				continue
			}
		}
		file, err := takeSnapshot(c, backupDir)
		if err != nil {
			log.Printf("snapshot failed: %v", err)
			continue
		}
		log.Printf("snapshot written: %s", file)
	}
}

func runHealthServe(c *vaultcluster.Client) error {
	nodeID := os.Getenv("VAULT_RAFT_NODE_ID")
	if nodeID == "" {
		return fmt.Errorf("VAULT_RAFT_NODE_ID is required for health serve")
	}
	if err := useOperatorToken(c); err != nil {
		return err
	}
	go c.RenewToken(tokenRenewInterval, nil)
	addr := getenv("VAULT_HEALTH_ADDR", ":8210")
	log.Printf("health listening on %s", addr)
	return c.ServeHealth(addr, nodeID)
}

func takeSnapshot(c *vaultcluster.Client, backupDir string) (string, error) {
	if bucket := os.Getenv("SNAPSHOT_BUCKET"); bucket != "" {
		store, err := s3.New()
		if err != nil {
			return "", err
		}
		b, err := c.RaftSnapshot()
		if err != nil {
			return "", err
		}
		return s3.PutSnapshot(store, bucket, getenv("SNAPSHOT_PREFIX", "vault-snapshots"), b)
	}
	return c.SnapshotTake(backupDir)
}

func listSnapshots(backupDir string) ([]string, error) {
	if bucket := os.Getenv("SNAPSHOT_BUCKET"); bucket != "" {
		store, err := s3.New()
		if err != nil {
			return nil, err
		}
		return s3.ListSnapshots(store, bucket, getenv("SNAPSHOT_PREFIX", "vault-snapshots"))
	}
	return vaultcluster.SnapshotList(backupDir)
}

func verifySnapshot(ref string) error {
	if strings.HasPrefix(ref, "s3://") {
		store, err := s3.New()
		if err != nil {
			return err
		}
		_, err = s3.GetSnapshot(store, ref)
		return err
	}
	return vaultcluster.SnapshotVerify(ref)
}

func restoreSnapshot(c *vaultcluster.Client, ref string) error {
	if strings.HasPrefix(ref, "s3://") {
		store, err := s3.New()
		if err != nil {
			return err
		}
		b, err := s3.GetSnapshot(store, ref)
		if err != nil {
			return err
		}
		return c.SnapshotRestoreData(b)
	}
	return c.SnapshotRestore(ref)
}

// usesLoginToken lists commands a human runs after `vault login`. Node services keep their platform tokens.
func usesLoginToken(cmd string, args []string) bool {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch cmd {
	case "tenants", "envs":
		return true
	case "snapshot":
		return sub == "take" || sub == "restore"
	case "health":
		return sub != "serve"
	}
	return false
}

func useOperatorToken(c *vaultcluster.Client) error {
	if c.Cfg.Token != "" {
		return nil
	}
	store, err := keyStore()
	if err != nil {
		return err
	}
	tok, err := store.LoadToken("operator")
	if err != nil {
		return fmt.Errorf("set VAULT_TOKEN or bootstrap first (operator token not found): %w", err)
	}
	c.API.SetToken(tok)
	c.Cfg.Token = tok
	return nil
}

func keyStore() (vaultcluster.KeyStore, error) {
	if os.Getenv("VAULT_OPERATOR_SECRET_ARN") != "" || os.Getenv("VAULT_INIT_SECRET_ARN") != "" {
		return awsKeyStore()
	}
	return fileKeyStore(), nil
}

func fileKeyStore() vaultcluster.FileKeyStore {
	return vaultcluster.FileKeyStore{Dir: bootstrapDir()}
}

func awsKeyStore() (*secretsmanager.KeyStore, error) {
	store, err := secretsmanager.New(
		os.Getenv("VAULT_INIT_SECRET_ARN"),
		os.Getenv("VAULT_PROVISIONING_SECRET_ARN"),
		os.Getenv("VAULT_OPERATOR_SECRET_ARN"),
	)
	if err != nil {
		return nil, err
	}
	store.AppsAuthARN = os.Getenv("VAULT_APPS_AUTH_SECRET_ARN")
	return store, nil
}

func awsClaimInit() (bool, error) {
	store, err := s3.New()
	if err != nil {
		return false, err
	}
	return s3.ClaimInit(store, os.Getenv("SNAPSHOT_BUCKET"), getenv("SNAPSHOT_PREFIX", "vault-snapshots"), os.Getenv("VAULT_RAFT_NODE_ID"))
}

func bootstrapDir() string {
	return getenv("BOOTSTRAP_DIR", ".bootstrap")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func runLifecycle(c *vaultcluster.Client) error {
	nodeID := os.Getenv("VAULT_RAFT_NODE_ID")
	if nodeID == "" {
		return fmt.Errorf("VAULT_RAFT_NODE_ID is required for lifecycle")
	}
	hooks, err := asg.New(nodeID)
	if err != nil {
		return err
	}
	log.Printf("lifecycle watching instance %s", nodeID)
	return vaultcluster.RunLifecycle(context.Background(), hooks, &lifecycleNode{c: c, nodeID: nodeID}, vaultcluster.LifecycleOptions{Logf: log.Printf})
}

// lifecycleNode loads the operator token lazily: on a fresh cluster it does not exist until bootstrap
// finishes, and on a rebuilt one Secrets Manager may still hold the previous cluster's token.
type lifecycleNode struct {
	c        *vaultcluster.Client
	nodeID   string
	renewing bool
}

func (n *lifecycleNode) withToken(op func() error) error {
	if err := useOperatorToken(n.c); err != nil {
		return err
	}
	if !n.renewing {
		n.renewing = true
		go n.c.RenewToken(tokenRenewInterval, nil)
	}
	err := op()
	var re *api.ResponseError
	if errors.As(err, &re) && re.StatusCode == 403 {
		n.c.Cfg.Token = ""
		n.c.API.ClearToken()
	}
	return err
}

func (n *lifecycleNode) Ready() error {
	return n.withToken(func() error { return n.c.NodeHealthOK(n.nodeID) })
}

func (n *lifecycleNode) Leave() error {
	return n.withToken(func() error { return n.c.LeaveRaft(n.nodeID) })
}
