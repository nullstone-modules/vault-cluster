# MUST BE REJECTED - sudo is root-equivalent on the path and has no place in a
# tenant policy.
path "kv/data/customers/tenant-a/*" {
  capabilities = ["read", "sudo"]
}
