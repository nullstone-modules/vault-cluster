# Onboard/offboard tenants. Writes AppRole and database roles only; tenant policies are static (bootstrap).
# allowed_parameters pins every role to the tenant policies and fixed SQL, so nothing written here
# can carry a platform policy. Can mint any tenant's credentials; cannot read tenant secrets itself.
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
    "token_policies"     = ["tenant-writer", "tenant-database"]
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

path "database/roles/tenant-*" {
  capabilities = ["create", "read", "update", "delete", "list"]
  allowed_parameters = {
    "db_name"             = []
    "creation_statements" = ["CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';", "GRANT app_readonly TO \"{{name}}\";", "GRANT app_readwrite TO \"{{name}}\";"]
    "default_ttl"         = []
    "max_ttl"             = []
  }
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

path "database/creds/*" {
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

# Shared cluster: tenants create writes one entity per tenant role (metadata env and tenant) and its alias.
# policies and disabled are refused, so an entity can never carry more than its roles grant.
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
