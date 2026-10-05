# Writes AWS auth roles for app capabilities. Cannot read tenant secrets.
path "sys/auth/aws" {
  capabilities = ["create", "read", "update", "sudo"]
}

path "sys/auth" {
  capabilities = ["read"]
}

path "auth/aws/role" {
  capabilities = ["list"]
}

path "auth/aws/role/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Human admin roles are written with the admin-auth token.
path "auth/aws/role/admin-*" {
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

path "{{.DatabaseMount}}/creds/*" {
  capabilities = ["deny"]
}
