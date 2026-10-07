# 0.1.2

* Node AMI serves the Vault web UI at `$VAULT_ADDR/ui`. Needs a fresh AMI.

# 0.1.1

* `aws-ec2-vault-cluster` plans on a fresh workspace: AMI looked up by name, NLB listener count known at plan time, vault-admin 0.2.2 (Secrets Manager egress).

# 0.1.0

Modules: `aws-ec2-vault-cluster` 0.1.0, `aws-vault-access` 0.2.0. Needs `vault-admin` 0.2.1 and a fresh node AMI.

* Apps log in as one tenant per request. `aws-vault-access` takes `access = reader | writer` (replaces `policies`) and binds `apps-reader` or `apps-writer`, which can only mint a one-time login for a tenant on `approle-reader` / `approle-writer`. Tenant tokens last 15 minutes. Tenant policies are static; `tenants create` prints no credentials.
* `vault-utils tenants list` lists tenants from the AppRole roles and flags a half-created one.
* `aws-auth` is now `apps-auth`; the cluster function writes app roles on `auth/aws` and `auth/gcp`.
* New outputs `vault_addr`, `user_vault_addr`, `tls_server_name`. Apps receive `VAULT_TLS_SERVER_NAME` (empty without TLS) and `VAULT_TENANT_MOUNT`.
* `vault-utils env --org --stack --env --block` prints shell settings for a Nullstone Vault cluster workspace. `tenants`, `snapshot take|restore`, and `health` use the `vault login` token when `VAULT_TOKEN` is unset.

# 0.0.3

* Initial release
