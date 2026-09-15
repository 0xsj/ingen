SHELL := /bin/sh

GO ?= go
GO_CACHE ?= $(CURDIR)/.cache/go-build
GO_MOD_CACHE ?= $(CURDIR)/.cache/go-mod
GO_CMD = GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" $(GO)
ARTIFACT_ROOT ?= .artifacts
CONTRACT ?= examples/document-pipeline-lab/contract/contract.yaml
POLICY ?= examples/document-pipeline-lab/policy/isolation.yaml
SEALED_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-contract
SUBJECT_ADDR ?= 127.0.0.1:8080
SUBJECT_URL ?= http://127.0.0.1:8080
SUBJECT_READY_PATH ?= /healthz
SUBJECT_POLICY ?= examples/document-pipeline-lab/policy/subject.yaml
SUBJECT_ROOT ?= .
SUBJECT_BINARY_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-subject
SUBJECT_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline
RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-run
CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-ci-result.json
NUBLAR_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/nublar-result.json
NUBLAR_WORKFLOW ?= nublar/workflows/document-pipeline.yaml
DEFECT_ADDR ?= 127.0.0.1:8081
DEFECT_URL ?= http://127.0.0.1:8081
DEFECT_READY_PATH ?= /healthz
DEFECT_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-defect-status-200
DEFECT_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline-defect
DEFECT_REMOVE_NAME_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline-defect-remove-name
DEFECT_UNSUPPORTED_TYPE_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline-defect-unsupported-type
DEFECT_PROCESS_STAYS_QUEUED_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline-defect-process-stays-queued
DEFECT_PERSISTENCE_WRONG_KEY_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline-defect-persistence-wrong-key
DEFECT_ACCEPTS_PNG_BINARY ?= $(SUBJECT_BINARY_DIR)/document-pipeline-defect-accepts-png
SANDBOX_ROOT ?= .
SANDBOX_PROBE_PATH ?= examples/document-pipeline-lab/contract/contract.yaml
ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-oracle
MUTATION_CATALOGUE ?= examples/document-pipeline-lab/mutations/catalogue.yaml
MUTATION_PLAN_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-mutation-plan.json
MUTATION_PROVIDER ?= examples/document-pipeline-lab/mutations/provider.yaml
MUTATION_PROVIDER_REQUIRE_PLAN_BINDING ?= false
MUTATION_PROVIDER_BINDING_FLAG = $(if $(filter true 1 yes,$(MUTATION_PROVIDER_REQUIRE_PLAN_BINDING)),--require-plan-binding,)
MUTATION_PROVIDER_REVIEW_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-provider-review.json
MUTATION_PROVIDER_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-provider-review-ci-result.json
MUTATION_CAMPAIGN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-campaign
MUTATION_CAMPAIGN_RESULT_OUTPUT ?= $(MUTATION_CAMPAIGN_OUTPUT_DIR)/campaign-result.json
MUTATION_CAMPAIGN_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-campaign-ci-result.json
MUTATION_GO_PROVIDER_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-go-provider
MUTATION_GO_PROVIDER_BINARY_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-subject/go-mutations
MUTATION_GO_PROVIDER ?= $(MUTATION_GO_PROVIDER_OUTPUT_DIR)/provider.yaml
MUTATION_GO_PROVIDER_SUMMARY ?= $(MUTATION_GO_PROVIDER_OUTPUT_DIR)/preparation.json
MUTATION_GO_PROVIDER_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-go-provider-ci-result.json
MUTATION_GO_PREPARATION_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-go-preparation-ci-result.json
MUTATION_GO_CAMPAIGN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-go-campaign
MUTATION_GO_CAMPAIGN_RESULT_OUTPUT ?= $(MUTATION_GO_CAMPAIGN_OUTPUT_DIR)/campaign-result.json
MUTATION_GO_CAMPAIGN_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-go-campaign-ci-result.json
MUTATION_SURVIVOR_CATALOGUE ?= examples/document-pipeline-lab/mutations/survivor-catalogue.yaml
MUTATION_SURVIVOR_PLAN_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-survivor-mutation-plan.json
MUTATION_SURVIVOR_PROVIDER_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-survivor-go-provider
MUTATION_SURVIVOR_PROVIDER_BINARY_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-subject/survivor-go-mutations
MUTATION_SURVIVOR_PROVIDER ?= $(MUTATION_SURVIVOR_PROVIDER_OUTPUT_DIR)/provider.yaml
MUTATION_SURVIVOR_PROVIDER_SUMMARY ?= $(MUTATION_SURVIVOR_PROVIDER_OUTPUT_DIR)/preparation.json
MUTATION_SURVIVOR_PROVIDER_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-survivor-go-provider-ci-result.json
MUTATION_SURVIVOR_PREPARATION_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-survivor-go-preparation-ci-result.json
MUTATION_SURVIVOR_CAMPAIGN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-survivor-go-campaign
MUTATION_SURVIVOR_CAMPAIGN_RESULT_OUTPUT ?= $(MUTATION_SURVIVOR_CAMPAIGN_OUTPUT_DIR)/campaign-result.json
MUTATION_SURVIVOR_CAMPAIGN_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-survivor-go-campaign-ci-result.json

.DEFAULT_GOAL := help

.PHONY: help build test test-race vet check alpha-interface-check \
	contract-validate contract-seal policy-validate subject-policy-validate subject-test subject-run subject-build defect-build sorna-run \
	sorna-external-run evidence-verify oracle-evidence-verify sorna-gate sorna-ci-result nublar-aggregate nublar-aggregate-fresh sorna-oracle-freeze \
	subject-defect-run sorna-defect-run mutation-catalogue-validate mutation-plan mutation-provider-validate mutation-provider-inspect mutation-provider-ci-result mutation-campaign-run mutation-campaign-verify mutation-campaign-ci-result mutation-go-provider-build mutation-go-provider-ci-result mutation-go-preparation-ci-result mutation-go-campaign-run mutation-go-campaign-verify mutation-go-campaign-ci-result mutation-go-survivor-run mutation-go-survivor-ci-result mutation-go-survivor-ci-result-fresh sandbox-contract-read defect-remove-name-build defect-unsupported-type-build defect-process-stays-queued-build defect-persistence-wrong-key-build defect-accepts-png-build

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

alpha-interface-check: ## Run tests and analysis for the current Sorna/Nublar alpha boundary
	$(GO_CMD) test ./core/... ./sorna/... ./examples/... ./nublar/...
	$(GO_CMD) vet ./core/... ./sorna/... ./examples/... ./nublar/...

contract-validate: ## Validate the document-pipeline contract
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(CONTRACT)"

contract-seal: ## Seal the document-pipeline contract into local artifacts
	mkdir -p "$(SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(CONTRACT)" --output-dir "$(SEALED_DIR)"

policy-validate: ## Validate the document-pipeline isolation policy
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$(POLICY)"

subject-policy-validate: ## Validate the managed-subject isolation policy
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$(SUBJECT_POLICY)"

subject-test: ## Test the document-pipeline subject only
	$(GO_CMD) test ./examples/document-pipeline-lab/subject/...

subject-run: ## Run the document-pipeline subject on SUBJECT_ADDR
	$(GO_CMD) run ./examples/document-pipeline-lab/subject/cmd/document-pipeline -addr "$(SUBJECT_ADDR)"

subject-build: ## Build the clean document-pipeline subject binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -trimpath -o "$(SUBJECT_BINARY)" ./examples/document-pipeline-lab/subject/cmd/document-pipeline

defect-build: ## Build the controlled status-200-create defect binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -o "$(DEFECT_BINARY)" ./examples/document-pipeline-lab/defects/status-200-create/cmd/document-pipeline-defect

defect-remove-name-build: ## Build the controlled remove-name-create defect binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -o "$(DEFECT_REMOVE_NAME_BINARY)" ./examples/document-pipeline-lab/defects/remove-name-create/cmd/document-pipeline-defect

defect-unsupported-type-build: ## Build the controlled unsupported-type-500 defect binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -o "$(DEFECT_UNSUPPORTED_TYPE_BINARY)" ./examples/document-pipeline-lab/defects/unsupported-type-500/cmd/document-pipeline-defect

defect-process-stays-queued-build: ## Build the controlled process-stays-queued defect binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -o "$(DEFECT_PROCESS_STAYS_QUEUED_BINARY)" ./examples/document-pipeline-lab/defects/process-stays-queued/cmd/document-pipeline-defect

defect-persistence-wrong-key-build: ## Build the controlled persistence-wrong-key defect binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -o "$(DEFECT_PERSISTENCE_WRONG_KEY_BINARY)" ./examples/document-pipeline-lab/defects/persistence-wrong-key/cmd/document-pipeline-defect

defect-accepts-png-build: ## Build the controlled accepts-png defect binary
	mkdir -p "$(SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -o "$(DEFECT_ACCEPTS_PNG_BINARY)" ./examples/document-pipeline-lab/defects/accepts-png/cmd/document-pipeline-defect

sorna-run: sorna-oracle-freeze subject-build ## Freeze the oracle, launch the isolated clean subject, run Sorna, and tear it down
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --subject-policy "$(SUBJECT_POLICY)" --subject-root "$(SUBJECT_ROOT)" --base-url "$(SUBJECT_URL)" --subject-command "$(SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant clean-baseline --output-dir "$(RUN_OUTPUT_DIR)"

sorna-external-run: sorna-oracle-freeze ## Freeze the oracle, then run against an already-running subject at SUBJECT_URL
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --base-url "$(SUBJECT_URL)" --subject-variant clean-baseline --output-dir "$(RUN_OUTPUT_DIR)"

evidence-verify: ## Verify the checksums in RUN_OUTPUT_DIR
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(RUN_OUTPUT_DIR)"

sorna-gate: ## Apply the default CI gate to RUN_OUTPUT_DIR; set GATE_MIN_OBSERVATION_COVERAGE for a strict minimum
	$(GO_CMD) run ./sorna/cmd/sorna gate $(if $(GATE_MIN_OBSERVATION_COVERAGE),--minimum-observation-coverage "$(GATE_MIN_OBSERVATION_COVERAGE)",) "$(RUN_OUTPUT_DIR)"

sorna-ci-result: sorna-run ## Run the clean baseline and write its language-neutral CI result envelope
	mkdir -p "$(dir $(CI_RESULT_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna gate --format ci-result $(if $(GATE_MIN_OBSERVATION_COVERAGE),--minimum-observation-coverage "$(GATE_MIN_OBSERVATION_COVERAGE)",) "$(RUN_OUTPUT_DIR)" > "$(CI_RESULT_OUTPUT)"

nublar-aggregate: sorna-ci-result mutation-go-provider-ci-result mutation-go-preparation-ci-result mutation-go-campaign-ci-result ## Aggregate Sorna behavioral, preparation, strict provider-preflight, and Go mutation CI results through Nublar
	mkdir -p "$(dir $(NUBLAR_RESULT_OUTPUT))"
	$(GO_CMD) run ./nublar/cmd/nublar aggregate --workflow "$(NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --output "$(NUBLAR_RESULT_OUTPUT)"

nublar-aggregate-fresh: ## Run the complete document-pipeline workflow in a fresh temporary workspace
	workspace=$$(mktemp -d /private/tmp/ingen-workspace.XXXXXX); \
	trap 'printf "workspace: %s\\nartifact root: %s\\n" "$$workspace" "$$workspace/.artifacts"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" nublar-aggregate; status=$$?; \
	exit $$status
	exit $$status

sandbox-contract-read: ## Run /bin/cat under the macOS Seatbelt policy backend
	$(GO_CMD) run ./sorna/cmd/sorna sandbox exec --policy "$(POLICY)" --root "$(SANDBOX_ROOT)" -- /bin/cat "$(SANDBOX_PROBE_PATH)"

sorna-oracle-freeze: ## Generate the document-pipeline oracle in a sandbox
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(CONTRACT)" --policy "$(POLICY)" --root . --output-dir "$(ORACLE_OUTPUT_DIR)"

oracle-evidence-verify: ## Verify the frozen oracle evidence bundle
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(ORACLE_OUTPUT_DIR)"

mutation-catalogue-validate: ## Validate the document-pipeline mutation catalogue
	$(GO_CMD) run ./sorna/cmd/sorna mutation validate "$(MUTATION_CATALOGUE)" --contract "$(CONTRACT)"

mutation-plan: sorna-run mutation-catalogue-validate ## Build a ready mutation campaign plan from the frozen oracle and clean baseline
	mkdir -p "$(dir $(MUTATION_PLAN_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna mutation plan "$(MUTATION_CATALOGUE)" --contract "$(CONTRACT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --baseline-evidence "$(RUN_OUTPUT_DIR)" --subject-policy "$(SUBJECT_POLICY)" --output "$(MUTATION_PLAN_OUTPUT)"

mutation-provider-validate: ## Validate the document-pipeline mutation provider manifest
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider validate "$(MUTATION_PROVIDER)"

mutation-provider-inspect: mutation-plan mutation-provider-validate ## Review provider capabilities against the ready mutation plan without launching subjects
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(MUTATION_PLAN_OUTPUT)" --provider "$(MUTATION_PROVIDER)" $(MUTATION_PROVIDER_BINDING_FLAG) --format json --output "$(MUTATION_PROVIDER_REVIEW_OUTPUT)"

mutation-provider-ci-result: mutation-plan mutation-provider-validate ## Write the provider preflight as a shared CI result envelope
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(MUTATION_PLAN_OUTPUT)" --provider "$(MUTATION_PROVIDER)" $(MUTATION_PROVIDER_BINDING_FLAG) --format ci-result --output "$(MUTATION_PROVIDER_CI_RESULT_OUTPUT)"

mutation-campaign-run: mutation-plan defect-build defect-remove-name-build defect-unsupported-type-build defect-process-stays-queued-build defect-persistence-wrong-key-build defect-accepts-png-build mutation-provider-validate ## Execute every planned document-pipeline mutation in an isolated fresh subject
	$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(MUTATION_PLAN_OUTPUT)" --provider "$(MUTATION_PROVIDER)" $(MUTATION_PROVIDER_BINDING_FLAG) --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --subject-policy "$(SUBJECT_POLICY)" --base-address "$(DEFECT_ADDR)" --output-dir "$(MUTATION_CAMPAIGN_OUTPUT_DIR)" --output "$(MUTATION_CAMPAIGN_RESULT_OUTPUT)"

mutation-campaign-verify: ## Verify campaign entries against their recorded evidence hashes
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(MUTATION_CAMPAIGN_RESULT_OUTPUT)"

mutation-campaign-ci-result: ## Verify the mutation campaign and write a shared CI result envelope
	-$(MAKE) mutation-campaign-run
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(MUTATION_CAMPAIGN_RESULT_OUTPUT)" --format ci-result --source-root . --output "$(MUTATION_CAMPAIGN_CI_RESULT_OUTPUT)"

mutation-go-provider-build: mutation-plan ## Copy, mutate, and build Go variants from the reviewed campaign plan
	$(GO_CMD) run ./sorna/cmd/sorna-go-provider --plan "$(MUTATION_PLAN_OUTPUT)" --source-root . --output-dir "$(MUTATION_GO_PROVIDER_OUTPUT_DIR)" --binary-dir "$(MUTATION_GO_PROVIDER_BINARY_DIR)" --provider "$(MUTATION_GO_PROVIDER)" --summary-output "$(MUTATION_GO_PROVIDER_SUMMARY)"

mutation-go-provider-ci-result: mutation-go-provider-build ## Review the generated Go provider with mandatory plan binding
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(MUTATION_PLAN_OUTPUT)" --provider "$(MUTATION_GO_PROVIDER)" --require-plan-binding --format ci-result --output "$(MUTATION_GO_PROVIDER_CI_RESULT_OUTPUT)"

mutation-go-preparation-ci-result: mutation-go-provider-build ## Expose Go provider preparation as a shared CI result
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider preparation "$(MUTATION_GO_PROVIDER_SUMMARY)" --provider "$(MUTATION_GO_PROVIDER)" --source-root . --format ci-result --output "$(MUTATION_GO_PREPARATION_CI_RESULT_OUTPUT)"

mutation-go-campaign-run: mutation-go-provider-build ## Execute the campaign using the source-level Go provider
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider validate "$(MUTATION_GO_PROVIDER)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(MUTATION_PLAN_OUTPUT)" --provider "$(MUTATION_GO_PROVIDER)" --require-plan-binding --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --subject-policy "$(SUBJECT_POLICY)" --base-address "$(DEFECT_ADDR)" --output-dir "$(MUTATION_GO_CAMPAIGN_OUTPUT_DIR)" --output "$(MUTATION_GO_CAMPAIGN_RESULT_OUTPUT)"


mutation-go-campaign-ci-result: mutation-go-provider-ci-result ## Verify the strict Go campaign and write a shared CI result envelope
	-$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(MUTATION_PLAN_OUTPUT)" --provider "$(MUTATION_GO_PROVIDER)" --require-plan-binding --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --subject-policy "$(SUBJECT_POLICY)" --base-address "$(DEFECT_ADDR)" --output-dir "$(MUTATION_GO_CAMPAIGN_OUTPUT_DIR)" --output "$(MUTATION_GO_CAMPAIGN_RESULT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(MUTATION_GO_CAMPAIGN_RESULT_OUTPUT)" --format ci-result --source-root . --output "$(MUTATION_GO_CAMPAIGN_CI_RESULT_OUTPUT)"

mutation-go-campaign-verify: ## Verify the source-level Go campaign result and evidence bindings
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(MUTATION_GO_CAMPAIGN_RESULT_OUTPUT)"

mutation-go-survivor-run: sorna-run ## Re-run the former survivor diagnostic and require it to be killed
	$(GO_CMD) run ./sorna/cmd/sorna mutation validate "$(MUTATION_SURVIVOR_CATALOGUE)" --contract "$(CONTRACT)"
	mkdir -p "$(dir $(MUTATION_SURVIVOR_PLAN_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna mutation plan "$(MUTATION_SURVIVOR_CATALOGUE)" --contract "$(CONTRACT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --baseline-evidence "$(RUN_OUTPUT_DIR)" --subject-policy "$(SUBJECT_POLICY)" --output "$(MUTATION_SURVIVOR_PLAN_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-go-provider --plan "$(MUTATION_SURVIVOR_PLAN_OUTPUT)" --source-root . --output-dir "$(MUTATION_SURVIVOR_PROVIDER_OUTPUT_DIR)" --binary-dir "$(MUTATION_SURVIVOR_PROVIDER_BINARY_DIR)" --provider "$(MUTATION_SURVIVOR_PROVIDER)" --summary-output "$(MUTATION_SURVIVOR_PROVIDER_SUMMARY)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(MUTATION_SURVIVOR_PLAN_OUTPUT)" --provider "$(MUTATION_SURVIVOR_PROVIDER)" --require-plan-binding --format ci-result --output "$(MUTATION_SURVIVOR_PROVIDER_CI_RESULT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider preparation "$(MUTATION_SURVIVOR_PROVIDER_SUMMARY)" --provider "$(MUTATION_SURVIVOR_PROVIDER)" --source-root . --format ci-result --output "$(MUTATION_SURVIVOR_PREPARATION_CI_RESULT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(MUTATION_SURVIVOR_PLAN_OUTPUT)" --provider "$(MUTATION_SURVIVOR_PROVIDER)" --require-plan-binding --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --subject-policy "$(SUBJECT_POLICY)" --base-address "$(DEFECT_ADDR)" --output-dir "$(MUTATION_SURVIVOR_CAMPAIGN_OUTPUT_DIR)" --output "$(MUTATION_SURVIVOR_CAMPAIGN_RESULT_OUTPUT)"

mutation-go-survivor-ci-result: mutation-go-survivor-run ## Emit the former survivor diagnostic as a passing CI result
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(MUTATION_SURVIVOR_CAMPAIGN_RESULT_OUTPUT)" --format ci-result --source-root . --output "$(MUTATION_SURVIVOR_CAMPAIGN_CI_RESULT_OUTPUT)"

mutation-go-survivor-ci-result-fresh: ## Run the survivor diagnostic in a fresh workspace
	workspace=$$(mktemp -d /private/tmp/ingen-survivor-workspace.XXXXXX); \
	trap 'printf "workspace: %s\\nartifact root: %s\\n" "$$workspace" "$$workspace/.artifacts"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" mutation-go-survivor-ci-result; status=$$?; \
	exit $$status

subject-defect-run: ## Run the status-200-create defect subject on DEFECT_ADDR
	$(GO_CMD) run ./examples/document-pipeline-lab/defects/status-200-create/cmd/document-pipeline-defect -addr "$(DEFECT_ADDR)"

sorna-defect-run: sorna-run defect-build ## Reuse the passing clean baseline, then run the isolated defect subject through Sorna
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(POLICY)" --subject-policy "$(SUBJECT_POLICY)" --subject-root "$(SUBJECT_ROOT)" --baseline-evidence "$(RUN_OUTPUT_DIR)" --base-url "$(DEFECT_URL)" --subject-command "$(DEFECT_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --ready-path "$(DEFECT_READY_PATH)" --subject-variant status-200-create --mutation-id status-200-create --mutation-plane implementation --mutation-description "valid document creation returns 200 instead of 202" --expected-rule document.create.valid.accepted --output-dir "$(DEFECT_RUN_OUTPUT_DIR)"
