# Dynamic DB creds for the tenant named by the caller's {{.WriterMount}} role only. Additive; does not widen KV access.
path "{{.DatabaseMount}}/creds/tenant-{{.WriterTenant}}-*" {
  capabilities = ["read"]
}

path "{{.DatabaseMount}}/roles/tenant-{{.WriterTenant}}-*" {
  capabilities = ["read"]
}

path "sys/leases/renew" {
  capabilities = ["update"]
}

path "sys/leases/revoke" {
  capabilities = ["update"]
}

path "{{.DatabaseMount}}/creds/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/roles/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/config/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/static-creds/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/rotate-root/*" {
  capabilities = ["deny"]
}
