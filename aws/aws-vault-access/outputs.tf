output "env" {
  value = [
    {
      name  = "VAULT_ADDR"
      value = local.vault_addr
    }
  ]
}
