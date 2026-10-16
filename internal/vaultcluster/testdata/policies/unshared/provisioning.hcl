# Onboard/offboard tenants. Writes AppRole roles only; tenant policies are static (bootstrap).
# allowed_parameters pins every role to the tenant policies, so nothing written here can carry
# a platform policy. Can mint any tenant's credentials; cannot read tenant secrets itself.
path "auth/approle-reader/role" {
  capabilities = ["list"]
}

path "auth/approle-reader/role/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
  allowed_parameters = {
    "token_policies"     = ["tenant-reader"]
    "token_ttl"          = []
    "token_max_ttl"      = []
    "token_type"         = ["service"]
    "secret_id_ttl"      = []
    "secret_id_num_uses" = []
    "bind_secret_id"     = []
  }
}

path "auth/approle-reader/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/approle-reader/role/+/secret-id" {
  capabilities = ["update"]
}

path "auth/approle-writer/role" {
  capabilities = ["list"]
}

path "auth/approle-writer/role/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
  allowed_parameters = {
    "token_policies"     = ["tenant-writer"]
    "token_ttl"          = []
    "token_max_ttl"      = []
    "token_type"         = ["service"]
    "secret_id_ttl"      = []
    "secret_id_num_uses" = []
    "bind_secret_id"     = []
  }
}

path "auth/approle-writer/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/approle-writer/role/+/secret-id" {
  capabilities = ["update"]
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

path "kv/*" {
  capabilities = ["deny"]
}

path "sys/policies/*" {
  capabilities = ["deny"]
}

path "sys/policy/*" {
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
