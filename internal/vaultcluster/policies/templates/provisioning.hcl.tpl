# Onboard/offboard tenants. Writes AppRole and database roles only; tenant policies are static (bootstrap).
# allowed_parameters pins every role to the tenant policies and fixed SQL, so nothing written here
# can carry a platform policy. Can mint any tenant's credentials; cannot read tenant secrets itself.
path "auth/{{.ReaderMount}}/role" {
  capabilities = ["list"]
}

path "auth/{{.ReaderMount}}/role/*" {
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

path "auth/{{.ReaderMount}}/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/{{.ReaderMount}}/role/+/secret-id" {
  capabilities = ["update"]
}

path "auth/{{.WriterMount}}/role" {
  capabilities = ["list"]
}

path "auth/{{.WriterMount}}/role/*" {
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

path "auth/{{.WriterMount}}/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/{{.WriterMount}}/role/+/secret-id" {
  capabilities = ["update"]
}

path "{{.DatabaseMount}}/roles/tenant-*" {
  capabilities = ["create", "read", "update", "delete", "list"]
  allowed_parameters = {
    "db_name"             = []
    "creation_statements" = [{{.DBCreationStatements}}]
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

path "{{.KVMount}}/*" {
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
