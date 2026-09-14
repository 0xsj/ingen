SHELL := /bin/sh

GO ?= go
GO_CACHE ?= $(CURDIR)/.cache/go-build
GO_MOD_CACHE ?= $(CURDIR)/.cache/go-mod
GO_CMD = GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" $(GO)

CONTRACT ?= examples/document-pipeline-lab/contract/contract.yaml
POLICY ?= examples/document-pipeline-lab/policy/isolation.yaml
SEALED_DIR ?= .artifacts/document-pipeline-contract
SUBJECT_ADDR ?= :8080
SUBJECT_URL ?= http://localhost:8080
SUBJECT_READY_PATH ?= /healthz
RUN_OUTPUT_DIR ?= .artifacts/document-pipeline-run
DEFECT_ADDR ?= :8081
DEFECT_URL ?= http://localhost:8081
DEFECT_READY_PATH ?= /healthz
DEFECT_RUN_OUTPUT_DIR ?= .artifacts/document-pipeline-defect-status-200
SANDBOX_ROOT ?= .
SANDBOX_PROBE_PATH ?= examples/document-pipeline-lab/contract/contract.yaml
ORACLE_OUTPUT_DIR ?= .artifacts/document-pipeline-oracle

.DEFAULT_GOAL := help

.PHONY: help build test test-race vet check \
	contract-validate contract-seal policy-validate subject-test subject-run sorna-run \
	sorna-external-run evidence-verify oracle-evidence-verify sorna-oracle-freeze \
	subject-defect-run sorna-defect-run sandbox-contract-read

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

policy-validate: ## Validate the document-pipeline isolation policy
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$(POLICY)"

subject-test: ## Test the document-pipeline subject only
	$(GO_CMD) test ./examples/document-pipeline-lab/subject/...

subject-run: ## Run the document-pipeline subject on SUBJECT_ADDR
	$(GO_CMD) run ./examples/document-pipeline-lab/subject/cmd/document-pipeline -addr "$(SUBJECT_ADDR)"

sorna-run: sorna-oracle-freeze ## Freeze the oracle, launch the clean subject, run Sorna, and tear it down
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --base-url "$(SUBJECT_URL)" --subject-command "$(GO)" --subject-arg run --subject-arg ./examples/document-pipeline-lab/subject/cmd/document-pipeline --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant clean-baseline --output-dir "$(RUN_OUTPUT_DIR)"

sorna-external-run: sorna-oracle-freeze ## Freeze the oracle, then run against an already-running subject at SUBJECT_URL
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --base-url "$(SUBJECT_URL)" --subject-variant clean-baseline --output-dir "$(RUN_OUTPUT_DIR)"

evidence-verify: ## Verify the checksums in RUN_OUTPUT_DIR
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(RUN_OUTPUT_DIR)"

sandbox-contract-read: ## Run /bin/cat under the macOS Seatbelt policy backend
	$(GO_CMD) run ./sorna/cmd/sorna sandbox exec --policy "$(POLICY)" --root "$(SANDBOX_ROOT)" -- /bin/cat "$(SANDBOX_PROBE_PATH)"

sorna-oracle-freeze: ## Generate the document-pipeline oracle in a sandbox
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(CONTRACT)" --policy "$(POLICY)" --root . --output-dir "$(ORACLE_OUTPUT_DIR)"

oracle-evidence-verify: ## Verify the frozen oracle evidence bundle
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(ORACLE_OUTPUT_DIR)"

subject-defect-run: ## Run the status-200-create defect subject on DEFECT_ADDR
	$(GO_CMD) run ./examples/document-pipeline-lab/defects/status-200-create/cmd/document-pipeline-defect -addr "$(DEFECT_ADDR)"

sorna-defect-run: sorna-oracle-freeze ## Freeze the oracle, launch the defect subject, run Sorna, and tear it down
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --base-url "$(DEFECT_URL)" --subject-command "$(GO)" --subject-arg run --subject-arg ./examples/document-pipeline-lab/defects/status-200-create/cmd/document-pipeline-defect --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --ready-path "$(DEFECT_READY_PATH)" --subject-variant status-200-create --mutation-id status-200-create --mutation-plane behavior --mutation-description "valid document creation returns 200 instead of 202" --expected-rule document.create.valid.accepted --output-dir "$(DEFECT_RUN_OUTPUT_DIR)"
