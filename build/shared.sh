#!/bin/sh

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

DBNAME=$(basename "$IOTENCODER_DATABASE_URL" | awk -F'[?]' '{print $1}')
if ! psql -tc "SELECT 1" "$DBNAME" >/dev/null 2>&1; then
  echo "Creating database $DBNAME"
  psql -c "CREATE DATABASE $DBNAME" postgres >/dev/null 2>&1;
fi
