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

# Shared cluster: the function writes one entity per app role (metadata env) and its alias, so the broker
# policies can scope the app to its env. policies and disabled are refused.
path "identity/entity" {
  capabilities = ["create", "update"]
  denied_parameters = { "policies" = [], "disabled" = [] }
}

path "identity/entity/name/*" {
  capabilities = ["read", "update", "delete"]
  denied_parameters = { "policies" = [], "disabled" = [] }
}

path "identity/entity-alias" {
  capabilities = ["create", "update"]
}

path "identity/entity-alias/id/*" {
  capabilities = ["read", "delete"]
}

path "identity/lookup/entity" {
  capabilities = ["update"]
}

path "identity/entity/merge" {
  capabilities = ["deny"]
}

path "identity/group*" {
  capabilities = ["deny"]
}

path "identity/oidc/*" {
  capabilities = ["deny"]
}

path "identity/mfa/*" {
  capabilities = ["deny"]
}
