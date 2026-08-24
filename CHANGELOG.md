# Changelog

All notable changes to the Secrets Vault Cluster.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `internal/vaultcluster` Go library, `cmd/vault-utils` CLI, and root `Dockerfile`
  for a `vault-utils` image (local-bootstrap, configure, tenant-create, health).
- `go test -short ./...` for tenant-id and policy lint; `TestIsolationMatrix`
  runs a throwaway Vault CE container and asserts isolation denials as HTTP 403.

### Changed

- `local/docker-compose.yml`, `local/setup.sh`, `local/bootstrap/up.sh` — Compose
  bootstrap is a one-shot `bootstrap` profile (`restart: "no"`), not a
  long-running Shamir unseal sidecar. A Vault process restart reseals until
  `./setup.sh` or `docker compose --profile bootstrap run --rm bootstrap`.
- `.github/workflows/validate.yml` — `go test -short ./...`; compose job expects
  the bootstrap profile, not an `unseal` service.
- `.github/workflows/test-local.yml` — isolation job is `go test`; Compose
  one-shot and credentials remain separate jobs.
- `local/tests/runtime-test.sh` — asserts bootstrap is not long-running; after
  restart, re-runs the one-shot to unseal and checks Raft persistence.
- `local/setup.sh`, `local/bootstrap/up.sh` — start PostgreSQL when
  `ENABLE_DYNAMIC_CREDENTIALS` is already true in `.env`, not only when
  `--with-credentials` is passed.

### Fixed

- `internal/vaultcluster/isolation_test.go` - parse the container ID from
  `docker run` stdout only. Combined stdout/stderr included image-pull noise
  on CI, so `docker port` failed.
- `local/bootstrap/compose-unseal.sh` - chown `vault-init.json` to the bind-mount
  owner so host `jq` can read it on Linux CI (root-owned mode 600).
- `.github/workflows/validate.yml` - enable the credentials profile with
  `COMPOSE_PROFILES` and `--env-file` so postgres is visible in `compose config`.
- `local/vault/config.hcl` — `disable_mlock = true` so Vault starts in Compose
  and GitHub Actions (`Failed to lock memory: cannot allocate memory`).
- `.github/workflows/validate.yml` — CE image grep no longer matches the
  workflow file itself.

### Changed

- Single root `README.md` is the operator and architecture guide. Removed
  `vault-cluster-readme.md` and `vault-cluster-technical-doc.md`.
- `.github/workflows` run CI on `master` and `develop` as well as `main`.
- `tests/conformance/credentials/*` — credential suites revoke the leases they
  issue as the tenant (provisioning cannot `revoke-prefix`). Residue scan no
  longer treats leftover test roles as orphans.
- `local/bootstrap/health.sh` — reports the database engine from the mount,
  not only from `.env`.
- `local/setup.sh` — `--with-credentials` persists `ENABLE_DYNAMIC_CREDENTIALS=true`
  in `.env` so later health/compose runs stay consistent.
- `local/bootstrap/bootstrap.sh`, `local/setup.sh` — `./setup.sh --with-credentials`
  on an isolation Vault now mounts the database engine and refreshes tenant
  roles instead of skipping as "already configured".
- Publish only root `README.md` and `CHANGELOG.md`.
  ADRs, design docs, runbooks, examples, and nested READMEs stay local via
  `.gitignore`.
- Trim essay comments and INV labels from policies, scripts, tests, and CI.
  Keep image pins, tenant IDs, and HTTP 403 checks.

### Added

- `./setup.sh` one-command local start, plus a Compose `unseal` sidecar:
  first run initializes a persistent Shamir Vault; every later start and every
  Vault container restart unseals automatically. Local Shamir automation, not
  production KMS auto-unseal. `./setup.sh` still configures the platform and
  synthetic tenants on a new machine.
- ADR-0009: repository layout uses `local/`, `aws/`, `gcp/`, and `azure/` as
  deployment targets. Shared Vault behaviour lives in `config/`, `scripts/`,
  and `tests/`. Phase 1 implements only `local/`; cloud directories are
  documentation stubs. There is no Terraform `source = "./${var.deployment_target}"`
  switch.

**Platform contract**

- `docs/PLATFORM_CONTRACT.md`: provider-independent inputs, safe outputs, the
  four-identity model, and twelve numbered security invariants, each citing the
  check that enforces it.
- ADR-0006: platform contract with runtime adapters, superseding the
  cloud-agnostic Terraform contract module in ADR-0005.
- ADR-0007: the provisioning identity has no access to tenant secret data,
  narrowing the grant drafted in ADR-0002.
- ADR-0008: capability layering — isolation standalone, dynamic credentials
  optional and off by default.

**Vault configuration**

- Policy templates for tenant reader, tenant writer, tenant database,
  provisioning, and operator. Every tenant template covers both KV v2 path
  families and carries explicit denies.
- KV v2, AppRole, file audit device, and PostgreSQL database engine
  configuration, all idempotent and driven purely by `VAULT_ADDR`.
- Tenant automation: validation, render, lint, apply, role binding, and
  offboarding with lease revocation.
- Policy linter enforcing INV-7, including detection of the KV v2 path-split
  mistake, with fixtures it must reject.

**Conformance suite**

- Isolation: KV lifecycle, cross-tenant matrix, path traversal and prefix
  anchoring, administrative surfaces, identity separation, and audit content.
- Credentials: issuance and connection, least privilege, cross-tenant denial,
  revocation and TTL expiry, and residue with a planted-orphan self-test.
- Denials are asserted as HTTP 403 specifically; a 404 is a distinct failure,
  because a test that accepts any failure as a denial passes against an empty
  Vault.

**Local Docker target**

- Compose environment with Vault CE 1.21.4 and PostgreSQL 16.15, pinned by tag
  and digest. No dev mode, no `latest`.
- Single-node Raft on a named volume, with audit logs on a **separate** volume,
  and the listener published to loopback only.
- Scripts for up, down, bootstrap, health, reset, and Raft snapshot with
  checksum. Destructive operations require an explicit flag.
- Runtime tests for persistence across restart, volume separation, port
  binding, and digest pinning.

**Documentation**

- Implementation plan with acceptance criteria for runtime, isolation, dynamic
  credentials, and revocation.
- Runbooks: bootstrap, health check and triage, tenant onboarding, backup and
  restore including a restore drill, and break-glass.
- Worked examples for both capability configurations.

**CI**

- `validate.yml`: shellcheck, linter self-test, secret scan, Community Edition
  guard, and enforcement of the one-way dependency rule.
- `test-local.yml`: isolation with no database present, then credentials as a
  separate job.

### Security

- Initial root token is revoked at the end of bootstrap.
- Bootstrap material is written under `umask 077` rather than chmod-ed
  afterwards, so it never exists world-readable even briefly.
- Audit is enabled first during bootstrap, so platform configuration is itself
  recorded.
- `log_raw` stays false; the audit suite asserts both that no fixture value
  appears in plaintext and that HMAC markers are present, so "no plaintext
  found" cannot be satisfied by an empty log.

### Known limitations

- TLS is not enabled; loopback-only binding is the compensating control
  (ADR-0004).
- Single-node Raft proves persistence, not high availability.
- The provisioning identity retains an indirect path to tenant data by
  authoring a permissive `tenant-*` policy and binding a role to it. The direct
  path is closed and the indirect one is audit-visible; separating policy
  authorship from role binding is recorded as follow-up work.
