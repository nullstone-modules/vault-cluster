# Full KV lifecycle on the tenant named by the caller's AppRole role ({{.WriterMount}} mount; role name = tenant ID). Static: written once at bootstrap.
# Name all five KV v2 path families.
path "{{.KVMount}}/data/{{.TenantPrefix}}/{{.WriterTenant}}/*" {
  capabilities = ["create", "read", "update", "patch", "delete", "list"]
}

path "{{.KVMount}}/metadata/{{.TenantPrefix}}/{{.WriterTenant}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "{{.KVMount}}/delete/{{.TenantPrefix}}/{{.WriterTenant}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/undelete/{{.TenantPrefix}}/{{.WriterTenant}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/destroy/{{.TenantPrefix}}/{{.WriterTenant}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/data/{{.TenantPrefix}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/metadata/{{.TenantPrefix}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/delete/{{.TenantPrefix}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/undelete/{{.TenantPrefix}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/destroy/{{.TenantPrefix}}/*" {
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

path "auth/{{.ReaderMount}}/role/*" {
  capabilities = ["deny"]
}

path "auth/{{.WriterMount}}/role/*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/config" {
  capabilities = ["deny"]
}
