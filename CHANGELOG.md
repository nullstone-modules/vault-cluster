# 0.1.0 (Unreleased)
* Initial release
* Human admin access over AWS IAM auth: per-level IAM roles and groups (`admin_group_names`, `admin_role_arns`), `admin_principals` for SSO and cross-account principals, `admin-*` Vault roles written by the nodes at boot
* `vault-utils env` prints `VAULT_ADDR` (and `VAULT_TLS_SERVER_NAME`) from a Nullstone cluster workspace
* `vault-utils` falls back to the `vault login` token when `VAULT_TOKEN` is unset
* `vault-utils admins reconcile` and `vault-utils version`
* Outputs `vault_addr`, `user_vault_addr`, `tls_server_name`; `aws-vault-access` uses them and reserves `admin-*` role names
* `health serve` renews the provisioning and aws-auth tokens, which previously expired after 24h
* Release workflow: binaries with SHA-256 checksums on GitHub Releases, image at `ghcr.io/nullstone-modules/vault-utils`
