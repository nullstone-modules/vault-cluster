# Read-only on the tenant named by the caller's AppRole role (approle-reader mount; role name = tenant ID). Static: written once at bootstrap.
# KV v2 needs data/ and metadata/; a token without a approle-reader alias renders no tenant path.
path "kv/data/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["read"]
}

path "kv/metadata/envs/{{identity.entity.metadata.env}}/customers/{{identity.entity.metadata.tenant}}/*" {
  capabilities = ["read", "list"]
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
