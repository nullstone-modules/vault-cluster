# Vault cluster — operator guide

Local Docker Compose target. AWS, GCP, and Azure are not implemented yet.

You do **not** init or unseal by hand. `./setup.sh` is first boot and every later start. A Compose sidecar initializes Shamir 5/3 once and unseals whenever Vault reseals. That is laptop DX, not production KMS auto-unseal.

## Prerequisites

Docker Desktop (Compose v2), `curl`, and `jq`. Vault CLI is optional.

## Start

```bash
./setup.sh
```

Optional dynamic database credentials (starts PostgreSQL):

```bash
./setup.sh --with-credentials
```

Then:

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=$(cat local/.bootstrap/provisioning.token)
```

Back up `local/.bootstrap/vault-init.json` immediately. It is gitignored (mode 600). Without it this volume cannot be unsealed.

Isolation path: `kv/data/customers/{tenant}/*`. This is not Vault Enterprise namespaces.

## Commands

| Command | Destructive | What it does |
|---|---|---|
| `./setup.sh` | no | Start, init (first time), unseal, configure, synthetic tenants |
| `./setup.sh --with-credentials` | no | Same, plus PostgreSQL and the database engine |
| `./local/stop.sh` | no | Stop containers. **Keeps all data.** |
| `./local/bootstrap/health.sh` | no | Health with a reason for each failure |
| `./local/reset.sh --yes` | **yes** | Destroys volumes **and** unseal keys |
| `./scripts/tenants/create-tenant.sh <id>` | no | Onboard a tenant |
| `./scripts/tenants/offboard-tenant.sh <id> --yes` | yes (access) | Revoke access; secrets kept |
| `./tests/run-conformance.sh --layer isolation` | no | Isolation tests |
| `./local/tests/runtime-test.sh` | no | Persistence and unseal sidecar tests |

Do not commit `.bootstrap/` or `.env`. Do not run `vault operator unseal`.
