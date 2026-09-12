# MUST PASS - correctly scoped to one tenant, covers both KV v2 path families,
# and denies the rest of the tenant prefix.
path "kv/data/customers/tenant-a/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
path "kv/metadata/customers/tenant-a/*" {
  capabilities = ["read", "list"]
}
path "kv/data/customers/*" {
  capabilities = ["deny"]
}
path "kv/metadata/customers/*" {
  capabilities = ["deny"]
}
path "sys/*" {
  capabilities = ["deny"]
}
