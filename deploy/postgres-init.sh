#!/usr/bin/env bash
# Creates the sms Postgres role and databases (sms, sms_test).
# Idempotent: safe to re-run. Requires sudo access to the postgres system user.
#
# Required env:
#   SMS_DB_PASSWORD  Password for the sms role (hex/url-safe recommended; avoid single quotes).
# Optional env:
#   SMS_DB_USER       Role name     (default: sms)
#   SMS_DB_NAME       Main database (default: sms)
#   SMS_DB_NAME_TEST  Test database (default: sms_test)

set -euo pipefail

: "${SMS_DB_PASSWORD:?SMS_DB_PASSWORD must be set}"
SMS_DB_USER="${SMS_DB_USER:-sms}"
SMS_DB_NAME="${SMS_DB_NAME:-sms}"
SMS_DB_NAME_TEST="${SMS_DB_NAME_TEST:-sms_test}"

case "$SMS_DB_PASSWORD" in
    *\'*) echo "SMS_DB_PASSWORD must not contain single quotes" >&2; exit 1 ;;
esac

psql_admin() { sudo -u postgres psql -v ON_ERROR_STOP=1 -d postgres "$@"; }

SQL_ROLE=$(cat <<EOF
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${SMS_DB_USER}') THEN
        CREATE ROLE ${SMS_DB_USER} LOGIN PASSWORD '${SMS_DB_PASSWORD}';
        RAISE NOTICE 'Created role ${SMS_DB_USER}';
    ELSE
        ALTER ROLE ${SMS_DB_USER} WITH LOGIN PASSWORD '${SMS_DB_PASSWORD}';
        RAISE NOTICE 'Updated password for role ${SMS_DB_USER}';
    END IF;
END
\$\$;
EOF
)
echo "$SQL_ROLE" | psql_admin

for db in "$SMS_DB_NAME" "$SMS_DB_NAME_TEST"; do
    exists=$(psql_admin -tAc "SELECT 1 FROM pg_database WHERE datname='${db}'")
    if [ "$exists" != "1" ]; then
        psql_admin -c "CREATE DATABASE ${db} OWNER ${SMS_DB_USER}"
        echo ">> Created database ${db}"
    else
        echo ">> Database ${db} already exists (skipped)"
    fi
done

echo "Done. DSN: postgres://${SMS_DB_USER}:***@localhost:5432/${SMS_DB_NAME}?sslmode=disable"
