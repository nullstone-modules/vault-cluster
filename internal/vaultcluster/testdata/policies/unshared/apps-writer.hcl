# App broker, write level. Lets an app log in as any onboarded tenant's writer: read the role ID and mint a
# one-time secret ID on approle-writer. The token that login yields sees one tenant; this token sees none.
path "auth/approle-writer/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/approle-writer/role/+/secret-id" {
  capabilities = ["update"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/approle-reader/*" {
  capabilities = ["deny"]
}

path "kv/*" {
  capabilities = ["deny"]
}

path "database/*" {
  capabilities = ["deny"]
}

path "sys/*" {
  capabilities = ["deny"]
}

path "auth/token/create*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}
