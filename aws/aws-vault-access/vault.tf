data "ns_connection" "vault" {
  name     = "vault"
  contract = "datastore/aws/vault:*"
}

locals {
  vault_fqdn            = data.ns_connection.vault.outputs.vault_fqdn
  nlb_security_group_id = data.ns_connection.vault.outputs.nlb_security_group_id
  admin_function_name   = data.ns_connection.vault.outputs.admin_function_name
  vault_api_port        = tonumber(data.ns_connection.vault.outputs.vault_api_port)
  vault_addr            = data.ns_connection.vault.outputs.vault_addr
  tls_server_name       = try(data.ns_connection.vault.outputs.tls_server_name, "")
}

# shared is true for a cluster in previews-shared (0.2.0+). Older clusters have no such output: not shared.
locals {
  shared = try(tobool(data.ns_connection.vault.outputs.shared), false)
  env    = local.shared ? data.ns_workspace.this.env_name : ""
}
