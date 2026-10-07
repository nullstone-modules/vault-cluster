output "env" {
  value = [
    {
      name  = "VAULT_ADDR"
      value = local.vault_addr
    },
    {
      name  = "VAULT_ROLE"
      value = local.role_name
    },
    {
      name  = "VAULT_TLS_SERVER_NAME"
      value = local.tls_server_name
    },
    {
      name  = "VAULT_TENANT_MOUNT"
      value = local.tenant_mount
    },
    {
      name  = "VAULT_ENV"
      value = local.env
    }
  ]
}
