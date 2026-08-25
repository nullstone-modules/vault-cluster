# Local operator: health and config read. Not a tenant secret reader. No restore.
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

path "sys/mounts/@@DATABASE_MOUNT@@" {
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

path "auth/@@AUTH_MOUNT@@/role" {
  capabilities = ["list"]
}

path "auth/@@AUTH_MOUNT@@/role/*" {
  capabilities = ["read"]
}

path "@@DATABASE_MOUNT@@/roles" {
  capabilities = ["list"]
}

path "@@DATABASE_MOUNT@@/roles/*" {
  capabilities = ["read"]
}

path "@@DATABASE_MOUNT@@/config/*" {
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

path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}

path "sys/storage/raft/configuration" {
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

path "@@KV_MOUNT@@/data/*" {
  capabilities = ["deny"]
}

path "@@KV_MOUNT@@/metadata/*" {
  capabilities = ["deny"]
}

path "@@DATABASE_MOUNT@@/creds/*" {
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
