#!/bin/sh
# Init schema and vault_admin (CREATEROLE). Password from the environment.
set -eu

: "${POSTGRES_USER:?POSTGRES_USER must be set}"
: "${POSTGRES_DB:?POSTGRES_DB must be set}"
: "${VAULT_DB_ADMIN_PASSWORD:?VAULT_DB_ADMIN_PASSWORD must be set}"

psql -v ON_ERROR_STOP=1 \
     -v vault_admin_password="${VAULT_DB_ADMIN_PASSWORD}" \
     -v dbname="${POSTGRES_DB}" \
     --username "${POSTGRES_USER}" \
     --dbname "${POSTGRES_DB}" <<'EOSQL'

-- Demo application schema. Fake data only. This exists so dynamic credential
-- least-privilege can be verified against real objects: a readonly credential
-- must be able to SELECT and must fail to INSERT. Asserting privileges against
-- an empty schema proves nothing.
CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE IF NOT EXISTS app.customers (
    id         serial PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app.orders (
    id           serial PRIMARY KEY,
    customer_id  integer NOT NULL REFERENCES app.customers(id),
    amount_cents integer NOT NULL CHECK (amount_cents >= 0)
);

INSERT INTO app.customers (name)
SELECT v FROM (VALUES ('Fake Customer One'), ('Fake Customer Two')) AS t(v)
WHERE NOT EXISTS (SELECT 1 FROM app.customers);

-- Privilege roles. NOLOGIN group roles. Vault's dynamic users are granted one
-- of these, so the privilege definition lives in PostgreSQL and stays
-- authoritative: Vault decides who gets access and for how long, PostgreSQL
-- decides what that access is.
DO $$ BEGIN
    CREATE ROLE app_readonly NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE ROLE app_readwrite NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

GRANT USAGE ON SCHEMA app TO app_readonly, app_readwrite;

GRANT SELECT ON ALL TABLES IN SCHEMA app TO app_readonly;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO app_readwrite;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA app TO app_readwrite;

-- Tables created later inherit the same grants, so a new table cannot silently
-- become invisible to readers or unwritable by writers.
ALTER DEFAULT PRIVILEGES IN SCHEMA app
    GRANT SELECT ON TABLES TO app_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA app
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_readwrite;

-- Keep the public schema closed. On PostgreSQL 15+ this is already the
-- default; stating it means a restored older dump cannot quietly reopen it.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Vault's management role: CREATEROLE, not SUPERUSER. Vault needs to create,
-- grant, and drop the temporary users it issues - nothing more. A superuser
-- here would mean a compromised Vault database config is a compromised
-- database, and would let dynamic credentials escalate past the privilege
-- roles above.
DO $$ BEGIN
    CREATE ROLE vault_admin WITH LOGIN CREATEROLE;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER ROLE vault_admin WITH PASSWORD :'vault_admin_password';

-- vault_admin must hold these roles WITH ADMIN OPTION to grant them onward to
-- the users it creates. Without ADMIN OPTION, issuance fails at the GRANT step
-- with a permission error that looks like a Vault problem but is not one.
GRANT app_readonly  TO vault_admin WITH ADMIN OPTION;
GRANT app_readwrite TO vault_admin WITH ADMIN OPTION;

GRANT CONNECT ON DATABASE :"dbname" TO app_readonly, app_readwrite;

EOSQL

echo "[init] PostgreSQL initialized: app schema, privilege roles, vault_admin"
