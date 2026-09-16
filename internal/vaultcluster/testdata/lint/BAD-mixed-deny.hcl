# MUST BE REJECTED - deny overrides the other capabilities, so this rule denies
# everything while reading as though it allows reads.
path "kv/data/customers/tenant-a/*" {
  capabilities = ["read", "list", "deny"]
}
