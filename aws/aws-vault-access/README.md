# aws-vault-access

Capability that connects an app to `nullstone/aws-ec2-vault-cluster`. It opens the security groups, binds the app's IAM role to one Vault auth role, and injects the settings the app needs. The app receives no token.

## What it does at apply

1. Adds egress on the app security group and ingress on the cluster NLB security group for the Vault API port. The app module must expose `security_group_id` and its IAM role name in `app_metadata`.
2. Calls the cluster function with the app IAM role ARN. The function writes `auth/aws/role/<VAULT_ROLE>` bound to exactly that principal, granting `apps-reader` or `apps-writer` and nothing else. On a shared cluster it also records the app's Nullstone env on the role's identity entity.
3. Injects environment variables:

| Variable | Value |
|---|---|
| `VAULT_ADDR` | `http(s)://vault.internal:8200` |
| `VAULT_TLS_SERVER_NAME` | Name to verify in the certificate when `VAULT_ADDR` is https; empty otherwise. The certificate names the user-facing host, not `vault.internal`. |
| `VAULT_ROLE` | The auth role to log in with |
| `VAULT_TENANT_MOUNT` | `approle-reader` or `approle-writer`, from `access` |
| `VAULT_ENV` | The app's env on a shared cluster; empty otherwise |

## Variables

| Name | Default | Purpose |
|---|---|---|
| `access` | `reader` | `reader`: read tenant KV. `writer`: full KV lifecycle plus dynamic database credentials. |
| `role_name` | empty | Vault auth role name. Empty gives `<app>-<suffix>`, unique per workspace. Leave it empty on a shared cluster: a fixed name collides across preview envs and the function refuses the second binding. |

## Gaining access from the app

Two logins. The first is once per process, the second is once per tenant request.

**1. Broker login with the IAM role.** Vault verifies a signed `sts:GetCallerIdentity` from the app's AWS credentials (ECS task role, Lambda execution role, or EC2 instance profile). This yields a broker token that can mint tenant logins and read nothing.

```bash
vault login -method=aws role="$VAULT_ROLE"
```

With an SDK, call `auth/aws/login` with `role` and the signed STS request; every Vault client library has an AWS IAM auth helper.

**2. Tenant login per request.** Read the role ID, mint a one-time secret ID, log in. The role is `$TENANT`, or `$VAULT_ENV.$TENANT` on a shared cluster. The resulting token sees one tenant for 15 minutes.

```bash
ROLE="${VAULT_ENV:+$VAULT_ENV.}$TENANT"
ROLE_ID=$(vault read -field=role_id "auth/$VAULT_TENANT_MOUNT/role/$ROLE/role-id")
SECRET_ID=$(vault write -f -field=secret_id "auth/$VAULT_TENANT_MOUNT/role/$ROLE/secret-id")
TENANT_TOKEN=$(vault write -field=token "auth/$VAULT_TENANT_MOUNT/login" role_id="$ROLE_ID" secret_id="$SECRET_ID")
```

**3. Use the tenant token.** Paths are `customers/$TENANT/...`, or `envs/$VAULT_ENV/customers/$TENANT/...` on a shared cluster.

```bash
PREFIX="${VAULT_ENV:+envs/$VAULT_ENV/}customers/$TENANT"
VAULT_TOKEN=$TENANT_TOKEN vault kv get -mount=kv "$PREFIX/app-config"
# writer only:
VAULT_TOKEN=$TENANT_TOKEN vault kv put -mount=kv "$PREFIX/app-config" api_key=...
VAULT_TOKEN=$TENANT_TOKEN vault read "database/creds/tenant-$ROLE-readwrite"
```

## Token lifetimes

| Token | TTL | Renewable | Do |
|---|---|---|---|
| Broker (from the IAM login) | Vault default, 32 days unless the mount is tuned | Yes | Cache it for the process. Renew with `auth/token/renew-self` before expiry, or log in again. Treat a 403 on `role-id` or `secret-id` as expired and log in again. |
| Secret ID | 60 seconds, one use | No | Mint right before login. Never cache or share it. |
| Tenant token | 15 minutes, hard maximum | Renew keeps it under 15 minutes total | Cache per tenant for up to 15 minutes if you like, then log in again. Never hold one past a request on behalf of a different tenant. |
| Database credentials (writer) | Lease from the cluster (`DatabaseTTL`) | Yes, up to the max | Renew with `sys/leases/renew` while in use; revoke with `sys/leases/revoke` when done. They outlive the tenant token that issued them until their lease ends. |

## Gotchas

- **The tenant must exist.** `tenants create <id>` (with `--env <env>` on a shared cluster) writes the roles. Until then `role-id` returns 404. There is no self-service onboarding from the app.
- **One tenant per token.** Reading another tenant's path, a parent path, a wildcard, or the other env returns 403. That is the contract, not a misconfiguration. Do not work around it by logging in as a broad tenant.
- **Shared cluster paths differ.** When `VAULT_ENV` is non-empty, both the role name and the KV prefix change. Branch on the variable rather than hard-coding either form.
- **Reader versus writer is the mount.** A `reader` app cannot log in on `approle-writer` even for its own tenant; `access = "writer"` changes both the broker policy and `VAULT_TENANT_MOUNT`.
- **TLS on `vault.internal`.** When `VAULT_ADDR` is https, set the TLS server name to `VAULT_TLS_SERVER_NAME` (the Vault CLI and SDKs read the variable). Without it the certificate fails to verify because it names the user-facing host.
- **IAM login region.** The STS request is signed for a region. Log in from the cluster's region, or pass `region=` to the login (CLI: `-region`, SDKs: the auth helper's region option). A signature error on login is almost always this.
- **Recreating the app IAM role.** The auth role binds the principal's unique ID. If the app's IAM role is destroyed and recreated with the same name, logins fail until the capability re-applies and the function rewrites the binding.
- **Task role, not execution role.** On ECS the signing credentials must be the task role that `app_metadata.role_name` names. The execution role is not bound.
- **No token in the environment.** Nothing injects `VAULT_TOKEN`. An app that expects one at startup must perform the IAM login itself.
- **Do not log secret IDs or tokens.** Vault HMACs them in its audit log; the app's logs should contain neither.
