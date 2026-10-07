# App broker, write level. Lets an app log in as any onboarded tenant's writer: read the role ID and mint a
# one-time secret ID on approle-writer. The token that login yields sees one tenant; this token sees none.
# Shared cluster: only roles of the app's own env (entity metadata). No parameter may be sent, so the role,
# its role-id, and its policies cannot be changed; an empty-body secret-id mint still works.
path "auth/approle-writer/role/{{identity.entity.metadata.env}}.*" {
  capabilities = ["read", "update"]
  denied_parameters = { "*" = [] }
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
