#!/bin/sh
set -eu

: "${POSTGRES_USER:?POSTGRES_USER must be set}"
: "${POSTGRES_DB:?POSTGRES_DB must be set}"
: "${VAULT_DB_ADMIN_PASSWORD:?VAULT_DB_ADMIN_PASSWORD must be set}"

psql -v ON_ERROR_STOP=1 \
     -v vault_admin_password="${VAULT_DB_ADMIN_PASSWORD}" \
     --username "${POSTGRES_USER}" \
     --dbname "${POSTGRES_DB}" <<'EOSQL'
DO $$ BEGIN
    CREATE ROLE app_readonly NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    CREATE ROLE app_readwrite NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    CREATE ROLE vault_admin WITH LOGIN CREATEROLE;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
ALTER ROLE vault_admin WITH PASSWORD :'vault_admin_password';
GRANT app_readonly  TO vault_admin WITH ADMIN OPTION;
GRANT app_readwrite TO vault_admin WITH ADMIN OPTION;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
EOSQL
