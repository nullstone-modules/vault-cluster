# MUST BE REJECTED - the KV v2 path split. Looks correct, parses correctly,
# applies without error, and matches nothing at all.
path "kv/customers/tenant-a/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
