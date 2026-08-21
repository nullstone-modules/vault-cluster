# Local Vault CE. Raft on /vault/file. No dev mode. TLS off; Compose binds 127.0.0.1.

cluster_name = "vault-local"
ui = true

storage "raft" {
  path    = "/vault/file"
  node_id = "vault-local-1"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = 1
}

api_addr     = "http://vault:8200"
cluster_addr = "http://vault:8201"

log_level  = "info"
log_format = "json"

# Compose and GitHub Actions cannot mlock. Production hosts use IPC_LOCK instead.
disable_mlock = true
