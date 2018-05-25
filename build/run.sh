#!/bin/sh

# Script used when starting local compose services. Ensures dev database
# exists before we continue.

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

source ./build/shared.sh

"$@"
