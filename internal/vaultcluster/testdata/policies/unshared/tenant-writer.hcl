# Full KV lifecycle on the tenant named by the caller's AppRole role (approle-writer mount; role name = tenant ID). Static: written once at bootstrap.
# Name all five KV v2 path families.
path "kv/data/customers/{{identity.entity.aliases.auth_approle_5e6f7a8b.metadata.role_name}}/*" {
  capabilities = ["create", "read", "update", "patch", "delete", "list"]
}

path "kv/metadata/customers/{{identity.entity.aliases.auth_approle_5e6f7a8b.metadata.role_name}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "kv/delete/customers/{{identity.entity.aliases.auth_approle_5e6f7a8b.metadata.role_name}}/*" {
  capabilities = ["update"]
}

path "kv/undelete/customers/{{identity.entity.aliases.auth_approle_5e6f7a8b.metadata.role_name}}/*" {
  capabilities = ["update"]
}

path "kv/destroy/customers/{{identity.entity.aliases.auth_approle_5e6f7a8b.metadata.role_name}}/*" {
  capabilities = ["update"]
}

path "kv/data/customers/*" {
  capabilities = ["deny"]
}

path "kv/metadata/customers/*" {
  capabilities = ["deny"]
}

path "kv/delete/customers/*" {
  capabilities = ["deny"]
}

path "kv/undelete/customers/*" {
  capabilities = ["deny"]
}

path "kv/destroy/customers/*" {
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
