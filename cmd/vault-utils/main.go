package main

import (
	"fmt"
	"log"
	"os"
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
	fmt.Fprintf(os.Stderr, `vault-utils <command>

Commands:
  local-bootstrap   Init (once), Shamir unseal, configure, synthetic tenants
  configure         Apply platform config (audit, KV, AppRole, policies)
  tenant-create     Onboard a tenant: vault-utils tenant-create <id> [--no-credentials]
  tenant-offboard   Revoke access: vault-utils tenant-offboard <id> --yes [--purge-secrets]
  health            Seal status and KV mount
`)
}

func run(cmd string, args []string) error {
	cfg := vaultcluster.ConfigFromEnv()
	c, err := vaultcluster.New(cfg)
	if err != nil {
		return err
	}
	switch cmd {
	case "local-bootstrap":
		dir := getenv("BOOTSTRAP_DIR", "/bootstrap")
		shares, _ := strconv.Atoi(getenv("VAULT_INIT_KEY_SHARES", "5"))
		thr, _ := strconv.Atoi(getenv("VAULT_INIT_KEY_THRESHOLD", "3"))
		return c.RunLocalBootstrap(vaultcluster.LocalBootstrap{
			Dir:       dir,
			InitFile:  getenv("INIT_FILE", dir+"/vault-init.json"),
			Shares:    shares,
			Threshold: thr,
			KeepRoot:  getenv("KEEP_ROOT", "false") == "true",
			TenantIDs: []string{"tenant-a", "tenant-b"},
		})
	case "configure":
		return c.Configure()
	case "tenant-create":
		if len(args) < 1 {
			return fmt.Errorf("usage: vault-utils tenant-create <tenant-id> [--no-credentials]")
		}
		issue := true
		id := ""
		for _, a := range args {
			switch a {
			case "--no-credentials":
				issue = false
			default:
				id = a
			}
		}
		return c.CreateTenant(id, issue)
	case "tenant-offboard":
		yes, purge := false, false
		id := ""
		for _, a := range args {
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
			return fmt.Errorf("usage: vault-utils tenant-offboard <tenant-id> --yes [--purge-secrets]")
		}
		return c.OffboardTenant(id, purge)
	case "health":
		return c.Health()
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
