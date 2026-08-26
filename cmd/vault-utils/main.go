package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

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
	fmt.Fprintf(os.Stderr, `vault-utils <command> <subcommand>

Commands:
  bootstrap local|aws|azure|gcp    Initialize a cluster: init (once), unseal, configure
  tenants create <id> [--no-credentials]
  tenants destroy <id> --yes [--purge-secrets]
  snapshot take|list|verify <file>|restore <file> --yes
  health

Key material for bootstrap local is stored under BOOTSTRAP_DIR (default .bootstrap).
`)
}

func run(cmd string, args []string) error {
	cfg := vaultcluster.ConfigFromEnv()
	c, err := vaultcluster.New(cfg)
	if err != nil {
		return err
	}
	switch cmd {
	case "bootstrap":
		return runBootstrap(c, args)
	case "tenants":
		return runTenants(c, args)
	case "snapshot":
		return runSnapshot(c, args)
	case "health":
		return c.Health()
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func runBootstrap(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vault-utils bootstrap local|aws|azure|gcp")
	}
	switch args[0] {
	case "local":
		shares, _ := strconv.Atoi(getenv("VAULT_INIT_KEY_SHARES", "5"))
		threshold, _ := strconv.Atoi(getenv("VAULT_INIT_KEY_THRESHOLD", "3"))
		return c.RunBootstrap(keyStore(), vaultcluster.BootstrapOptions{
			Shares:    shares,
			Threshold: threshold,
			KeepRoot:  getenv("KEEP_ROOT", "false") == "true",
		})
	case "aws", "azure", "gcp":
		return fmt.Errorf("bootstrap %s is not implemented yet", args[0])
	default:
		return fmt.Errorf("unknown platform %q (local, aws, azure, gcp)", args[0])
	}
}

func runTenants(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vault-utils tenants create|destroy <id>")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		issue := true
		id := ""
		for _, a := range rest {
			switch a {
			case "--no-credentials":
				issue = false
			default:
				id = a
			}
		}
		if id == "" {
			return fmt.Errorf("usage: vault-utils tenants create <id> [--no-credentials]")
		}
		return c.CreateTenant(id, issue)
	case "destroy":
		yes, purge := false, false
		id := ""
		for _, a := range rest {
			switch a {
			case "--yes":
				yes = true
			case "--purge-secrets":
				purge = true
			default:
				if !strings.HasPrefix(a, "-") {
					id = a
				}
			}
		}
		if id == "" || !yes {
			return fmt.Errorf("usage: vault-utils tenants destroy <id> --yes [--purge-secrets]")
		}
		return c.OffboardTenant(id, purge)
	default:
		return fmt.Errorf("unknown subcommand %q (create, destroy)", sub)
	}
}

func runSnapshot(c *vaultcluster.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vault-utils snapshot take|list|verify <file>|restore <file> --yes")
	}
	backupDir := filepath.Join(bootstrapDir(), "backups")
	switch args[0] {
	case "take":
		if err := useOperatorToken(c); err != nil {
			return err
		}
		file, err := c.SnapshotTake(backupDir)
		if err != nil {
			return err
		}
		log.Printf("snapshot written: %s", file)
		log.Printf("this file contains every secret in the cluster; treat it as one")
		return nil
	case "list":
		files, err := vaultcluster.SnapshotList(backupDir)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			log.Printf("no snapshots under %s", backupDir)
			return nil
		}
		for _, f := range files {
			fmt.Println(f)
		}
		return nil
	case "verify":
		if len(args) < 2 {
			return fmt.Errorf("usage: vault-utils snapshot verify <file>")
		}
		if err := vaultcluster.SnapshotVerify(args[1]); err != nil {
			return err
		}
		log.Printf("checksum OK: %s", args[1])
		return nil
	case "restore":
		if len(args) < 3 || args[2] != "--yes" {
			return fmt.Errorf("restore replaces the entire cluster; re-run with: vault-utils snapshot restore <file> --yes")
		}
		if c.Cfg.Token == "" {
			return fmt.Errorf("restore requires VAULT_TOKEN with sys/storage/raft/snapshot-force (break-glass root); the operator token cannot restore")
		}
		if err := c.SnapshotRestore(args[1]); err != nil {
			return err
		}
		log.Printf("restore submitted; Vault will seal")
		log.Printf("unseal with the key shares that were current when this snapshot was taken")
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (take, list, verify, restore)", args[0])
	}
}

func useOperatorToken(c *vaultcluster.Client) error {
	if c.Cfg.Token != "" {
		return nil
	}
	tok, err := keyStore().LoadToken("operator")
	if err != nil {
		return fmt.Errorf("set VAULT_TOKEN or bootstrap first (operator token not found): %w", err)
	}
	c.API.SetToken(tok)
	c.Cfg.Token = tok
	return nil
}

func keyStore() vaultcluster.FileKeyStore {
	return vaultcluster.FileKeyStore{Dir: bootstrapDir()}
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
