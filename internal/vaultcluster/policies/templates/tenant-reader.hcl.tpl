# Read-only on one tenant. KV v2 needs data/ and metadata/; kv/{prefix}/{id} matches nothing.
path "{{.KVMount}}/data/{{.TenantPrefix}}/{{.TenantID}}/*" {
  capabilities = ["read"]
}

path "{{.KVMount}}/metadata/{{.TenantPrefix}}/{{.TenantID}}/*" {
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

path "auth/{{.AuthMount}}/role/*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/config" {
  capabilities = ["deny"]
}
