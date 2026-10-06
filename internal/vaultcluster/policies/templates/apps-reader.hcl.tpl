# App broker, read level. Lets an app log in as any onboarded tenant's reader: read the role ID and mint a
# one-time secret ID on {{.ReaderMount}}. The token that login yields sees one tenant; this token sees none.
path "auth/{{.ReaderMount}}/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/{{.ReaderMount}}/role/+/secret-id" {
  capabilities = ["update"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/{{.WriterMount}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/*" {
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
