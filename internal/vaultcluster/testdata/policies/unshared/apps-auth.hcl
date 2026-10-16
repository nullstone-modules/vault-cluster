# Held by the cluster function through auth/aws/role/apps-auth. Writes app auth roles on the aws and gcp mounts,
# which bootstrap owns; reads sys/auth only for a mount accessor. Cannot read tenant secrets.
path "sys/auth" {
  capabilities = ["read"]
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
