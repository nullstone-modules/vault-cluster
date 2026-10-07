{{- /* Unshared: customers/<role_name alias>. Shared: envs/<entity env>/customers/<entity tenant>. */ -}}
{{- $tenant := printf "%s/%s" .TenantPrefix .WriterTenant -}}
{{- $deny := .TenantPrefix -}}
{{- if .Shared}}{{$tenant = printf "%s/%s/%s" .EnvPrefix .Env $tenant}}{{$deny = .EnvPrefix}}{{end -}}
# Full KV lifecycle on the tenant named by the caller's AppRole role ({{.WriterMount}} mount; role name = tenant ID). Static: written once at bootstrap.
# Name all five KV v2 path families.
path "{{.KVMount}}/data/{{$tenant}}/*" {
  capabilities = ["create", "read", "update", "patch", "delete", "list"]
}

path "{{.KVMount}}/metadata/{{$tenant}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "{{.KVMount}}/delete/{{$tenant}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/undelete/{{$tenant}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/destroy/{{$tenant}}/*" {
  capabilities = ["update"]
}

path "{{.KVMount}}/data/{{$deny}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/metadata/{{$deny}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/delete/{{$deny}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/undelete/{{$deny}}/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/destroy/{{$deny}}/*" {
  capabilities = ["deny"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "sys/*" {
  capabilities = ["deny"]
}

path "auth/token/create*" {
  capabilities = ["deny"]
}

path "auth/{{.ReaderMount}}/role/*" {
  capabilities = ["deny"]
}

path "auth/{{.WriterMount}}/role/*" {
  capabilities = ["deny"]
}

path "identity/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/config" {
  capabilities = ["deny"]
}
