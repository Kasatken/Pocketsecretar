#!/usr/bin/env bash
set -euo pipefail

DB_NAME="${DB_NAME:-pocketsecret}"
DB_USER="${DB_USER:-pocketsecret}"
DB_PASSWORD="${DB_PASSWORD:-CHANGE_ME}"

if [[ "${DB_PASSWORD}" == "CHANGE_ME" ]]; then
  echo "Set DB_PASSWORD env before running." >&2
  exit 1
fi

sudo apt-get update
sudo apt-get install -y postgresql postgresql-contrib

sudo -u postgres psql <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_USER}') THEN
    CREATE ROLE ${DB_USER} LOGIN PASSWORD '${DB_PASSWORD}';
  END IF;
END
\$\$;
SQL

DB_EXISTS=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'")
if [[ "${DB_EXISTS}" != "1" ]]; then
  sudo -u postgres psql <<SQL
CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};
SQL
fi

sudo -u postgres psql <<SQL
GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};
SQL

echo "PostgreSQL is ready."
