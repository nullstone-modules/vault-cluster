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
6. [Getting started (admins)](#getting-started-admins)
7. [Commands](#commands)
8. [Tenant isolation](#tenant-isolation)
9. [Identities](#identities)
10. [Health](#health)
11. [Backup, restore, and disaster recovery](#backup-restore-and-disaster-recovery)
12. [Break-glass](#break-glass)
13. [Testing](#testing)
14. [Troubleshooting](#troubleshooting)
15. [App access](#app-access)
16. [Setting up admins](#setting-up-admins)
17. [Security](#security)

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
- AWS AMI, user-data, internal NLB (TLS 8200, health 8210), `vault.internal`, optional user-facing subdomain SNI, ASG (`cluster_size`), rolling instance refresh on launch-template change

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

`local/compose.yml` builds `vault-utils` from this tree. Released binaries and `ghcr.io/nullstone-modules/vault-utils` images come from pushing a `v*` tag (`.github/workflows/release.yml`).

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

## Getting started (admins)

For an `aws-ec2-vault-cluster`. Admins log in with their own AWS identity. Nobody handles the `provisioning` or `operator` token.

1. **Prerequisites**
   - Membership in the cluster's admin IAM group (output `admin_group_names`), or a principal in `admin_principals`. Ask the cluster owner ([Setting up admins](#setting-up-admins)).
   - A network path into the VPC. The NLB is internal: use a VPN, a Tailscale subnet router, or similar.
   - Vault CLI and AWS CLI v2.
   - `vault-utils` from [Releases](https://github.com/nullstone-modules/vault-cluster/releases). Check it against `SHA256SUMS`.
     Or run `ghcr.io/nullstone-modules/vault-utils:<tag>@sha256:<digest>` (digest in the release run summary). Pin by digest.
   - A Nullstone API key (`nullstone set-profile`, or `NULLSTONE_API_KEY`).

2. **Connect**

   ```bash
   eval "$(vault-utils env --org <org> --stack <stack> --env <env> --block <block>)"
   vault status
   ```

   PowerShell: `vault-utils env ... | Out-String | Invoke-Expression`. `--internal` uses `vault.internal` instead of the user-facing name. `env` warns on stderr if Vault is unreachable.

3. **Log in**

   Group members add an AWS profile. The session name must be your IAM user name. MFA is required by default.

   ```ini
   # ~/.aws/config
   [profile vault-tenants]
   source_profile    = default
   role_arn          = <admin_role_arns["tenants"]>
   role_session_name = <your IAM user name>
   mfa_serial        = arn:aws:iam::<account>:mfa/<device>
   ```

   ```bash
   eval "$(aws configure export-credentials --profile vault-tenants --format env)"
   vault login -method=aws role=admin-tenants
   vault token lookup
   ```

   `token lookup` shows `policies [default provisioning]` and a 1h TTL (8h max). SSO users run `aws sso login --profile <p>`, export that profile, and log in to `role=admin-<key>`. Leave `region` unset: the auth mount verifies against the global STS endpoint.

4. **First tasks**

   ```bash
   vault-utils health
   vault-utils tenants create acme-corp
   ```

   The reader and writer `role_id` and `secret_id` print once. Write a secret as the writer:

   ```bash
   TENANT_TOKEN=$(vault write -field=token auth/approle/login role_id=<writer role_id> secret_id=<writer secret_id>)
   VAULT_TOKEN=$TENANT_TOKEN vault kv put -mount=kv customers/acme-corp/app-config api_key=FAKE-value
   vault-utils tenants destroy acme-corp --yes
   ```

   `vault-utils` uses the `vault login` token when `VAULT_TOKEN` is unset.

5. **When things fail**

   | Symptom | Cause |
   |---|---|
   | Timeout or connection refused | No network path into the VPC, or the VPN does not route the VPC CIDR |
   | x509 certificate error with `--internal` | `VAULT_TLS_SERVER_NAME` unset. Re-run `vault-utils env` |
   | `role "admin-…" could not be found` | No such admin role. Check the name, or the cluster has not rolled since `admin_principals` changed |
   | 403 on `vault login` | Your AWS identity is not the bound principal: wrong profile, session name is not your IAM user name, or not in the group |
   | `AccessDenied` from `aws configure export-credentials` | Not in the IAM group, or no MFA |
   | 403 after login | That access level does not allow it. `vault token lookup` shows your policies |
   | 503 sealed | Nodes cannot reach KMS. Escalate to the cluster owner |

6. **What admins cannot do**

   Read or purge tenant secrets, restore snapshots, write policies or auth roles, or mint tokens. Use [Break-glass](#break-glass).

## Commands

Run from `local/` unless noted. Destructive commands require `--yes`.

| Command | Destructive | Purpose |
|---|---|---|
| `docker compose up -d --build` | no | Start, init (first time), unseal, configure |
| `docker compose down` | no | Stop containers. Keeps all data. |
| `docker compose down --volumes --remove-orphans && rm -rf .bootstrap` | yes | Destroys volumes and unseal keys |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants create <id>` | no | Onboard a tenant |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants destroy <id> --yes` | yes (access) | Revoke access; secrets kept |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenants destroy <id> --yes --purge-secrets` | yes | Also destroy secret versions |
| `docker compose run --rm bootstrap snapshot take` | no | Raft snapshot plus SHA-256 |
| `docker compose run --rm bootstrap snapshot restore <file> --yes` | yes | Replaces all Vault state |
| `go test -short ./...` | no | Unit tests (tenant ID, policy lint, render, compose lint) — repo root |
| `go test ./internal/vaultcluster` | no | Isolation and credentials (needs Docker) — repo root |
| `go test ./local` | no | Compose runtime conformance (needs Docker) — repo root |
| `vault-utils env --org <org> --stack <stack> --env <env> --block <block>` | no | Shell settings for an AWS cluster — admin machine |
| `vault-utils admins reconcile` | yes (revokes unlisted `admin-*`) | Re-apply `VAULT_ADMINS_FILE` — AWS node |
| `vault-utils version` | no | Build version |

## Tenant isolation

Paths (KV v2 requires `data/` and `metadata/`):

```
kv/data/customers/{tenant_id}/*
kv/metadata/customers/{tenant_id}/*
```

`kv/customers/...` matches nothing.

Tenant IDs: `^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$` (3-32 characters). Rejected: `/`, `..`, `*`, `sys`, `data`, `root`, and similar reserved names.

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat local/.bootstrap/provisioning.token)
cd local
docker compose run --rm -e VAULT_TOKEN bootstrap tenants create acme-corp
```

`role_id` and `secret_id` print once and are not stored. Re-issue a secret_id if lost.

Write as the tenant writer:

```bash
curl -s -H "X-Vault-Token: ${TENANT_TOKEN}" \
  -X POST --data '{"data":{"api_key":"FAKE-value"}}' \
  "${VAULT_ADDR}/v1/kv/data/customers/acme-corp/app-config"
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
| Provisioning | Create and offboard tenants. Cannot read tenant secrets. Automation and break-glass only. |
| Tenant AppRole | One reader and one writer per tenant. |
| Operator | Health, mounts, snapshots. Can start generate-root (recovery keys still required). Not a tenant secret reader. Cannot restore. Automation and break-glass only. |
| AWS auth | Writes AWS auth roles: app roles through the cluster function (which refuses `admin-*`), admin roles from the nodes at boot. Cannot read tenant secrets. |
| Human admin (AWS auth) | `admin-<level>` role bound to an IAM principal. Gets the provisioning and/or operator policy; 1h tokens, 8h max. See [Setting up admins](#setting-up-admins). |

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
4. In the plan, expect IAM, three Secrets Manager secrets (`protect_platform_secrets` default true, 30-day recovery), node and NLB security groups, a launch template, an alias on the network internal zone, an internal NLB on 8200 (health 8210), an admin IAM role and group per access level, and an ASG of `cluster_size` (`max_size` is `cluster_size + 1` for surge). A connected subdomain adds a user-facing cert (SNI) and alias. A launch-template change starts a rolling instance refresh: one extra node joins, then one old node leaves. Clients use output `vault_addr` (or `user_vault_addr`). The NLB terminates TLS only with a connected subdomain; Vault nodes listen HTTP.

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

`vault-node/files/` holds the cloud-neutral image content: base `vault.hcl` and the systemd units. `vault-node/aws/vault-node-configure` is the only AWS-specific piece, and other clouds add a sibling directory. The bake installs Vault CE 2.0, `vault-utils`, and that content, then enables every unit.

On boot, `vault-configure.service` runs after cloud-init, writes `/etc/vault.d/cloud.hcl` and `/etc/vault.d/node.env`, and exits. Systemd ordering then starts Vault, bootstrap, health, and snapshots. User-data only writes `/etc/vault.d/vault-utils.env`. Override with `ami` when using a different architecture.

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

`role_name` is optional. If empty, the Vault role is `<app-name>-<resource-suffix>`. The app receives `VAULT_ADDR` and `VAULT_ROLE`, plus `VAULT_TLS_SERVER_NAME` when the NLB terminates TLS. `admin-*` role names are reserved. During apply the capability calls the cluster function, which binds only that IAM role to that role.

The app does not receive a Vault token. At startup it logs in with its IAM role:

```bash
vault login -method=aws role="$VAULT_ROLE"
```

A login as any other role is denied. The operator token is not injected.

## Setting up admins

For whoever owns the `aws-ec2-vault-cluster` workspace. The module creates one IAM group and one IAM role per access level. Vault role `admin-<level>` is bound to that IAM role.

| Access | Vault role | Policy | Allows |
|---|---|---|---|
| `tenants` | `admin-tenants` | `provisioning` | Onboard and offboard tenants |
| `operator` | `admin-operator` | `operator` | Health, mounts, snapshots, start generate-root |

Neither level reads tenant secrets. Tokens last 1h (8h max) and are not periodic.

**Grant:** add the IAM user to the group in output `admin_group_names`. Groups are named `<stack>-<env>-<block ref>-<suffix>-vault-<level>`, so clusters sharing an AWS account do not collide. **Revoke:** remove them. Neither needs an apply. The role trust requires the session name to equal the IAM user name, and MFA unless `admin_require_mfa = false`.

Principals that cannot join an IAM group (Identity Center permission sets, roles in other accounts, a single IAM user) go in `admin_principals`. Each key becomes `admin-<key>`:

```hcl
admin_principals = {
  "sso-vault-admins" = { principal_arn = "arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/AWSReservedSSO_VaultAdmin_*", access = ["tenants"] }
  "ops"              = { principal_arn = "arn:aws:iam::210987654321:role/vault-ops", access = ["operator"] }
  "brad"             = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = ["tenants", "operator"] }
}
```

To rotate a principal, change `principal_arn` and apply. To revoke it, remove the key and apply. Changes reach Vault through user-data, so an apply rolls the nodes. Each new node writes the `admin-*` roles at boot and deletes any `admin-*` role not listed. A failed binding is logged (`journalctl -u vault-bootstrap`) and does not stop the node.

- One principal maps to one Vault role across the AWS auth mount. A principal already bound to an app role is refused.
- A trailing `*` is the only wildcard. SSO roles live under `/aws-reserved/sso.amazonaws.com/` (sometimes with a region segment), so include that path. For a wildcard, Vault looks up the caller's full ARN with `iam:GetRole` or `iam:GetUser`, which the node role has.
- Exact ARNs are pinned to the principal's unique ID (`resolve_aws_unique_ids`). A deleted and recreated user or role is locked out until the next node roll, or until `vault-utils admins reconcile` runs on a node.
- Audit: `auth.metadata.client_arn` on each request names the person: `assumed-role/<admin role>/<IAM user>`, or `assumed-role/AWSReservedSSO_…/<SSO user>`. The mount keeps `iam_alias = role_id` (the default), so all members of a role share one Vault entity, and app logins don't create an entity per session.
- The NLB admits the VPC CIDR only. A VPN or subnet router must source-NAT into the VPC.
- Apply rights on this workspace, and IAM rights over the admin groups, are effectively Vault admin rights.

## Security

- Host ports bind to `127.0.0.1` only
- AWS NLB terminates TLS only when a subdomain is connected, with a certificate for that name. Clients on `vault.internal` verify it with `VAULT_TLS_SERVER_NAME` (output `tls_server_name`). Without a subdomain the NLB forwards plain TCP. Vault nodes listen HTTP on 8200
- No Vault `-dev` mode
- Root token revoked after bootstrap
- Unseal keys, tokens, and `.env` are gitignored (mode 600). Never printed to logs
- AWS platform secrets (`init`, `provisioning`, `operator`) have `prevent_destroy` (var `protect_platform_secrets`, default true) and a 30-day recovery window. Set the var to false before destroying the workspace.
- Audit values are HMAC'd. Raw secrets must not appear in the audit log
- Provisioning cannot read tenant KV
- Operator cannot read tenant KV
- Admin access: humans log in with AWS IAM to `admin-*` roles (1h tokens, 8h max) and never handle platform tokens. Apps cannot write `admin-*` roles. Apply rights on the cluster workspace, or IAM rights on the admin groups, are Vault admin rights
- `health serve` renews the periodic provisioning, operator, and aws-auth tokens on every node
- Local Compose sets `disable_mlock = true` because Docker and GitHub Actions cannot mlock. Production hosts should use `IPC_LOCK`
- Dynamic credentials (Go library and tests only): bounded TTL, revoke drops the Postgres role, residue scan expects zero leftover `v-*` roles
