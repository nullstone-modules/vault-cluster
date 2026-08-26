# Full KV lifecycle on one tenant. Name all five KV v2 path families.
path "{{.KVMount}}/data/{{.TenantPrefix}}/{{.TenantID}}/*" {
  capabilities = ["create", "read", "update", "patch", "delete", "list"]
}

path "{{.KVMount}}/metadata/{{.TenantPrefix}}/{{.TenantID}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "{{.KVMount}}/delete/{{.TenantPrefix}}/{{.TenantID}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/undelete/{{.TenantPrefix}}/{{.TenantID}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/destroy/{{.TenantPrefix}}/{{.TenantID}}/*" {
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

path "auth/{{.AuthMount}}/role/*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/config" {
  capabilities = ["deny"]
}
