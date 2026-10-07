# Writes app auth roles through the cluster function, on the aws and gcp mounts. Cannot read tenant secrets.
path "sys/auth" {
  capabilities = ["read"]
}

path "sys/auth/aws" {
  capabilities = ["create", "read", "update", "sudo"]
}

path "sys/auth/gcp" {
  capabilities = ["create", "read", "update", "sudo"]
}

path "auth/aws/role" {
  capabilities = ["list"]
}

path "auth/aws/role/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "auth/gcp/role" {
  capabilities = ["list"]
}

path "auth/gcp/role/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "kv/*" {
  capabilities = ["deny"]
}

path "database/creds/*" {
  capabilities = ["deny"]
}
