# Full KV lifecycle on one tenant. Name all five KV v2 path families.
path "@@KV_MOUNT@@/data/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["create", "read", "update", "patch", "delete", "list"]
}

path "@@KV_MOUNT@@/metadata/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "@@KV_MOUNT@@/delete/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["update"]
}

path "@@KV_MOUNT@@/undelete/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["update"]
}

path "@@KV_MOUNT@@/destroy/@@TENANT_PREFIX@@/@@TENANT_ID@@/*" {
  capabilities = ["update"]
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
