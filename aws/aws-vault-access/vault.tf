data "ns_connection" "vault" {
  name     = "vault"
  contract = "datastore/aws/vault:*"
}

locals {
  vault_fqdn            = data.ns_connection.vault.outputs.vault_fqdn
  nlb_security_group_id = data.ns_connection.vault.outputs.nlb_security_group_id
  admin_function_name   = data.ns_connection.vault.outputs.admin_function_name
  vault_api_port        = tonumber(data.ns_connection.vault.outputs.vault_api_port)
  vault_addr            = "http://${local.vault_fqdn}:${local.vault_api_port}"
}
