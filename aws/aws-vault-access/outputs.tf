output "env" {
  value = [
    {
      name  = "VAULT_ADDR"
      value = local.vault_addr
    },
    {
      name  = "VAULT_ROLE"
      value = local.role_name
    }
  ]
}
