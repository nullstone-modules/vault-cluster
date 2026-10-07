{{- /* Unshared: tenant-<role_name alias>. Shared: tenant-<entity env>.<entity tenant>. */ -}}
{{- $role := printf "tenant-%s" .WriterTenant -}}
{{- if .Shared}}{{$role = printf "tenant-%s.%s" .Env .WriterTenant}}{{end -}}
# Dynamic DB creds for the tenant named by the caller's {{.WriterMount}} role only. Additive; does not widen KV access.
path "{{.DatabaseMount}}/creds/{{$role}}-*" {
  capabilities = ["read"]
}

path "{{.DatabaseMount}}/roles/{{$role}}-*" {
  capabilities = ["read"]
}

path "sys/leases/renew" {
  capabilities = ["update"]
}

path "sys/leases/revoke" {
  capabilities = ["update"]
}

path "{{.DatabaseMount}}/creds/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/roles/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/config/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/static-creds/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/rotate-root/*" {
  capabilities = ["deny"]
}
