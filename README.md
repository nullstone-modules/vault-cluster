# Secrets Vault Cluster

Multi-platform modules to configure a self-hosted Vault cluster on local, AWS, GCP, and Azure

Applications share one Vault. They do not see each other's secrets. Isolation is a path prefix plus ACL policy plus AppRole. Vault CE has no namespaces. This is not Enterprise.

**Dev machine:** `cd local && docker compose up -d --build`. You do not init or unseal by hand.

| Target | Role | Status |
|---|---|---|
| `local/` | Docker Compose | Implemented |
| `aws/aws-ec2-vault-cluster/` | `aws-ec2-vault-cluster` | IAM, SM, SG, AMI, user-data, launch template, internal NLB (TLS), ASG with rolling refresh. |
| `gcp/` | GCP | Not implemented |
| `azure/` | Azure | Not implemented |

There is no `module "vault_cluster" { source = "./${var.cloud}" }` switch. Shared trees talk to Vault only through `VAULT_ADDR` and a token.

## Contents

1. [Scope](#scope)
2. [Architecture](#architecture)
3. [Repository layout](#repository-layout)
4. [Prerequisites](#prerequisites)
5. [Quick start](#quick-start)
6. [Connecting to a cluster](#connecting-to-a-cluster)
7. [Commands](#commands)
8. [Tenant isolation](#tenant-isolation)
9. [Identities](#identities)
10. [Health](#health)
11. [Backup, restore, and disaster recovery](#backup-restore-and-disaster-recovery)
12. [Break-glass](#break-glass)
13. [Testing](#testing)
14. [Troubleshooting](#troubleshooting)
15. [App access](#app-access)
16. [Security](#security)

## Scope

Implemented:

- Vault CE 2.0 (never `-dev`, never Enterprise)
- Docker Compose, Raft on a named volume
- KV v2 at `kv/customers/{tenant}/*`
- Dynamic PostgreSQL credentials in the Go library only (`TestCredentialsMatrix`); the local stack runs no PostgreSQL and never mounts `database/`
- File audit on a volume separate from Raft
- Auto-init (first start) and one-shot Shamir unseal via `vault-utils`
- Isolation tests in Go (`go test`); credentials tests in Go (`TestCredentialsMatrix`)
- `bootstrap aws` (KMS auto-unseal, Secrets Manager tokens), health on 8210, S3 snapshots and S3 restore
- AWS AMI, user-data, internal NLB (TLS 8200, health 8210), `vault.internal`, optional user-facing subdomain SNI, ASG (`cluster_size`), rolling instance refresh on launch-template change gated by lifecycle hooks (join before the old node leaves; leave the peer set before termination)

Not implemented:

- GCP, Azure, Kubernetes
- Vault listener TLS, DR replication

Local unseal submits Shamir shares (5 shares, threshold 3) for laptop use. It is not AWS KMS, Cloud KMS, or Azure Key Vault auto-unseal.

## Architecture

```
Developer
  |
Docker Compose
  |
  +-- Vault
  |     Raft
  |     KV v2
  |     ACL policies
  |     AppRole
  |     Audit
  |
  +-- vault-utils bootstrap   (one-shot: init, unseal, configure)
```

Isolation and credentials tests are Go (`go test ./internal/vaultcluster`). Both run throwaway containers; the credentials suite brings its own PostgreSQL, so the local stack never runs one.

## Repository layout

```
vault-cluster/
├── README.md
├── CHANGELOG.md
├── Dockerfile            vault-utils image
├── cmd/                  Go app entrypoints (vault-utils CLI)
├── internal/vaultcluster/ shared Vault library
├── internal/aws/         AWS adapters (secretsmanager, s3)
├── internal/nsenv/       Nullstone workspace lookup for `vault-utils env`
├── local/                Compose target, snapshots
├── aws/aws-ec2-vault-cluster/   Nullstone AWS module
├── gcp/                  Nullstone Terraform module (not yet implemented)
└── azure/                Nullstone Terraform module (not yet implemented)
```

## Prerequisites

Docker Desktop (Compose v2). Go 1.26 for `go test`. `curl` and `jq` for the manual examples below; Vault CLI is optional except break-glass decode.

Images are pinned by tag and digest in `local/compose.yml` (Vault 2.0, PostgreSQL 18-alpine). Never `latest`.

```bash
docker --version
docker compose version
```

## Quick start

```bash
cd local
docker compose up -d
```

First run:

1. Starts persistent Vault CE
2. Initializes Shamir 5/3
3. Writes keys to `local/.bootstrap/` (gitignored, mode 600)
4. Unseals
5. Enables audit, KV v2, AppRole, policies
6. Revokes the root token

Bootstrap only initializes the cluster. Tenants are created explicitly with `tenants create`.

Later runs skip init if the provisioning token still works. A Vault process restart reseals. Unseal is a one-shot (`docker compose run --rm bootstrap`), not a long-running sidecar. `docker compose up -d` starts Vault and runs bootstrap once, then bootstrap exits.

Then:

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat local/.bootstrap/provisioning.token)
```

Back up `local/.bootstrap/vault-init.json` immediately. Without it this volume cannot be unsealed.

Do not commit `.bootstrap/` or `.env`. Do not run `vault operator unseal`.

## Connecting to a cluster

`vault-utils env` reads the cluster workspace in Nullstone and prints `VAULT_ADDR`, plus `VAULT_TLS_SERVER_NAME` when the load balancer terminates TLS, for your shell. It needs a Nullstone API key (`nullstone set-profile`, or `NULLSTONE_API_KEY`) and a network path into the VPC (VPN, Tailscale subnet router, or similar). It prints nothing sensitive.

```bash
eval "$(vault-utils env --org <org> --stack <stack> --env <env> --block <block>)"
vault status
```

PowerShell: `vault-utils env ... | Out-String | Invoke-Expression`. `--internal` uses `vault.internal` instead of the user-facing name. A warning on stderr means Vault is unreachable from this machine. Any other Nullstone workspace is refused.

The web UI is at `$VAULT_ADDR/ui` on every target. Sign in with a token; root is revoked at bootstrap, so use the operator or provisioning token.

Without `VAULT_TOKEN`, `vault-utils tenants`, `snapshot take|restore`, and `health` use the token saved by `vault login` (the configured `token_helper`, else `~/.vault-token`).

## Commands

Run from `local/` unless noted. Destructive commands require `--yes`.

| Command | Destructive | Purpose |
|---|---|---|
| `docker compose up -d --build` | no | Start, init (first time), unseal, configure |
| `docker compose down` | no | Stop containers. Keeps all data. |
| `docker compose down --volumes --remove-orphans && rm -rf .bootstrap` | yes | Destroys volumes and unseal keys |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants create <id>` | no | Onboard a tenant (no credentials printed) |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants list` | no | List tenants; flags one missing a reader or writer role |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants destroy <id> --yes` | yes (access) | Revoke access; secrets kept |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants destroy <id> --yes --purge-secrets` | yes | Also destroy secret versions |
| `vault-utils tenants create <id> --env <env>` | no | Onboard a tenant for one env (shared cluster; see [Shared cluster](#shared-cluster)) |
| `vault-utils envs list` | no | Envs with tenants (shared cluster) |
| `vault-utils envs destroy <env> --yes [--purge-secrets]` | yes | Offboard every tenant of an env; purge also destroys its secrets |
| `docker compose run --rm bootstrap snapshot take` | no | Raft snapshot plus SHA-256 |
| `docker compose run --rm bootstrap snapshot restore <file> --yes` | yes | Replaces all Vault state |
| `go test -short ./...` | no | Unit tests (tenant ID, policy lint, render, compose lint) — repo root |
| `go test ./internal/vaultcluster` | no | Isolation and credentials (needs Docker) — repo root |
| `go test ./local` | no | Compose runtime conformance (needs Docker) — repo root |
| `vault-utils env --org <org> --stack <stack> --env <env> --block <block>` | no | Shell settings for a Nullstone Vault cluster — your machine |

## Tenant isolation

An app never holds a token that spans tenants. It logs in with its cloud identity and gets a broker token (`apps-reader` or `apps-writer`). For each request it logs in again as one tenant on the matching AppRole mount; that tenant token sees one path prefix and expires after 15 minutes.

Paths (KV v2 requires `data/` and `metadata/`):

```
kv/data/customers/{tenant_id}/*
kv/metadata/customers/{tenant_id}/*
```

`kv/customers/...` matches nothing.

| Piece | Name | Purpose |
|---|---|---|
| AppRole mounts | `approle-reader`, `approle-writer` | One role per tenant, named by the tenant ID. Tokens last 15 minutes and cannot be renewed past that. Secret IDs last 60 seconds and work once. |
| Tenant policies | `tenant-reader`, `tenant-writer`, `tenant-database` | Static, written at bootstrap. The tenant comes from the login role name (`{{identity.entity.aliases.<accessor>.metadata.role_name}}`), so nothing is written per tenant. |
| Broker policies | `apps-reader`, `apps-writer` | Read `role-id` and mint `secret-id` for any onboarded tenant on one mount. No KV, database, or sys access. |

Tenant IDs: `^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$` (3-32 characters). Rejected: `/`, `..`, `*`, `sys`, `data`, `root`, `envs`, and similar reserved names.

### Shared cluster

A cluster launched in the stack's shared previews env (Nullstone env type `PreviewsSharedEnv`) serves every preview env. Its output `shared` is true, its nodes run with `SHARED_ENVS=true`, and every tenant belongs to one env:

```
kv/data/envs/{env}/customers/{tenant_id}/*
kv/metadata/envs/{env}/customers/{tenant_id}/*
```

| Piece | Shared cluster |
|---|---|
| Tenant role name | `{env}.{tenant_id}` on the same two mounts. Env names use the tenant ID character class, so the `.` is unambiguous. |
| Tenant policies | Env and tenant come from entity metadata (`{{identity.entity.metadata.env}}`, `...tenant`). `tenants create --env` writes the roles, one entity per role with that metadata, and an alias on the role's `role_id`. Nothing per env. |
| Broker policies | `auth/approle-<kind>/role/{{identity.entity.metadata.env}}.*` with every parameter denied: an app mints logins only for its own env and cannot change a role. The env comes from the entity the cluster function writes on the app's auth role (see [App access](#app-access)). |
| Provisioning, apps-auth | May create entities and aliases; `policies` and `disabled` are refused, groups and OIDC are denied. |

A cluster anywhere else is unshared and renders the 0.1.x policies unchanged. Converting a cluster between the two is not supported: launch a new one.

```bash
vault-utils tenants create acme-corp --env pr-123
vault-utils tenants list
vault-utils envs list
vault-utils envs destroy pr-123 --yes --purge-secrets
```

`--env` is required on a shared cluster and refused on an unshared one; `vault-utils env` exports `SHARED_ENVS` so the CLI knows which it is talking to. `envs destroy` offboards every tenant of the env; `--purge-secrets` also destroys `kv/metadata/envs/<env>` and needs break-glass.

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat local/.bootstrap/provisioning.token)
cd local
docker compose run --rm -e VAULT_TOKEN bootstrap tenants create acme-corp
```

`tenants create` writes the reader and writer roles (and database roles when credentials are enabled). It prints no credentials; apps mint their own at login. Those roles are the record of truth: `tenants list` reads both mounts and flags a tenant missing from one.

Log in as one tenant, the way an app does per request (the provisioning token may also mint):

```bash
MOUNT=approle-writer TENANT=acme-corp
ROLE_ID=$(vault read -field=role_id auth/$MOUNT/role/$TENANT/role-id)
SECRET_ID=$(vault write -f -field=secret_id auth/$MOUNT/role/$TENANT/secret-id)
TENANT_TOKEN=$(vault write -field=token auth/$MOUNT/login role_id=$ROLE_ID secret_id=$SECRET_ID)
VAULT_TOKEN=$TENANT_TOKEN vault kv put -mount=kv customers/acme-corp/app-config api_key=FAKE-value
```

Offboard (revoke access, keep secrets):

```bash
cd local
docker compose run --rm -e VAULT_TOKEN bootstrap tenants destroy acme-corp --yes
```

Offboard and destroy data (needs break-glass; provisioning cannot read or purge KV):

```bash
cd local
docker compose run --rm -e VAULT_TOKEN bootstrap tenants destroy acme-corp --yes --purge-secrets
```

Cross-tenant, wildcard, and traversal reads return HTTP 403. That is the isolation contract. A 200 on those paths is a breach.

## Identities

| Identity | Role |
|---|---|
| Root | Bootstrap only. Revoked when setup finishes. |
| Provisioning | Create and offboard tenants by writing AppRole and database roles. Cannot write policies; a role can carry only the static tenant policies. Can mint tenant logins, but cannot read tenant secrets with its own token. |
| App broker | `apps-reader` or `apps-writer`, from the app's cloud login. Mints one-tenant logins at its level. Reads nothing itself. |
| Tenant token | One tenant, one level, 15 minutes. The result of a login on `approle-reader` or `approle-writer`. |
| Operator | Health, mounts, snapshots. Can start generate-root (recovery keys still required). Not a tenant secret reader. Cannot restore. |
| Apps auth | Held by the cluster function. Writes app auth roles on the `aws` and `gcp` mounts (`apps-auth` policy). Cannot read tenant secrets. |

## Health

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8200/v1/sys/health
```

| Code | Meaning |
|---|---|
| 200 | Unsealed and active |
| 501 | Uninitialized |
| 503 | Sealed |

After a Vault process restart, expect 503 (sealed). Re-run the one-shot:

```bash
cd local && docker compose run --rm bootstrap
```

Vault fails closed when audit cannot write. If every request is denied:

```bash
cd local && docker compose exec vault sh -c 'ls -la /vault/logs && df -h /vault/logs'
```

## Backup, restore, and disaster recovery

A snapshot is the whole cluster (secrets, policies, tokens). Treat it like Vault itself. A backup that has never been restored is not a backup.

Keep the matching `vault-init.json` with each snapshot. After restore, Vault unseals only with the shares that were current when the snapshot was taken.

Snapshots are written to `local/.bootstrap/backups/` on the host, which the bootstrap container sees as `/bootstrap/backups/`.

### Backup

```bash
cd local
docker compose run --rm bootstrap snapshot take
docker compose run --rm bootstrap snapshot list
docker compose run --rm bootstrap snapshot verify /bootstrap/backups/vault-<stamp>.snap
```

### Restore (destructive)

Restore needs a token with `sys/storage/raft/snapshot-force`. The operator token cannot restore; generate a break-glass root first (see [Break-glass](#break-glass)).

```bash
cd local
docker compose run --rm -e VAULT_TOKEN=<break-glass-root> bootstrap snapshot restore /bootstrap/backups/vault-<stamp>.snap --yes
docker compose run --rm bootstrap
```

### Restore drill

```bash
cd local
docker compose up -d --build
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat .bootstrap/provisioning.token)
docker compose run --rm -e VAULT_TOKEN bootstrap tenants create tenant-a

docker compose run --rm bootstrap snapshot take
cp .bootstrap/vault-init.json /tmp/keys-at-snapshot.json

docker compose run --rm -e VAULT_TOKEN bootstrap tenants create tenant-drill
docker compose down --volumes --remove-orphans && rm -rf .bootstrap

docker compose up -d
cp /tmp/keys-at-snapshot.json .bootstrap/vault-init.json
# generate a break-glass root (see Break-glass), then:
docker compose run --rm -e VAULT_TOKEN=<break-glass-root> bootstrap snapshot restore /bootstrap/backups/vault-<stamp>.snap --yes
docker compose run --rm bootstrap
go test ./internal/vaultcluster -run TestIsolationMatrix
```

Expected: `tenant-a` exists, `tenant-drill` does not, isolation suite passes.

### AWS restore (destructive)

On an AWS node, `snapshot take` and `snapshot list` use the connected S3 bucket. Restore reads an `s3://` URI, checks the SHA-256 object, then force-restores. Needs a token with `sys/storage/raft/snapshot-force`. The operator token cannot restore; mint a break-glass root with that token and the recovery keys first (see [Break-glass](#break-glass)). After restore, Vault seals; KMS auto-unseal brings the node back.

```bash
vault-utils snapshot list
vault-utils snapshot verify s3://<bucket>/vault-snapshots/vault-<stamp>.snap
vault-utils snapshot restore s3://<bucket>/vault-snapshots/vault-<stamp>.snap --yes
```

### If unseal keys are lost

The Raft volume cannot be unsealed. Data is gone. That is Shamir working. Restore from a snapshot that still has its matching keys, or destroy the volumes (`docker compose down --volumes --remove-orphans && rm -rf .bootstrap` in `local/`) and start empty.

## Break-glass

Use only when no routine identity can do the job (read tenant data in an incident, purge secrets, repair audit, restore). Two people. Record why first.

Vault 2.0 requires a token on `sys/generate-root`. Use the operator token. Recovery keys are still required. The operator token cannot restore; the new root can.

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=<operator>
vault operator generate-root -init -format=json
# each of 3 share holders:
vault operator generate-root -nonce=<nonce> <share>
vault operator generate-root -decode=<encoded_token> -otp=<otp>
# one recorded action, then:
vault token revoke -self
```

Cancel an in-flight attempt:

```bash
VAULT_TOKEN=<operator> vault operator generate-root -cancel
```

Clusters that applied the operator policy before this change will still get 403. Do not leave `enable_unauthenticated_access` on. One-shot: add `enable_unauthenticated_access = ["generate-root"]` to `vault.hcl`, SIGHUP, generate-root, write the current operator policy with the new root, remove that line, SIGHUP, revoke the root.

## Testing

```bash
go test -short ./...

go test ./internal/vaultcluster

go test ./local
```

In `local/`, `TestLocalComposeStatic` lints `compose.yml` (digest pins, no dev mode, loopback-only ports). `TestLocalComposeRuntime` brings up an isolated copy of the Compose stack and verifies the one-shot bootstrap, Shamir over Raft, audit/Raft volume separation, and persistence across a restart. It never touches your dev stack or `local/.bootstrap/`.

Denials must be HTTP 403. A 404 is a different failure.

### AWS module (`aws/aws-ec2-vault-cluster/`)

OpenTofu in this directory is connections, IAM, Secrets Manager, security groups, AMI lookup, and user-data. `go test ./internal/aws/...` covers the SM KeyStore and S3 snapshot helpers. `go test ./internal/vaultcluster` covers Raft health, leader check, and cron parse. There is no live-AWS test in CI.

From `aws/aws-ec2-vault-cluster/`:

```bash
tofu fmt -check
tofu init -backend=false
tofu validate
```

`tofu plan` and `tofu apply` need a Nullstone workspace plus AWS credentials. Without them, plan fails with `no nullstone workspace 0/0/0` and missing AWS credentials. That is expected. Do not `tofu apply` from this repo unless you intend to create IAM, Secrets Manager, and security groups.

To plan against real connections:

1. Install the Nullstone CLI (`ns`).
2. In an AWS Nullstone stack, attach this module and connect:
   - `network` → `network/aws/vpc` (same VPC pattern as Nullstone EC2 apps)
   - `snapshots_bucket` → `datastore/aws/s3` (snapshot bucket)
   - `unseal_key` → `datastore/aws/kms` (dedicated unseal key, not the bucket SSE key)
   - optional `subdomain` → `subdomain/aws/route53` (user-facing TLS name on the NLB)
3. Run workspace preview/plan in Nullstone so `ns_connection` outputs resolve.
4. In the plan, expect IAM, four Secrets Manager secrets (`init`, `provisioning`, `operator`, `apps-auth`; `protect_platform_secrets` default true, 30-day recovery), node and NLB security groups, a launch template, an ACM cert for `vault.internal`, an alias on the network internal zone, an internal NLB with TLS on 8200 (health 8210), and an ASG of `cluster_size` (`max_size` is `cluster_size + 1` for surge). A connected subdomain adds a user-facing cert (SNI) and alias. A launch-template change starts a rolling instance refresh: one extra node joins, then one old node leaves. Clients use output `vault_addr` (or `user_vault_addr`) with `tls_server_name`. The NLB terminates TLS only with a connected subdomain; Vault nodes listen HTTP.

Bake the node AMI (x86_64, matches default `t3.micro`) from `vault-node/`:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o vault-node/vault-utils ./cmd/vault-utils
cd vault-node
packer init .
packer build -var region="$AWS_REGION" -var-file=ami.pkrvars.hcl vault.pkr.hcl
```

`.github/workflows/build-ami.yml` runs the same bake on demand or on a push to `main` that touches the image inputs. It assumes the Packer IAM role through GitHub OIDC. Role ARN, bake region, and public subnet come from the Nullstone `aws-packer-builder` workspace (`stack=internal`, `env=local`). Copy regions, org launch ARNs, and public launch (`ami_groups = ["all"]`) are `vault-node/ami.pkrvars.hcl`. See [nullstone-modules/aws-packer-builder-github](https://github.com/nullstone-modules/aws-packer-builder-github).

Repo configuration for the bake:

- secret `NULLSTONE_API_KEY`

The module looks up `tag:Name = nullstone-vault` from `ami_owner` (default `522657839841`, the account that publishes the Vault AMI). Set `ami_owner` to `self` when this account bakes the image. Override with `ami` for a specific image or architecture.

`vault-node/files/` holds the cloud-neutral image content: base `vault.hcl` and the systemd units. `vault-node/aws/vault-node-configure` is the only AWS-specific piece, and other clouds add a sibling directory. The bake installs Vault CE 2.0, `vault-utils`, and that content, then enables every unit.

On boot, `vault-configure.service` runs after cloud-init, writes `/etc/vault.d/cloud.hcl` and `/etc/vault.d/node.env`, and exits. Systemd ordering then starts Vault, bootstrap, health, and snapshots. User-data only writes `/etc/vault.d/vault-utils.env`.

## Troubleshooting

| Symptom | Action |
|---|---|
| Docker daemon is not running | Start Docker Desktop |
| Port already allocated | Change `VAULT_HOST_PORT` in `local/.env` |
| Initialized but `.bootstrap` missing | Restore `vault-init.json`, or destroy volumes and start empty |
| Health 503 | Vault is sealed. Run `docker compose run --rm bootstrap` in `local/` |
| Permission denied on tenant create | Use the provisioning token, not operator |
| generate-root permission denied | Vault 2.0 needs the operator token. See [Break-glass](#break-glass). |
| Permission denied on tenant secrets | Expected for provisioning |
| Everything denied | Audit volume full or unwritable |

## App access

`nullstone/aws-vault-access` connects an app to `aws-ec2-vault-cluster`.

Connect `vault` to the cluster. The app module must expose `security_group_id` and its IAM role name.

`role_name` is optional. If empty, the Vault role is `<app-name>-<resource-suffix>`. `access` is `reader` (default) or `writer`; the capability binds the matching broker policy (`apps-reader` or `apps-writer`) and nothing else. The app receives `VAULT_ADDR`, `VAULT_ROLE`, `VAULT_TLS_SERVER_NAME` (empty unless the NLB terminates TLS), `VAULT_TENANT_MOUNT`, and `VAULT_ENV`. During apply the capability calls the cluster function with `method = "aws"` and the app IAM role ARN; the function binds only that principal to that role. The same function serves GCP service accounts (`method = "gcp"`) for a future GCP cluster module.

On a shared cluster (output `shared`), the capability also sends the app's Nullstone env name. The function then gives the auth role an identity entity with that env, which the broker policies template, and `VAULT_ENV` carries it to the app. On an unshared cluster `VAULT_ENV` is empty and the function never touches identity. Leave `role_name` empty on a shared cluster: a fixed name collides across preview envs and the function refuses the second binding.

The app does not receive a Vault token. At startup it logs in with its IAM role, then for every request it logs in again as one tenant. The role is `$TENANT`, or `$VAULT_ENV.$TENANT` when `VAULT_ENV` is set; the KV path is `customers/$TENANT/...`, or `envs/$VAULT_ENV/customers/$TENANT/...`:

```bash
vault login -method=aws role="$VAULT_ROLE"

ROLE="${VAULT_ENV:+$VAULT_ENV.}$TENANT"
ROLE_ID=$(vault read -field=role_id auth/$VAULT_TENANT_MOUNT/role/$ROLE/role-id)
SECRET_ID=$(vault write -f -field=secret_id auth/$VAULT_TENANT_MOUNT/role/$ROLE/secret-id)
TENANT_TOKEN=$(vault write -field=token auth/$VAULT_TENANT_MOUNT/login role_id=$ROLE_ID secret_id=$SECRET_ID)
```

The broker token reads no secrets. The tenant token is good for that tenant only, for 15 minutes. A login as any other role is denied. The operator token is not injected.

### Shared cluster in Nullstone

Launch the cluster block in the stack's shared previews env (named `previews-shared` below). In `.nullstone/previews.yml`, point each app's capability at it; everywhere else the app uses its own env's cluster:

```yaml
version: "0.1"

apps:
  api:
    capabilities:
      vault:
        connections:
          vault: previews-shared.vault
```

Create the preview env's tenants with `vault-utils tenants create <id> --env <env>` and remove them with `vault-utils envs destroy <env> --yes --purge-secrets` when the env goes away.

## Security

- Host ports bind to `127.0.0.1` only
- AWS NLB terminates TLS only when a subdomain is connected, with a certificate for that name. Clients on `vault.internal` verify it with `VAULT_TLS_SERVER_NAME` (output `tls_server_name`). Without a subdomain the NLB forwards plain TCP. Vault nodes listen HTTP on 8200
- No Vault `-dev` mode
- Root token revoked after bootstrap
- Unseal keys, tokens, and `.env` are gitignored (mode 600). Never printed to logs
- AWS platform secrets (`init`, `provisioning`, `operator`, `apps-auth`) have `prevent_destroy` (var `protect_platform_secrets`, default true) and a 30-day recovery window. Set the var to false before destroying the workspace.
- Audit values are HMAC'd. Raw secrets must not appear in the audit log
- Provisioning cannot read tenant KV with its own token, write policies, or attach anything but the static tenant policies to a role (`allowed_parameters`)
- Apps hold no tenant access directly: a broker token mints a 15-minute, one-tenant token per login
- Operator cannot read tenant KV
- Local Compose sets `disable_mlock = true` because Docker and GitHub Actions cannot mlock. Production hosts should use `IPC_LOCK`
- Dynamic credentials (Go library and tests only): bounded TTL, revoke drops the Postgres role, residue scan expects zero leftover `v-*` roles
