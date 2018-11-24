[![CircleCI](https://circleci.com/gh/DECODEproject/iotencoder.svg?style=svg)](https://circleci.com/gh/DECODEproject/iotencoder)

# iotencoder

Implementaton of proposed stream encoder interface for the DECODE
IoTPilot/Scale Model.

This component is responsible for subscribing to MQTT topics representing
streams of data from a device, encoding the incoming data and writing it to a
datastore.

Uses an experimental template structure from here:
https://github.com/thingful/go-build-template

## Building

Run `make` or `make build` to build our binary compiled for `linux/amd64`
with the current directory volume mounted into place. This will store
incremental state for the fastest possible build. To build for `arm` or
`arm64` you can use: `make build ARCH=arm` or `make build ARCH=arm64`. To
build all architectures you can run `make all-build`.

Run `make container` to package the binary inside a container. It will
calculate the image tag based on the current VERSION (calculated from git tag
or commit - see `make version` to view the current version). To build
containers for the other supported architectures you can run
`make container ARCH=arm` or `make container ARCH=arm64`. To make all
containers run `make all-container`.

Run `make push` to push the container image to `REGISTRY`, and similarly you
can run `make push ARCH=arm` or `make push ARCH=arm64` to push different
architecture containers. To push all containers run `make all-push`.

Run `make clean` to clean up.

To remove all containers, volumes run `make teardown`.

## Testing

To run the test suite, use the make task `test`. This will run all testcases
inside a containerized environment but pointing at a different DB instance to
avoid overwriting any data stored in your local development DB.

In addition, there is a simple bash script (in `client/client.sh`) that uses
curl to exercise the basic functions of the API. The script inserts 4
entries, then paginates through them, before deleting all inserted data. The
purpose of this script is just to sanity check the functionality from the
command line.
