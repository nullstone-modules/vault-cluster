# Reconciles human admin-* roles on the aws, gcp, and oidc auth mounts. Reads other roles for the one-principal guard. Cannot read tenant secrets.
path "sys/auth" {
  capabilities = ["read"]
}

path "sys/auth/aws" {
  capabilities = ["create", "read", "update", "sudo"]
}

path "auth/aws/role" {
  capabilities = ["list"]
}

path "auth/aws/role/*" {
  capabilities = ["read"]
}

path "auth/aws/role/admin-*" {
  capabilities = ["create", "read", "update", "delete"]
}

path "sys/auth/gcp" {
  capabilities = ["create", "read", "update", "sudo"]
}

path "auth/gcp/role" {
  capabilities = ["list"]
}

path "auth/gcp/role/*" {
  capabilities = ["read"]
}

path "auth/gcp/role/admin-*" {
  capabilities = ["create", "read", "update", "delete"]
}

path "sys/auth/oidc" {
  capabilities = ["create", "read", "update", "sudo"]
}

path "auth/oidc/role" {
  capabilities = ["list"]
}

path "auth/oidc/role/*" {
  capabilities = ["read"]
}

path "auth/oidc/role/admin-*" {
  capabilities = ["create", "read", "update", "delete"]
}

path "auth/oidc/config" {
  capabilities = ["create", "read", "update"]
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

path "{{.DatabaseMount}}/creds/*" {
  capabilities = ["deny"]
}
