# Changelog

All notable changes to the Secrets Vault Cluster.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `internal/vaultcluster` Go library, `cmd/vault-utils` CLI, and root `Dockerfile`
  for a `vault-utils` image (`bootstrap local`, `tenants create`,
  `tenants destroy`, `snapshot`, `health`).
- `internal/vaultcluster/keystore.go`: pluggable `KeyStore` for init key
  material and platform tokens; `FileKeyStore` backs the local platform.
- `internal/vaultcluster/snapshot.go`: Raft snapshot take, list, verify, and
  restore in Go (replaces `local/snapshot.sh`). Restore requires an explicit
  `VAULT_TOKEN` (break-glass root); the operator policy denies
  `snapshot-force` by design.
- `go test -short ./...` for tenant-id and policy lint; `TestIsolationMatrix`
  and `TestCredentialsMatrix` run throwaway Vault CE (and Postgres) containers.
- `./setup.sh` one-command local start: first run initializes a persistent
  Shamir Vault; later starts unseal via the one-shot bootstrap. Local Shamir
  automation, not production KMS auto-unseal.
- ADR-0009: repository layout uses `local/`, `aws/`, `gcp/`, and `azure/` as
  deployment targets. Phase 1 implements only `local/`; cloud directories are
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
- ADR-0008: capability layering: isolation standalone, dynamic credentials
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

### Changed

- `vault-utils` CLI is command/subcommand: `bootstrap local|aws|azure|gcp`
  (cloud platforms not implemented yet), `tenants create`, `tenants destroy`,
  `snapshot take|list|verify|restore`, `health`.
- Bootstrap no longer creates tenants; it only initializes the cluster
  (init, unseal, configure, issue tokens, revoke root). Tenants are created
  with `vault-utils tenants create <id>`.
- `BOOTSTRAP_DIR` defaults to `.bootstrap` so `vault-utils bootstrap local`
  works on a host without Compose env vars.
- Policy templates use Go `text/template` (`{{.KVMount}}`) instead of
  `@@KEY@@` string replacement.
- `local/docker-compose.yml` renamed to `local/compose.yml`; images are
  digest-pinned inline instead of `VAULT_IMAGE` / `POSTGRES_IMAGE` env vars.
- `bootstrap` is a default one-shot Compose service (`restart: "no"`).
  `docker compose up -d` starts Vault and runs bootstrap once. A Vault process
  restart reseals until `./setup.sh` or `docker compose run --rm bootstrap`.
- `.github/workflows/validate.yml`: unit checks are `go test -short ./...`.
  Compose job requires `bootstrap` as a default service and forbids `unseal`.
- `.github/workflows/test-local.yml`: isolation and credentials are `go test`;
  Compose one-shot is a separate job.
- Local operators use `vault-utils` for tenant create and offboard.
- `local/runtime-test.sh`: asserts bootstrap is not long-running; after
  restart, re-runs the one-shot to unseal and checks Raft persistence. The
  persistence probe creates its tenant explicitly.
- `local/setup.sh`: start PostgreSQL when `ENABLE_DYNAMIC_CREDENTIALS` is
  already true in `.env`, not only when `--with-credentials` is passed;
  `--with-credentials` persists the flag in `.env`. Rebuilds the `vault-utils`
  image before bootstrap so the one-shot matches the current source.
- Policy templates moved to `internal/vaultcluster/policies`. Lint fixtures
  moved to `internal/vaultcluster/testdata/lint`.
- Lint and conformance are Go tests in `internal/vaultcluster`
  (`TestIsolationMatrix`, `TestCredentialsMatrix`).
- Compose helpers are `local/lib.sh`, `local/setup.sh`, `local/reset.sh`.
  Vault bootstrap is only `vault-utils bootstrap local`.
- `local/postgres/init.sh` creates `vault_admin` and privilege roles only.
- Single root `README.md` is the operator and architecture guide. Removed
  `vault-cluster-readme.md` and `vault-cluster-technical-doc.md`.
- `.github/workflows` run CI on `master` and `develop` as well as `main`.
- Credential suites revoke the leases they issue as the tenant (provisioning
  cannot `revoke-prefix`). Residue scan no longer treats leftover test roles
  as orphans.
- Publish only root `README.md` and `CHANGELOG.md`. ADRs, design docs,
  runbooks, examples, and nested READMEs stay local via `.gitignore`.
- Trim essay comments and INV labels from policies, scripts, tests, and CI.
  Keep image pins, tenant IDs, and HTTP 403 checks.

### Removed

- `local/bootstrap/compose-unseal.sh` (long-running Shamir sidecar).
- `local/bootstrap/bootstrap.sh` (bash local bootstrap).
- `config/`, `scripts/`, and `tests/` (contents relocated).
- `local/bootstrap/` (Compose helpers moved to `local/`).
- `local/scripts/` and bash lint/conformance under `local/tests/`.
- Demo `app.customers` / `app.orders` schema in `local/postgres/init.sh`.
- `local/snapshot.sh` (now `vault-utils snapshot`) and `local/stop.sh`
  (documented `docker compose down`).

### Fixed

- `./setup.sh --with-credentials` after an isolation bootstrap mounts the
  database engine with the operator token instead of skipping Configure and
  404ing on `database/roles`.
- `internal/vaultcluster/policies/templates/operator.hcl.tpl`: operator can
  create the database mount and connection so credentials can be enabled
  after root is revoked.
- `internal/vaultcluster/isolation_test.go`: parse the container ID from
  `docker run` stdout only. Combined stdout/stderr included image-pull noise
  on CI, so `docker port` failed.
- Bootstrap material written as root in a container is chowned to the
  bind-mount owner so the host user can read it on Linux CI.
- `.github/workflows/validate.yml`: enable the credentials profile with
  `COMPOSE_PROFILES` and `--env-file` so postgres is visible in
  `compose config`; CE image grep no longer matches the workflow file itself.
- Runtime test: Vault `-dev` check no longer matches `local-dev-only`
  password strings.
- `local/vault/config.hcl`: `disable_mlock = true` so Vault starts in Compose
  and GitHub Actions (`Failed to lock memory: cannot allocate memory`).

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
