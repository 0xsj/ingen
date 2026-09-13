SHELL := /bin/sh

GO ?= go
GO_CACHE ?= $(CURDIR)/.cache/go-build
GO_MOD_CACHE ?= $(CURDIR)/.cache/go-mod
GO_CMD = GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" $(GO)

CONTRACT ?= examples/document-pipeline-lab/contract/contract.yaml
SEALED_DIR ?= .artifacts/document-pipeline-contract
SUBJECT_ADDR ?= :8080
SUBJECT_URL ?= http://localhost:8080
RUN_OUTPUT_DIR ?= .artifacts/document-pipeline-run

.DEFAULT_GOAL := help

.PHONY: help build test test-race vet check \
	contract-validate contract-seal subject-test subject-run sorna-run

help: ## Show the available development commands
	@awk 'BEGIN {FS = ":.*## "; printf "InGen commands:\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-20s %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

build: ## Compile every Go package
	$(GO_CMD) build ./...

test: ## Run all Go tests
	$(GO_CMD) test ./...

test-race: ## Run all Go tests with the race detector
	$(GO_CMD) test -race ./...

vet: ## Run the Go static analysis checks
	$(GO_CMD) vet ./...

check: test vet ## Run the normal test and analysis checks

contract-validate: ## Validate the document-pipeline contract
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(CONTRACT)"

contract-seal: ## Seal the document-pipeline contract into local artifacts
	mkdir -p "$(SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(CONTRACT)" --output-dir "$(SEALED_DIR)"

subject-test: ## Test the document-pipeline subject only
	$(GO_CMD) test ./examples/document-pipeline-lab/subject/...

subject-run: ## Run the document-pipeline subject on SUBJECT_ADDR
	$(GO_CMD) run ./examples/document-pipeline-lab/subject/cmd/document-pipeline -addr "$(SUBJECT_ADDR)"

sorna-run: ## Run Sorna against the subject at SUBJECT_URL
	$(GO_CMD) run ./sorna/cmd/sorna run --contract "$(CONTRACT)" --base-url "$(SUBJECT_URL)" --output-dir "$(RUN_OUTPUT_DIR)"
