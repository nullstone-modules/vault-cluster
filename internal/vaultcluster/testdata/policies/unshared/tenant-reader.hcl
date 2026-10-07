# Read-only on the tenant named by the caller's AppRole role (approle-reader mount; role name = tenant ID). Static: written once at bootstrap.
# KV v2 needs data/ and metadata/; a token without a approle-reader alias renders no tenant path.
path "kv/data/customers/{{identity.entity.aliases.auth_approle_1a2b3c4d.metadata.role_name}}/*" {
  capabilities = ["read"]
}

path "kv/metadata/customers/{{identity.entity.aliases.auth_approle_1a2b3c4d.metadata.role_name}}/*" {
  capabilities = ["read", "list"]
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
