# Secrets Vault Cluster

Multi-platform modules to configure a self-hosted Vault cluster on local, AWS, GCP, and Azure

Applications share one Vault. They do not see each other's secrets. Isolation is a path prefix plus ACL policy plus AppRole. Vault CE has no namespaces. This is not Enterprise.

**Dev machine:** `./setup.sh`. You do not init or unseal by hand.

| Target | Role | Status |
|---|---|---|
| `local/` | Docker Compose | Implemented |
| `aws/` | AWS | Not implemented |
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

- Vault CE 1.21.4 (never `-dev`, never Enterprise)
- Docker Compose, Raft on a named volume
- KV v2 at `kv/customers/{tenant}/*`
- Optional PostgreSQL dynamic credentials
- File audit on a volume separate from Raft
- Auto-init (first start) and one-shot Shamir unseal via `vault-utils`
- Isolation tests in Go (`go test`); credentials tests in Go (`TestCredentialsMatrix`)

Not implemented:

- AWS, GCP, Azure, Kubernetes
- Production KMS auto-unseal
- TLS, multi-node Raft, DR replication
- OpenTofu / Terraform

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
  |     Database engine (optional)
  |     Audit
  |
  +-- vault-utils bootstrap   (one-shot: init, unseal, configure)
  |
  +-- PostgreSQL    (Compose profile `credentials` only)
```

Isolation and credentials tests are Go (`go test ./internal/vaultcluster`). Isolation runs Vault alone. PostgreSQL starts only with `--with-credentials`.

## Repository layout

```
vault-cluster/
├── README.md
├── CHANGELOG.md
├── setup.sh
├── Dockerfile            vault-utils image
├── cmd/                  Go app entrypoints (vault-utils CLI)
├── internal/             Go libraries, policy templates, lint fixtures
├── local/                Compose target, setup, snapshots
├── aws/                  Nullstone Terraform module (not yet implemented)
├── gcp/                  Nullstone Terraform module (not yet implemented)
└── azure/                Nullstone Terraform module (not yet implemented)
```

## Prerequisites

Docker Desktop (Compose v2), `curl`, and `jq`. Go 1.23 for `go test`. Vault CLI is optional except break-glass decode.

Images are pinned by tag and digest in `local/.env.example` (Vault 1.21.4, PostgreSQL 16.15-alpine). Never `latest`.

```bash
docker --version
docker compose version
jq --version
curl --version
```

## Quick start

```bash
./setup.sh
```

First run:

1. Starts persistent Vault CE
2. Initializes Shamir 5/3
3. Writes keys to `local/.bootstrap/` (gitignored, mode 600)
4. Unseals
5. Enables audit, KV v2, AppRole, policies
6. Revokes the root token
7. Onboards synthetic `tenant-a` and `tenant-b`

Later runs skip init if the provisioning token still works. A Vault process restart reseals. Unseal is a one-shot (`./setup.sh` or `docker compose run --rm bootstrap`), not a long-running sidecar. `docker compose up -d` starts Vault and runs bootstrap once, then bootstrap exits.

Dynamic credentials (starts PostgreSQL):

```bash
./setup.sh --with-credentials
```

Then:

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat local/.bootstrap/provisioning.token)
```

Back up `local/.bootstrap/vault-init.json` immediately. Without it this volume cannot be unsealed.

Do not commit `.bootstrap/` or `.env`. Do not run `vault operator unseal`.

## Commands

Run from the repository root unless noted. Destructive commands require `--yes`.

| Command | Destructive | Purpose |
|---|---|---|
| `./setup.sh` | no | Start, init (first time), unseal, configure, synthetic tenants |
| `./setup.sh --with-credentials` | no | Same, plus PostgreSQL and the database engine |
| `./local/stop.sh` | no | Stop containers. Keeps all data. |
| `./local/snapshot.sh take` | no | Raft snapshot plus SHA-256 |
| `./local/snapshot.sh restore <file> --yes` | yes | Replaces all Vault state |
| `./local/reset.sh --yes` | yes | Destroys volumes and unseal keys |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenant-create <id>` | no | Onboard a tenant |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenant-offboard <id> --yes` | yes (access) | Revoke access; secrets kept |
| `docker compose run --rm -e VAULT_TOKEN=... bootstrap tenant-offboard <id> --yes --purge-secrets` | yes | Also destroy secret versions |
| `go test -short ./...` | no | Unit tests (tenant ID, policy lint, render) |
| `go test ./internal/vaultcluster` | no | Isolation and credentials (needs Docker) |
| `./local/runtime-test.sh` | no | Persistence and one-shot unseal after restart |

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
docker compose --env-file .env run --rm -e VAULT_TOKEN bootstrap tenant-create acme-corp
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
docker compose --env-file .env run --rm -e VAULT_TOKEN bootstrap tenant-offboard acme-corp --yes
```

Offboard and destroy data (needs break-glass; provisioning cannot read or purge KV):

```bash
cd local
docker compose --env-file .env run --rm -e VAULT_TOKEN bootstrap tenant-offboard acme-corp --yes --purge-secrets
```

Cross-tenant, wildcard, and traversal reads return HTTP 403. That is the isolation contract. A 200 on those paths is a breach.

## Identities

| Identity | Role |
|---|---|
| Root | Bootstrap only. Revoked when setup finishes. |
| Provisioning | Create and offboard tenants. Cannot read tenant secrets. |
| Tenant AppRole | One reader and one writer per tenant. |
| Operator | Health, mounts, snapshots. Not a tenant secret reader. |

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
./setup.sh
# or: cd local && docker compose run --rm bootstrap
```

Vault fails closed when audit cannot write. If every request is denied:

```bash
docker exec vault-cluster-vault sh -c 'ls -la /vault/logs && df -h /vault/logs'
```

## Backup, restore, and disaster recovery

A snapshot is the whole cluster (secrets, policies, tokens). Treat it like Vault itself. A backup that has never been restored is not a backup.

Keep the matching `vault-init.json` with each snapshot. After restore, Vault unseals only with the shares that were current when the snapshot was taken.

### Backup

```bash
cd local
./snapshot.sh take
./snapshot.sh list
./snapshot.sh verify .bootstrap/backups/vault-<stamp>.snap
```

### Restore (destructive)

```bash
cd local
./snapshot.sh restore .bootstrap/backups/vault-<stamp>.snap --yes
./setup.sh
```

### Restore drill

```bash
cd local
./setup.sh
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat .bootstrap/provisioning.token)
docker compose --env-file .env run --rm -e VAULT_TOKEN bootstrap tenant-create tenant-a

./snapshot.sh take
cp .bootstrap/vault-init.json /tmp/keys-at-snapshot.json

docker compose --env-file .env run --rm -e VAULT_TOKEN bootstrap tenant-create tenant-drill
./reset.sh --yes

./setup.sh
cp /tmp/keys-at-snapshot.json .bootstrap/vault-init.json
./snapshot.sh restore .bootstrap/backups/vault-<stamp>.snap --yes
./setup.sh
go test ./internal/vaultcluster -run TestIsolationMatrix
```

Expected: `tenant-a` exists, `tenant-drill` does not, isolation suite passes.

### If unseal keys are lost

The Raft volume cannot be unsealed. Data is gone. That is Shamir working. Restore from a snapshot that still has its matching keys, or run `./local/reset.sh --yes` and start empty.

PostgreSQL is not in the Vault snapshot. Dynamic credentials are re-issued.

## Break-glass

Use only when no routine identity can do the job (read tenant data in an incident, purge secrets, repair audit). Two people. Record why first.

```bash
export VAULT_ADDR=http://127.0.0.1:8200
curl -s -X PUT "${VAULT_ADDR}/v1/sys/generate-root/attempt" | jq
# each of 3 share holders:
curl -s -X PUT --data '{"key":"<share>","nonce":"<nonce>"}' \
  "${VAULT_ADDR}/v1/sys/generate-root/update" | jq
vault operator generate-root -decode=<encoded_token> -otp=<otp>
# one recorded action, then:
curl -s -X POST -H "X-Vault-Token: ${ROOT_TOKEN}" \
  "${VAULT_ADDR}/v1/auth/token/revoke-self"
```

Cancel an in-flight attempt:

```bash
curl -s -X DELETE "${VAULT_ADDR}/v1/sys/generate-root/attempt"
```

## Testing

```bash
go test -short ./...

go test ./internal/vaultcluster

cd local && ./runtime-test.sh
```

Denials must be HTTP 403. A 404 is a different failure.

## Troubleshooting

| Symptom | Action |
|---|---|
| Docker daemon is not running | Start Docker Desktop |
| Port already allocated | Change `VAULT_HOST_PORT` in `local/.env` |
| Initialized but `.bootstrap` missing | Restore `vault-init.json`, or `./local/reset.sh --yes` |
| Health 503 | Vault is sealed. Run `./setup.sh` (one-shot bootstrap) |
| Permission denied on tenant create | Use the provisioning token, not operator |
| Permission denied on tenant secrets | Expected for provisioning |
| `failed to verify connection` | Start with `--with-credentials` |
| Everything denied | Audit volume full or unwritable |

## Security

- Host ports bind to `127.0.0.1` only
- No Vault `-dev` mode
- Root token revoked after bootstrap
- Unseal keys, tokens, and `.env` are gitignored (mode 600). Never printed to logs
- Audit values are HMAC'd. Raw secrets must not appear in the audit log
- Provisioning cannot read tenant KV
- Operator cannot read tenant KV
- Local Compose sets `disable_mlock = true` because Docker and GitHub Actions cannot mlock. Production hosts should use `IPC_LOCK`
- Dynamic credentials: bounded TTL, revoke drops the Postgres role, residue scan expects zero leftover `v-*` roles
