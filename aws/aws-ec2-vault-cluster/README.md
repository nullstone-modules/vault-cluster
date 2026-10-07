# aws-ec2-vault-cluster

Self-hosted Vault CE on an EC2 Auto Scaling Group: Raft storage, KMS auto-unseal, internal NLB on `vault.internal`, S3 snapshots, platform tokens in Secrets Manager. Apps connect through `nullstone/aws-vault-access`. Operations, backup, and break-glass are in the [repo README](../../README.md).

## Connections

| Name | Contract | Purpose |
|---|---|---|
| `network` | `network/aws/vpc` | VPC, private subnets, internal zone |
| `snapshots_bucket` | `datastore/aws/s3` | Raft snapshots and the init claim |
| `unseal_key` | `datastore/aws/kms` | Auto-unseal key (not the bucket key) |
| `subdomain` (optional) | `subdomain/aws/route53` | User-facing name; the NLB terminates TLS only with this |

## Variables

| Name | Default | Purpose |
|---|---|---|
| `cluster_size` | `1` | Odd number of nodes |
| `instance_type` | `t3.micro` | |
| `ami`, `ami_owner` | latest `nullstone-vault` AMI from `522657839841` | Set `ami_owner = "self"` when this account bakes it |
| `backup_schedule` | empty | Cron for S3 snapshots; empty disables |
| `protect_platform_secrets` | `true` | `prevent_destroy` on the token secrets; set false before destroying the workspace |

## Shared cluster

Launched in the stack's shared previews env (Nullstone env type `PreviewsSharedEnv`), the cluster is shared: output `shared` is true and the nodes run with `SHARED_ENVS=true`. Every tenant then belongs to an env. Anywhere else the cluster is unshared and nothing below changes from 0.1.x. A cluster cannot switch modes; launch a new one.

## Where a tenant's secrets live

| Cluster | KV path | AppRole role name | Database roles |
|---|---|---|---|
| Unshared | `kv/data/customers/{tenant}/*` | `{tenant}` | `tenant-{tenant}-readonly`, `-readwrite` |
| Shared | `kv/data/envs/{env}/customers/{tenant}/*` | `{env}.{tenant}` | `tenant-{env}.{tenant}-readonly`, `-readwrite` |

KV v2: `metadata/`, `delete/`, `undelete/`, and `destroy/` mirror the `data/` tree. Tenant IDs and env names match `^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$`; neither contains `.`, so the role name splits unambiguously.

## How a tenant token is scoped

The tenant policies (`tenant-reader`, `tenant-writer`, `tenant-database`) are static, written once at bootstrap, and never name a tenant. A Vault ACL template fills the tenant in from the identity of the token that logged in:

- Unshared: the AppRole role name (`{{identity.entity.aliases.<accessor>.metadata.role_name}}`), which only provisioning can set.
- Shared: entity metadata `env` and `tenant` (`{{identity.entity.metadata.env}}`), written by provisioning when it creates the tenant's roles and aliased on each role's `role_id`.

Apps cannot reach the identity API, so neither value can be forged. A token whose entity lacks the metadata matches no path. Cross-tenant, cross-env, parent, wildcard, and traversal requests return 403; a 200 there is a breach.

## Apps

An app never holds a token that spans tenants.

1. It logs in with its IAM role (`vault login -method=aws role=$VAULT_ROLE`) and gets a broker token holding `apps-reader` or `apps-writer`. That token reads no secrets.
2. For each request it mints a one-time secret-id for one tenant's role on `$VAULT_TENANT_MOUNT` (`approle-reader` or `approle-writer`) and logs in again. The result is a 15 minute token for that tenant only.

On a shared cluster the broker policy is templated on the app entity's `env`, which the cluster function writes when `aws-vault-access` binds the app: `auth/approle-<kind>/role/{{identity.entity.metadata.env}}.*` with every parameter denied. The app can mint logins only for its own env's tenants and cannot alter a role. `VAULT_ENV` tells the app its env; it is empty on an unshared cluster, where the role is `$TENANT` and the path `customers/$TENANT`.

## Humans

There is no per-person login. A human uses a platform token from Secrets Manager, or the token `vault login` saved, with `vault-utils` (`vault-utils env --org --stack --env --block` prints `VAULT_ADDR`, `VAULT_TLS_SERVER_NAME`, and `SHARED_ENVS`):

| Token | Can | Cannot |
|---|---|---|
| Provisioning (`provisioning_secret_arn`) | `tenants create / list / destroy`, `envs list / destroy`; on a shared cluster also the tenant entities. Mint any tenant's login. | Read tenant KV with its own token, write policies, give an entity policies, touch groups or OIDC |
| Operator (`operator_secret_arn`) | Health, mounts, snapshots, start generate-root | Read tenant KV, restore |
| Break-glass root | Restore, purge, read tenant data in an incident | Kept: minted from operator plus recovery keys, two people, revoked after one action |

The function's `apps-auth` token writes app auth roles and, on a shared cluster, app entities. It cannot read tenant secrets.

Per-tenant CLI on a shared cluster takes `--env`; on an unshared one `--env` is refused.

Nodes advertise `api_addr` as `user_vault_addr` when a subdomain is connected, else `vault_addr`. Redirects from a standby therefore land on the origin clients already use. Clients on `vault.internal` set `VAULT_TLS_SERVER_NAME` to `tls_server_name`.

## Outputs

`vault_addr`, `user_vault_addr`, `tls_server_name`, `vault_fqdn`, `user_fqdn`, `vault_api_port`, `nlb_security_group_id`, `admin_function_name`, `shared`, `operator_secret_arn`, `provisioning_secret_arn`, `db_hostname`, `db_port`, `db_endpoint`, `private_urls`, `public_urls`, plus node identifiers (`role_name`, `instance_profile_name`, `security_group_id`, `autoscaling_group_name`, `ami_id`).

## Testing

```bash
tofu fmt -check
tofu init -backend=false
tofu test
```

`tofu test` mocks every provider. A real plan needs a Nullstone workspace and AWS credentials.
