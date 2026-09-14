SHELL := /bin/sh

GO ?= go
ROOT := $(abspath ..)
GO_CACHE ?= $(ROOT)/.cache/paddock-go-build
GO_MOD_CACHE ?= $(ROOT)/.cache/paddock-go-mod
GO_CMD = GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" $(GO)

BINARY_DIR ?= $(ROOT)/.artifacts/paddock
BINARY ?= $(BINARY_DIR)/paddock
VERSION ?= dev
COMMIT ?= unknown
BUILD_DATE ?= unknown
RELEASE_DIR ?= $(ROOT)/.artifacts/paddock-release/$(VERSION)
RELEASE_TARGETS ?= darwin/arm64 darwin/amd64 linux/amd64 linux/arm64
RELEASE_MANIFEST ?= $(RELEASE_DIR)/release-manifest.json
LDFLAGS := -X ingen/paddock/internal/version.Version=$(VERSION) -X ingen/paddock/internal/version.Commit=$(COMMIT) -X ingen/paddock/internal/version.BuildDate=$(BUILD_DATE)

.DEFAULT_GOAL := help

.PHONY: help build install test vet check release-check release-artifacts release-verify

help: ## Show Paddock development and release commands
	@awk 'BEGIN {FS = ":.*## "; printf "Paddock commands:\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

build: ## Build a reproducible Paddock binary under .artifacts/paddock
	mkdir -p "$(BINARY_DIR)"
	$(GO_CMD) build -trimpath -ldflags "$(LDFLAGS)" -o "$(BINARY)" ./cmd/paddock

install: ## Install Paddock with the requested version metadata
	$(GO_CMD) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/paddock

test: ## Run Paddock tests
	$(GO_CMD) test ./...

vet: ## Run Paddock static analysis
	$(GO_CMD) vet ./...

check: test vet ## Run Paddock tests and static analysis

release-check: build check ## Build and verify the release-shaped binary
	"$(BINARY)" version
	"$(BINARY)" version --format json

release-artifacts: check ## Build cross-platform Paddock archives and SHA-256 checksums
	GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" \
	PADDOCK_VERSION="$(VERSION)" PADDOCK_COMMIT="$(COMMIT)" PADDOCK_BUILD_DATE="$(BUILD_DATE)" \
	PADDOCK_RELEASE_DIR="$(RELEASE_DIR)" PADDOCK_TARGETS="$(RELEASE_TARGETS)" \
	./release.sh

release-verify: build ## Verify release archives against their manifest
	"$(BINARY)" release verify --manifest "$(RELEASE_MANIFEST)"
