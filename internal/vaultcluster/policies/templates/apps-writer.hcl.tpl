# App broker, write level. Lets an app log in as any onboarded tenant's writer: read the role ID and mint a
# one-time secret ID on {{.WriterMount}}. The token that login yields sees one tenant; this token sees none.
{{if .Shared -}}
# Shared cluster: only roles of the app's own env (entity metadata). No parameter may be sent, so the role,
# its role-id, and its policies cannot be changed; an empty-body secret-id mint still works.
path "auth/{{.WriterMount}}/role/{{.Env}}.*" {
  capabilities = ["read", "update"]
  denied_parameters = { "*" = [] }
}
{{- else -}}
path "auth/{{.WriterMount}}/role/+/role-id" {
  capabilities = ["read"]
}

path "auth/{{.WriterMount}}/role/+/secret-id" {
  capabilities = ["update"]
}
{{- end}}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/{{.ReaderMount}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/*" {
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
