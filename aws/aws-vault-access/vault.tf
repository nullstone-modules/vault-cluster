data "ns_connection" "vault" {
  name     = "vault"
  contract = "datastore/aws/vault:*"
}

locals {
  vault_fqdn            = data.ns_connection.vault.outputs.vault_fqdn
  nlb_security_group_id = data.ns_connection.vault.outputs.nlb_security_group_id
  admin_function_name   = data.ns_connection.vault.outputs.admin_function_name
  vault_api_port        = tonumber(data.ns_connection.vault.outputs.vault_api_port)
  # Clusters before vault_addr existed listened plain TCP without a subdomain; with one, they need an upgrade.
  vault_addr      = coalesce(try(data.ns_connection.vault.outputs.vault_addr, ""), "http://${local.vault_fqdn}:${local.vault_api_port}")
  tls_server_name = try(data.ns_connection.vault.outputs.tls_server_name, "")
}
