# Read-only on one tenant. KV v2 needs data/ and metadata/; kv/{prefix}/{id} matches nothing.
path "@@KV_MOUNT@@/data/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["read"]
}

path "@@KV_MOUNT@@/metadata/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["read", "list"]
}

path "@@KV_MOUNT@@/data/@@TENANT_PREFIX@@/*" {
  capabilities = ["deny"]
}

path "@@KV_MOUNT@@/metadata/@@TENANT_PREFIX@@/*" {
  capabilities = ["deny"]
}

path "@@KV_MOUNT@@/delete/@@TENANT_PREFIX@@/*" {
  capabilities = ["deny"]
}

path "@@KV_MOUNT@@/undelete/@@TENANT_PREFIX@@/*" {
  capabilities = ["deny"]
}

path "@@KV_MOUNT@@/destroy/@@TENANT_PREFIX@@/*" {
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

path "auth/@@AUTH_MOUNT@@/role/*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}

path "@@KV_MOUNT@@/config" {
  capabilities = ["deny"]
}
