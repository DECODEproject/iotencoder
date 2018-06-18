#!/bin/bash

# Script used when booting dev/shell environment. Just ensures dev database
# exists before we continue.

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

source ./build/shared.sh

/bin/bash
