package vaultcluster

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	Token         string
	KVMount       string
	TenantPrefix  string
	AuthMount     string
	DatabaseMount string
	// SharedEnvs scopes every tenant under an env (envs/{env}/customers/{tenant}). Set only on a cluster
	// that several Nullstone envs share. Off, the cluster behaves exactly as an unshared one.
	SharedEnvs        bool
	EnvPrefix         string
	AuditPath         string
	AuditDevice       string
	EnableAudit       bool
	EnableCredentials bool
	DatabaseURL       string
	DatabaseUsername  string
	DatabasePassword  string
	DatabaseConnName  string
	DatabaseTTL       string
	DatabaseMaxTTL    string
	TokenTTL          string
	TokenMaxTTL       string
	HTTPTimeout       time.Duration
}

func ConfigFromEnv() Config {
	c := Config{
		Addr:          getenv("VAULT_ADDR", ""),
		Token:         getenv("VAULT_TOKEN", ""),
		KVMount:       getenv("KV_MOUNT", "kv"),
		TenantPrefix:  getenv("TENANT_PREFIX", "customers"),
		AuthMount:     getenv("AUTH_MOUNT", "approle"),
		DatabaseMount: getenv("DATABASE_MOUNT", "database"),
		SharedEnvs:    getenv("SHARED_ENVS", "false") == "true",
		EnvPrefix:     getenv("ENV_PREFIX", "envs"),
		AuditPath:     getenv("AUDIT_LOG_PATH", "/vault/logs/audit.log"),
		AuditDevice:   getenv("AUDIT_DEVICE_NAME", "file"),
		EnableAudit:   getenv("ENABLE_AUDIT", "true") == "true",
		// The dynamic-credentials fields (EnableCredentials, DatabaseURL,
		// DatabaseUsername, ...) are never set from the environment: the
		// database engine is configured only by callers that opt in
		// programmatically. DatabaseMount stays because the tenant and
		// platform policies reference its paths even when the engine is
		// not mounted.
		TokenTTL:    getenv("DEFAULT_TOKEN_TTL", "15m"),
		TokenMaxTTL: getenv("MAX_TOKEN_TTL", "15m"),
		HTTPTimeout: 15 * time.Second,
	}
	return c
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// TenantMount is the AppRole mount for one access kind (reader, writer). Role names on it are tenant IDs,
// so the static tenant-<kind> policy reads the tenant from the login's role_name alias metadata.
func (c Config) TenantMount(kind string) string {
	return c.AuthMount + "-" + kind
}

// TenantPolicy is the static policy for one access kind (reader, writer, database).
func (c Config) TenantPolicy(kind string) string {
	return "tenant-" + kind
}

// AppsPolicy lets an app log in as any tenant on the TenantMount of one access kind (reader, writer).
func (c Config) AppsPolicy(kind string) string {
	return "apps-" + kind
}

// TenantRole is the AppRole role name for a tenant: the tenant ID, or env.tenant on a shared cluster.
// Neither an env name nor a tenant ID may contain ".", so the split is unambiguous.
func (c Config) TenantRole(env, tenantID string) string {
	if env == "" {
		return tenantID
	}
	return env + "." + tenantID
}

// SplitTenantRole is the inverse of TenantRole. env is empty for an unscoped role.
func SplitTenantRole(role string) (env, tenantID string) {
	if i := strings.IndexByte(role, '.'); i >= 0 {
		return role[:i], role[i+1:]
	}
	return "", role
}

// tenantPath is the KV path segment for one tenant under the mount's data/metadata families.
func (c Config) tenantPath(env, tenantID string) string {
	if env == "" {
		return c.TenantPrefix + "/" + tenantID
	}
	return c.EnvPrefix + "/" + env + "/" + c.TenantPrefix + "/" + tenantID
}

func (c Config) KVDataPath(env, tenantID, secret string) string {
	p := fmt.Sprintf("%s/data/%s", c.KVMount, c.tenantPath(env, tenantID))
	if secret != "" {
		p += "/" + strings.TrimPrefix(secret, "/")
	}
	return p
}

func (c Config) KVMetaPath(env, tenantID, secret string) string {
	p := fmt.Sprintf("%s/metadata/%s", c.KVMount, c.tenantPath(env, tenantID))
	if secret != "" {
		p += "/" + strings.TrimPrefix(secret, "/")
	}
	return p
}

// checkEnv enforces the mode: a shared cluster needs an env on every tenant, an unshared one refuses it.
func (c Config) checkEnv(env string) error {
	if c.SharedEnvs {
		if env == "" {
			return fmt.Errorf("this cluster is shared across envs; an env is required")
		}
		return ValidateEnvName(env)
	}
	if env != "" {
		return fmt.Errorf("this cluster is not shared across envs; env %q is not allowed", env)
	}
	return nil
}
