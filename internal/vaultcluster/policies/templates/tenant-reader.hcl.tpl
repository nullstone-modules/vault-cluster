{{- /* Unshared: customers/<role_name alias>. Shared: envs/<entity env>/customers/<entity tenant>. */ -}}
{{- $tenant := printf "%s/%s" .TenantPrefix .ReaderTenant -}}
{{- $deny := .TenantPrefix -}}
{{- if .Shared}}{{$tenant = printf "%s/%s/%s" .EnvPrefix .Env $tenant}}{{$deny = .EnvPrefix}}{{end -}}
# Read-only on the tenant named by the caller's AppRole role ({{.ReaderMount}} mount; role name = tenant ID). Static: written once at bootstrap.
# KV v2 needs data/ and metadata/; a token without a {{.ReaderMount}} alias renders no tenant path.
path "{{.KVMount}}/data/{{$tenant}}/*" {
  capabilities = ["read"]
}

path "{{.KVMount}}/metadata/{{$tenant}}/*" {
  capabilities = ["read", "list"]
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
