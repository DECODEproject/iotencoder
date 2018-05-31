#!/bin/sh

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

RETRIES=5

until nc -z postgres:5432 || [ "$RETRIES" -eq 0 ]; do
  echo "Waiting for postgres server, $((RETRIES--)) remaining attempts"
  sleep 1
done