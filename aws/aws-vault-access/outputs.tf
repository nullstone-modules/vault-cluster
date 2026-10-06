output "env" {
  value = concat([
    {
      name  = "VAULT_ADDR"
      value = local.vault_addr
    },
    {
      name  = "VAULT_ROLE"
      value = local.role_name
    }
  ], local.tls_server_name == "" ? [] : [{ name = "VAULT_TLS_SERVER_NAME", value = local.tls_server_name }])
}
