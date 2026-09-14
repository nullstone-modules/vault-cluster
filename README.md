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
6. [Commands](#commands)
7. [Tenant isolation](#tenant-isolation)
8. [Identities](#identities)
9. [Health](#health)
10. [Backup, restore, and disaster recovery](#backup-restore-and-disaster-recovery)
11. [Break-glass](#break-glass)
12. [Testing](#testing)
13. [Troubleshooting](#troubleshooting)
14. [Security](#security)

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
- AWS AMI, user-data, internal NLB (TLS 8200, health 8210), `vault.internal`, ASG (`cluster_size`), rolling instance refresh on launch-template change

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
| Provisioning | Create and offboard tenants. Cannot read tenant secrets. |
| Tenant AppRole | One reader and one writer per tenant. |
| Operator | Health, mounts, snapshots. Can start generate-root (recovery keys still required). Not a tenant secret reader. Cannot restore. |

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
3. Run workspace preview/plan in Nullstone so `ns_connection` outputs resolve.
4. In the plan, expect IAM, three Secrets Manager secrets (`protect_platform_secrets` default true, 30-day recovery), node and NLB security groups, a launch template, an ACM cert for `vault.internal`, an alias on the network internal zone, an internal NLB with TLS on 8200 (health 8210), and an ASG of `cluster_size` (`max_size` is `cluster_size + 1` for surge). A launch-template change starts a rolling instance refresh: one extra node joins, then one old node leaves. Clients use `https://vault.internal:8200`. The NLB terminates TLS; Vault nodes still listen HTTP.

Bake the node AMI (x86_64, matches default `t3.micro`) from `vault-node/`:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o vault-node/vault-utils ./cmd/vault-utils
cd vault-node
packer init .
packer build -var region="$AWS_REGION" vault.pkr.hcl
```

`.github/workflows/build-ami.yml` runs the same bake on demand or on a push to `main` that touches the image inputs, and copies the result to every region in the `AMI_REGIONS` repository variable. It assumes the `AWS_ROLE_ARN` secret through GitHub OIDC and bakes in `AWS_REGION` (default `us-east-1`).

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

## Security

- Host ports bind to `127.0.0.1` only
- AWS NLB terminates TLS for `vault.internal`; Vault nodes listen HTTP on 8200
- No Vault `-dev` mode
- Root token revoked after bootstrap
- Unseal keys, tokens, and `.env` are gitignored (mode 600). Never printed to logs
- AWS platform secrets (`init`, `provisioning`, `operator`) have `prevent_destroy` (var `protect_platform_secrets`, default true) and a 30-day recovery window. Set the var to false before destroying the workspace.
- Audit values are HMAC'd. Raw secrets must not appear in the audit log
- Provisioning cannot read tenant KV
- Operator cannot read tenant KV
- Local Compose sets `disable_mlock = true` because Docker and GitHub Actions cannot mlock. Production hosts should use `IPC_LOCK`
- Dynamic credentials (Go library and tests only): bounded TTL, revoke drops the Postgres role, residue scan expects zero leftover `v-*` roles
