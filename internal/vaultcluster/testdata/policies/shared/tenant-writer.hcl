# Full KV lifecycle on the tenant named by the caller's AppRole role (approle-writer mount; role name = tenant ID). Static: written once at bootstrap.
# Name all five KV v2 path families.
path "kv/data/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["create", "read", "update", "patch", "delete", "list"]
}

path "kv/metadata/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "kv/delete/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["update"]
}

path "kv/undelete/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["update"]
}

path "kv/destroy/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["update"]
}

path "kv/data/envs/*" {
  capabilities = ["deny"]
}

path "kv/metadata/envs/*" {
  capabilities = ["deny"]
}

path "kv/delete/envs/*" {
  capabilities = ["deny"]
}

path "kv/undelete/envs/*" {
  capabilities = ["deny"]
}

path "kv/destroy/envs/*" {
  capabilities = ["deny"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "sys/*" {
  capabilities = ["deny"]
}

path "auth/token/create*" {
  capabilities = ["deny"]
}

path "auth/approle-reader/role/*" {
  capabilities = ["deny"]
}

path "auth/approle-writer/role/*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}

path "kv/config" {
  capabilities = ["deny"]
}
