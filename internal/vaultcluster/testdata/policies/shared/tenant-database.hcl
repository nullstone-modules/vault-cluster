# Dynamic DB creds for the tenant named by the caller's approle-writer role only. Additive; does not widen KV access.
path "database/creds/tenant-{{identity.entity.metadata.env}}.{{identity.entity.metadata.tenant}}-*" {
  capabilities = ["read"]
}

path "database/roles/tenant-{{identity.entity.metadata.env}}.{{identity.entity.metadata.tenant}}-*" {
  capabilities = ["read"]
}

path "sys/leases/renew" {
  capabilities = ["update"]
}

path "sys/leases/revoke" {
  capabilities = ["update"]
}

path "database/creds/*" {
  capabilities = ["deny"]
}

path "database/roles/*" {
  capabilities = ["deny"]
}

path "database/config/*" {
  capabilities = ["deny"]
}

path "database/static-creds/*" {
  capabilities = ["deny"]
}

path "database/rotate-root/*" {
  capabilities = ["deny"]
}
