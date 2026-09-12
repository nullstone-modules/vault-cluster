# Local operator: health and config read. Can start generate-root (recovery keys still required). Not a tenant secret reader. No restore.
path "sys/health" {
  capabilities = ["read", "sudo"]
}

path "sys/seal-status" {
  capabilities = ["read"]
}

path "sys/leader" {
  capabilities = ["read"]
}

path "sys/mounts" {
  capabilities = ["read", "sudo"]
}

path "sys/mounts/{{.DatabaseMount}}" {
  capabilities = ["create", "update", "sudo"]
}

path "sys/mounts/*" {
  capabilities = ["read"]
}

path "sys/auth" {
  capabilities = ["read"]
}

path "sys/policies/acl" {
  capabilities = ["list"]
}

path "sys/policies/acl/*" {
  capabilities = ["read"]
}

path "sys/audit" {
  capabilities = ["read", "sudo"]
}

path "auth/{{.AuthMount}}/role" {
  capabilities = ["list"]
}

path "auth/{{.AuthMount}}/role/*" {
  capabilities = ["read"]
}

path "{{.DatabaseMount}}/roles" {
  capabilities = ["list"]
}

path "{{.DatabaseMount}}/roles/*" {
  capabilities = ["read"]
}

path "{{.DatabaseMount}}/config/*" {
  capabilities = ["create", "update", "read"]
}

path "sys/leases/lookup" {
  capabilities = ["update"]
}

path "sys/leases/lookup/*" {
  capabilities = ["list", "read"]
}

path "sys/leases/revoke" {
  capabilities = ["update"]
}

path "sys/leases/revoke-prefix/*" {
  capabilities = ["update", "sudo"]
}

path "sys/generate-root" {
  capabilities = ["create", "update", "read", "delete", "sudo"]
}

path "sys/generate-root/*" {
  capabilities = ["create", "update", "read", "delete", "sudo"]
}

path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}

path "sys/storage/raft/configuration" {
  capabilities = ["read"]
}

path "sys/storage/raft/autopilot/state" {
  capabilities = ["read"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/token/revoke-self" {
  capabilities = ["update"]
}

path "{{.KVMount}}/data/*" {
  capabilities = ["deny"]
}

path "{{.KVMount}}/metadata/*" {
  capabilities = ["deny"]
}

path "{{.DatabaseMount}}/creds/*" {
  capabilities = ["deny"]
}

path "sys/policies/acl/operator" {
  capabilities = ["deny"]
}

path "sys/audit/*" {
  capabilities = ["deny"]
}

path "sys/storage/raft/snapshot-force" {
  capabilities = ["deny"]
}
