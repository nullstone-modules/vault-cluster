# MUST BE REJECTED - only one stanza takes effect and which one is unspecified.
path "kv/data/customers/tenant-a/*" {
  capabilities = ["read"]
}
path "kv/data/customers/tenant-a/*" {
  capabilities = ["create", "read", "update", "delete"]
}
