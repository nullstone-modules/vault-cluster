# MUST BE REJECTED - grants read over the entire Vault API, including sys/,
# auth/, and every tenant's secrets.
path "*" {
  capabilities = ["read", "list"]
}
