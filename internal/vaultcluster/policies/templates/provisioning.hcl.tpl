# Onboard/offboard tenants. Cannot read tenant secrets.
path "sys/policies/acl/tenant-*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "sys/policies/acl" {
  capabilities = ["list"]
}

path "auth/{{.AuthMount}}/role/tenant-*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "auth/{{.AuthMount}}/role" {
  capabilities = ["list"]
}

path "{{.DatabaseMount}}/roles/tenant-*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "sys/mounts" {
  capabilities = ["read"]
}

path "sys/auth" {
  capabilities = ["read"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "{{.KVMount}}/*" {
  capabilities = ["deny"]
}

path "sys/audit" {
  capabilities = ["deny"]
}

path "sys/audit/*" {
  capabilities = ["deny"]
}

path "sys/audit-hash/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/creds/*" {
  capabilities = ["deny"]
}

path "sys/mounts/*" {
  capabilities = ["deny"]
}

path "sys/auth/*" {
  capabilities = ["deny"]
}

path "auth/token/create*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}
