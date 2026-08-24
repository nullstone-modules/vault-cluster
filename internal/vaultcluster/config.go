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
		Addr:              getenv("VAULT_ADDR", ""),
		Token:             getenv("VAULT_TOKEN", ""),
		KVMount:           getenv("KV_MOUNT", "kv"),
		TenantPrefix:      getenv("TENANT_PREFIX", "customers"),
		AuthMount:         getenv("AUTH_MOUNT", "approle"),
		DatabaseMount:     getenv("DATABASE_MOUNT", "database"),
		AuditPath:         getenv("AUDIT_LOG_PATH", "/vault/logs/audit.log"),
		AuditDevice:       getenv("AUDIT_DEVICE_NAME", "file"),
		EnableAudit:       getenv("ENABLE_AUDIT", "true") == "true",
		EnableCredentials: getenv("ENABLE_DYNAMIC_CREDENTIALS", "false") == "true",
		DatabaseURL:       os.Getenv("DATABASE_CONNECTION_URL"),
		DatabaseUsername:  getenv("DATABASE_USERNAME", "vault_admin"),
		DatabasePassword:  os.Getenv("DATABASE_PASSWORD"),
		DatabaseConnName:  getenv("DATABASE_CONNECTION_NAME", "app"),
		DatabaseTTL:       getenv("DATABASE_DEFAULT_TTL", "1h"),
		DatabaseMaxTTL:    getenv("DATABASE_MAX_TTL", "24h"),
		TokenTTL:          getenv("DEFAULT_TOKEN_TTL", "1h"),
		TokenMaxTTL:       getenv("MAX_TOKEN_TTL", "24h"),
		HTTPTimeout:       15 * time.Second,
	}
	return c
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c Config) TenantPolicy(kind, tenantID string) string {
	return fmt.Sprintf("tenant-%s-%s", tenantID, kind)
}

func (c Config) TenantRole(kind, tenantID string) string {
	return fmt.Sprintf("tenant-%s-%s", tenantID, kind)
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
