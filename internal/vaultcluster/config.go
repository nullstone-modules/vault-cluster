package vaultcluster

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr              string
	Token             string
	KVMount           string
	TenantPrefix      string
	AuthMount         string
	DatabaseMount     string
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

func (c Config) KVDataPath(tenantID, secret string) string {
	p := fmt.Sprintf("%s/data/%s/%s", c.KVMount, c.TenantPrefix, tenantID)
	if secret != "" {
		p += "/" + strings.TrimPrefix(secret, "/")
	}
	return p
}

func (c Config) KVMetaPath(tenantID, secret string) string {
	p := fmt.Sprintf("%s/metadata/%s/%s", c.KVMount, c.TenantPrefix, tenantID)
	if secret != "" {
		p += "/" + strings.TrimPrefix(secret, "/")
	}
	return p
}
