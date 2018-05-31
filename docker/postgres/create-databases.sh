#!/bin/sh

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

create_database() {
  local db=$1
  echo "Creating database: $db"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
CREATE DATABASE $db;
EOSQL
}

create_databases() {
  local IFS=','
  dbs=$DATABASES
  for db in $dbs; do
    create_database "$db"
  done
}

create_databases