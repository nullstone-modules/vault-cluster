# Read-only on the tenant named by the caller's AppRole role ({{.ReaderMount}} mount; role name = tenant ID). Static: written once at bootstrap.
# KV v2 needs data/ and metadata/; a token without a {{.ReaderMount}} alias renders no tenant path.
path "{{.KVMount}}/data/{{.TenantPrefix}}/{{.ReaderTenant}}/*" {
  capabilities = ["read"]
}

path "{{.KVMount}}/metadata/{{.TenantPrefix}}/{{.ReaderTenant}}/*" {
  capabilities = ["read", "list"]
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
