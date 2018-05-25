#!/bin/sh

# Script used when compiling binary during build phase

set -o errexit
set -o nounset
if set -o | grep -q "pipefail"; then
  set -o pipefail
fi

# export build flags
export CGO_ENABLED="${CGO_ENABLED:-0}"
export GOARCH="${ARCH}"

# generate bindata assets
go generate -x "${PKG}/pkg/migrations/"

# compile our binary using install, the mounted volume ensures we can see it
# outside the build container
go install \
    -v \
    -installsuffix "static" \
    -ldflags "-extldflags -static -X ${PKG}/pkg/version.Version=${VERSION} -X \"${PKG}/pkg/version.BuildDate=${BUILD_DATE}\" -X ${PKG}/pkg/version.BinaryName=${BINARY_NAME}" \
    ./...
