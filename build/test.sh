#!/bin/sh

# Script used when running tests inside the containerized environment.

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

source ./build/shared.sh

export CGO_ENABLED=${CGO_ENABLED:-0}

TARGETS=$(for d in "$@"; do echo ./$d/...; done)

go test -i -installsuffix "static" ${TARGETS}

if [ ! -z "$RUN" ]; then
    go test -v -timeout 30s ${TARGETS} -run "$RUN"
else
    go test -v -p 1 -installsuffix "static" -coverprofile=.coverage/coverage.out -timeout 30s ${TARGETS}
    go tool cover -html=.coverage/coverage.out -o .coverage/coverage.html
    go tool cover -func=.coverage/coverage.out
    echo

    echo -n "Checking gofmt: "
    ERRS=$(find "$@" -type f ! -name assets.go -name \*.go | xargs gofmt -l 2>&1 || true)
    if [ -n "${ERRS}" ]; then
        echo "FAIL - the following files need to be gofmt'ed:"
        for e in ${ERRS}; do
            echo "    $e"
        done
        echo
        exit 1
    fi
    echo "PASS"

    echo -n "Checking go vet: "
    ERRS=$(go vet ${TARGETS} 2>&1 || true)
    if [ -n "${ERRS}" ]; then
        echo "FAIL"
        echo "${ERRS}"
        echo
        exit 1
    fi
    echo "PASS"
fi
