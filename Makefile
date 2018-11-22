# Template for Go apps

# The binary to build (just the basename)
BIN := iotenc

# The projects root import path (under GOPATH)
PKG := github.com/DECODEproject/iotencoder

# Docker Hub ID to which docker images should be pushed
REGISTRY ?= thingful

# Default architecture to build
ARCH ?= amd64

# Version string - to be added to the binary
VERSION := $(shell git describe --tags --always --dirty)

# Build date - to be added to the binary
BUILD_DATE := $(shell date -u "+%FT%H:%M:%S%Z")

# Whether to enable CGO, set to 0 to disable it.
CGO_ENABLED := 1

# Do not change the following variables

PWD := $(shell pwd)

SRC_DIRS := cmd pkg

ALL_ARCH := amd64 arm arm64

ifeq ($(ARCH),amd64)
	BASE_IMAGE?=busybox:glibc
	BUILD_IMAGE?=golang:1.11-stretch
endif
ifeq ($(ARCH),arm)
	BASE_IMAGE?=arm32v7/busybox
	BUILD_IMAGE?=arm32v7/golang:1.11-stretch
endif
ifeq ($(ARCH),arm64)
	BASE_IMAGE?=arm64v8/busybox
	BUILD_IMAGE?=arm64v8/golang:1.11
endif

IMAGE := $(REGISTRY)/$(BIN)-$(ARCH)
all: build

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

container-%:
	@$(MAKE) --no-print-directory ARCH=$* container

push-%:
	@$(MAKE) --no-print-directory ARCH=$* push

all-build: $(addprefix build-, $(ALL_ARCH))

all-container: $(addprefix container-, $(ALL_ARCH))

all-push: $(addprefix push-, $(ALL_ARCH))

build: bin/$(ARCH)/$(BIN) ## Build our binary inside a container

bin/$(ARCH)/$(BIN): .build-dirs .compose
	@echo "--> Building in the containerized environment"
	@docker-compose -f .docker-compose-$(ARCH).yml build
	@docker-compose -f .docker-compose-$(ARCH).yml \
		run \
		--rm \
		-u $$(id -u):$$(id -g) \
		--no-deps \
		app \
		/bin/sh -c " \
			ARCH=$(ARCH) \
			VERSION=$(VERSION) \
			PKG=$(PKG) \
			BUILD_DATE=$(BUILD_DATE) \
			BINARY_NAME=$(BIN) \
			CGO_ENABLED=$(CGO_ENABLED) \
			./build/build.sh \
		"

shell: .shell-$(ARCH) ## Open shell in containerized environment
.shell-$(ARCH): .build-dirs .compose
	@echo "--> Launching shell in the containerized environment"
	@docker-compose -f .docker-compose-$(ARCH).yml \
		run \
		--rm \
		-u "$$(id -u):$$(id -g)" \
		app \
		/bin/sh -c " \
			./build/dev.sh\
		"

.PHONY: test
test: .build-dirs .compose ## Run tests in the containerized environment
	@echo "--> Running tests in the containerized environment"
	@docker-compose -f .docker-compose-$(ARCH).yml \
		run \
		--rm \
		-u $$(id -u):$$(id -g) \
		-e "IOTENCODER_DATABASE_URL=postgres://iotenc:password@postgres/iotenc_test?sslmode=disable" \
		app \
		/bin/sh -c " \
			RUN=$(RUN) \
			CGO_ENABLED=$(CGO_ENABLED) \
			./build/test.sh $(SRC_DIRS) \
		"

DOTFILE_IMAGE = $(subst :,_,$(subst /,_,$(IMAGE))-$(VERSION))

container: .container-$(DOTFILE_IMAGE) container-name ## Create delivery container image
.container-$(DOTFILE_IMAGE): bin/$(ARCH)/$(BIN) Dockerfile.in
	@sed \
		-e 's|ARG_BIN|$(BIN)|g' \
		-e 's|ARG_ARCH|$(ARCH)|g' \
		-e 's|ARG_FROM|$(BASE_IMAGE)|g' \
		Dockerfile.in > .dockerfile-in-$(ARCH)
	@docker build -t $(IMAGE):$(VERSION) -f .dockerfile-in-$(ARCH) .
	@docker images -q $(IMAGE):$(VERSION) > $@

.PHONY: container-name
container-name: ## Show the name of the delivery container
	@echo "  container: $(IMAGE):$(VERSION)"

.PHONY: .compose
.compose: ## Create environment specific compose file
	@sed \
		-e 's|ARG_FROM|$(BUILD_IMAGE)|g' \
		-e 's|ARG_WORKDIR|/go/src/$(PKG)|g' \
		Dockerfile.dev > .dockerfile-dev-$(ARCH)
	@sed \
		-e 's|ARG_DOCKERFILE|.dockerfile-dev-$(ARCH)|g' \
		-e 's|ARG_IMAGE|$(IMAGE)-dev:$(VERSION)|g' \
		-e 's|ARG_PWD|$(PWD)|g' \
		-e 's|ARG_PKG|$(PKG)|g' \
		-e 's|ARG_ARCH|$(ARCH)|g' \
		-e 's|ARG_BIN|$(BIN)|g' \
		docker-compose.yml > .docker-compose-$(ARCH).yml

.PHONY: .build-dirs
.build-dirs: ## creates build directories
	@mkdir -p bin/$(ARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(ARCH) .cache/go-build .coverage

.PHONY: version
version: ## returns the current version
	@echo Version: $(VERSION) - $(BUILD_DATE) $(IMAGE)

.PHONY: push
push: .push-$(DOTFILE_IMAGE) push-name
.push-$(DOTFILE_IMAGE):
	@docker push $(IMAGE):$(VERSION)
	@docker images -q $(IMAGE):$(VERSION) > $@

.PHONY: push-name
push-name:
	@echo "  pushed $(IMAGE):$(VERSION)"

.PHONY: start
start: .compose ## start compose services
	@docker-compose -f .docker-compose-$(ARCH).yml \
		up

.PHONY: teardown
teardown: .compose ## teardown compose services
	@docker-compose -f .docker-compose-$(ARCH).yml \
		down -v

.PHONY: clean
clean: container-clean bin-clean ## remove all artefacts

.PHONY: container-clean
container-clean: ## clean container artefacts
	@rm -rf .container-* .dockerfile-* .docker-compose-* .push-*

.PHONY: bin-clean
bin-clean: ## remove generated build artefacts
	@rm -rf .go bin .cache .coverage

.PHONY: psql
psql: ## open psql shell
	@docker-compose -f .docker-compose-$(ARCH).yml \
    run \
    --rm \
    -e "PGHOST=postgres" \
    -e "PGUSER=iotenc" \
    -e "PGPASSWORD=password" \
    postgres \
    psql \
    iotenc_development