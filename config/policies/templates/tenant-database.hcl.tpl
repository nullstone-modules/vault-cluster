# Dynamic DB creds for this tenant only. Additive; does not widen KV access.
path "@@DATABASE_MOUNT@@/creds/tenant-@@TENANT_ID@@-*" {
  capabilities = ["read"]
}

path "@@DATABASE_MOUNT@@/roles/tenant-@@TENANT_ID@@-*" {
  capabilities = ["read"]
}

path "sys/leases/renew" {
  capabilities = ["update"]
}

path "sys/leases/revoke" {
  capabilities = ["update"]
}

path "@@DATABASE_MOUNT@@/creds/*" {
  capabilities = ["deny"]
}

path "@@DATABASE_MOUNT@@/roles/*" {
  capabilities = ["deny"]
}

path "@@DATABASE_MOUNT@@/config/*" {
  capabilities = ["deny"]
}

path "@@DATABASE_MOUNT@@/static-creds/*" {
  capabilities = ["deny"]
}

path "@@DATABASE_MOUNT@@/rotate-root/*" {
  capabilities = ["deny"]
}
