# 0.4.1

Module: `aws-ec2-vault-cluster` 0.4.1. No new AMI. A cluster that left `instance_type` unset rolls to the new size on its next apply.

* Default `instance_type` is `t3.small`. Vault runs with mlock on and pins its binary in RAM; on a 1 GB `t3.micro` the kernel OOM-killed Vault about six minutes after boot, before bootstrap finished.

# 0.4.0

Module: `aws-ec2-vault-cluster` 0.4.0. Needs a fresh node AMI. The operator policy gains `sys/step-down` and `sys/storage/raft/remove-peer`; a cluster bootstrapped before this release must rewrite it with a break-glass root before the leave step works. A launch-template change rolls the nodes.

* A rolling instance refresh is safe on a one-node cluster. A launch lifecycle hook (`vault-join`) holds a new node until `vault-utils lifecycle` reports it an unsealed, caught-up Raft voter; a node that never joins is abandoned after 30 minutes, which fails the refresh and rolls it back with the old node untouched. A terminate hook (`vault-leave`) holds a departing node until it has stepped down and removed itself from the peer set, so the survivor keeps quorum. Before this, the refresh counted a new node healthy on EC2 status alone and terminated the only copy of the data.

# 0.3.1

Module: `aws-ec2-vault-cluster` 0.3.1. Launch-template change; nodes roll.

* Nodes advertise `api_addr` as the URL clients use: `user_vault_addr` when a subdomain is connected, else `vault_addr`. It was always `https://vault.internal:8200`, so a standby redirect sent browsers to an origin the NLB certificate does not name and the web UI failed with "The request failed and the interceptors did not return an alternative response".

# 0.3.0

Modules: `aws-ec2-vault-cluster` 0.3.0, `aws-vault-access` 0.3.0. No runtime change.

* A `v*` tag push publishes both modules at that version (`.github/workflows/publish.yml`). The cluster module packages `templates/*`.

# 0.2.0

Modules: `aws-ec2-vault-cluster` 0.2.0, `aws-vault-access` 0.3.0. Needs `vault-admin` 0.3.0 and a fresh node AMI. Nothing changes for a cluster outside the shared previews env.

* A cluster launched in the shared previews env (Nullstone env type `PreviewsSharedEnv`) is shared: output `shared = true`, and every tenant belongs to an env. Tenant roles are `<env>.<tenant>`, secrets live under `kv/data/envs/<env>/customers/<tenant>/`, and an app's broker can only log in as its own env's tenants. The cluster function binds each app role to its env through an identity entity; provisioning does the same for tenant roles.
* `aws-vault-access` passes the app's env to the function on a shared cluster and injects `VAULT_ENV` (empty on an unshared cluster).
* `vault-utils tenants create|destroy` take `--env` (required on a shared cluster, refused elsewhere). `tenants list` shows the env. New `envs list` and `envs destroy <env> --yes [--purge-secrets]`. `vault-utils env` prints `SHARED_ENVS=true` for a shared cluster.

# 0.1.2

* Node AMI serves the Vault web UI at `$VAULT_ADDR/ui`. Needs a fresh AMI.
* `aws-ec2-vault-cluster` adds datastore outputs `db_hostname`, `db_port`, `db_endpoint` (`vault://<host>:8200`), `private_urls`, and `public_urls`. The host is the user-facing name, or vault.internal without a subdomain. `private_urls` holds the UI on vault.internal; `public_urls` holds it on the subdomain when one is connected.

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
