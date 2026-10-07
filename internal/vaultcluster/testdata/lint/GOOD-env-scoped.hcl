# MUST PASS - shared cluster: scoped to one env and one tenant, denies the env
# prefix for everything else.
path "kv/data/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
path "kv/metadata/envs/dev/customers/tenant-a/*" {
  capabilities = ["read", "list"]
}
path "kv/data/envs/*" {
  capabilities = ["deny"]
}
path "kv/metadata/envs/*" {
  capabilities = ["deny"]
}
path "sys/*" {
  capabilities = ["deny"]
}
