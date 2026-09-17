SHELL := /bin/sh

GO ?= go
GO_CACHE ?= $(CURDIR)/.cache/go-build
GO_MOD_CACHE ?= $(CURDIR)/.cache/go-mod
GO_CMD = GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" $(GO)
ARTIFACT_ROOT ?= .artifacts
SORNA_BINARY_DIR ?= $(ARTIFACT_ROOT)/sorna
SORNA_BINARY ?= $(SORNA_BINARY_DIR)/sorna
SORNA_VERSION ?= dev
SORNA_COMMIT ?= unknown
SORNA_BUILD_DATE ?= unknown
SORNA_LDFLAGS := -X ingen/sorna/internal/version.Version=$(SORNA_VERSION) -X ingen/sorna/internal/version.Commit=$(SORNA_COMMIT) -X ingen/sorna/internal/version.BuildDate=$(SORNA_BUILD_DATE)
SORNA_RELEASE_DIR ?= $(ARTIFACT_ROOT)/sorna-release/$(SORNA_VERSION)
SORNA_RELEASE_TARGETS ?= darwin/arm64 darwin/amd64 linux/amd64 linux/arm64
SORNA_RELEASE_MANIFEST ?= $(SORNA_RELEASE_DIR)/release-manifest.json
SORNA_RELEASE_VERIFICATION ?= $(SORNA_RELEASE_DIR)/release-verification.json
SORNA_RELEASE_PROVENANCE ?= $(SORNA_RELEASE_DIR)/release-provenance.json
SORNA_RELEASE_REPOSITORY ?= local
SORNA_RELEASE_REF ?= local
SORNA_RELEASE_TAG ?= sorna-v$(SORNA_VERSION)
SORNA_RELEASE_WORKFLOW ?= local
SORNA_RELEASE_RUN_ID ?= local
SORNA_RELEASE_RUN_ATTEMPT ?= 1
SORNA_RELEASE_RUNNER ?= local
MALCOLM_EXAMPLE ?= malcolm/examples/healthz.malcolm
MALCOLM_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-healthz.ir.json
MALCOLM_SORNA_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-healthz-contract.json
MALCOLM_ORACLE_POLICY ?= malcolm/examples/healthz/oracle-policy.yaml
MALCOLM_SUBJECT_POLICY ?= malcolm/examples/healthz/subject-policy.yaml
MALCOLM_SORNA_SEALED_DIR ?= $(ARTIFACT_ROOT)/malcolm-healthz-contract-sealed
MALCOLM_SORNA_ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-healthz-oracle
MALCOLM_SORNA_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-healthz-run
MALCOLM_FLOW_EXAMPLE ?= malcolm/examples/document_flow.malcolm
MALCOLM_FLOW_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-flow.ir.json
MALCOLM_FLOW_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-flow-contract.json
MALCOLM_FLOW_ORACLE_POLICY ?= malcolm/examples/document_flow/oracle-policy.yaml
MALCOLM_FLOW_SUBJECT_POLICY ?= malcolm/examples/document_flow/subject-policy.yaml
MALCOLM_FLOW_SEALED_DIR ?= $(ARTIFACT_ROOT)/malcolm-flow-contract-sealed
MALCOLM_FLOW_ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-flow-oracle
MALCOLM_FLOW_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-flow-run
MALCOLM_FLOW_EVENT_DEFECT_BINARY ?= $(ARTIFACT_ROOT)/document-pipeline-subject/malcolm-flow-event-defect
MALCOLM_FLOW_EVENT_DEFECT_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-flow-event-defect-run
MALCOLM_FLOW_EVENT_DEFECT_ADDR ?= $(SUBJECT_ADDR)
MALCOLM_FLOW_EVENT_DEFECT_URL ?= $(SUBJECT_URL)
MALCOLM_BOUNDARY_EXAMPLE ?= malcolm/examples/document_boundary.malcolm
MALCOLM_BOUNDARY_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-boundary.ir.json
MALCOLM_BOUNDARY_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-boundary-contract.json
MALCOLM_BOUNDARY_ORACLE_POLICY ?= malcolm/examples/document_boundary/oracle-policy.yaml
MALCOLM_BOUNDARY_SUBJECT_POLICY ?= malcolm/examples/document_boundary/subject-policy.yaml
MALCOLM_BOUNDARY_SEALED_DIR ?= $(ARTIFACT_ROOT)/malcolm-boundary-contract-sealed
MALCOLM_BOUNDARY_ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-boundary-oracle
MALCOLM_BOUNDARY_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-boundary-run
MALCOLM_INTERPOLATION_EXAMPLE ?= malcolm/examples/document_interpolation.malcolm
MALCOLM_INTERPOLATION_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-interpolation.ir.json
MALCOLM_INTERPOLATION_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-interpolation-contract.json
MALCOLM_INTERPOLATION_ORACLE_POLICY ?= malcolm/examples/document_interpolation/oracle-policy.yaml
MALCOLM_INTERPOLATION_SUBJECT_POLICY ?= malcolm/examples/document_interpolation/subject-policy.yaml
MALCOLM_INTERPOLATION_SEALED_DIR ?= $(ARTIFACT_ROOT)/malcolm-interpolation-contract-sealed
MALCOLM_INTERPOLATION_ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-interpolation-oracle
MALCOLM_INTERPOLATION_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/malcolm-interpolation-run
MALCOLM_MUTATION_EXAMPLE ?= malcolm/examples/document_mutation.malcolm
MALCOLM_MUTATION_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-mutation.ir.json
MALCOLM_MUTATION_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-mutation-contract.json
MALCOLM_MUTATION_CATALOGUE_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-mutation-catalogue.json
MALCOLM_PROVENANCE_EXAMPLE ?= malcolm/examples/document_provenance.malcolm
MALCOLM_PROVENANCE_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-provenance.ir.json
MALCOLM_PROVENANCE_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-provenance-contract.json
MALCOLM_FIXTURE_EXAMPLE ?= malcolm/examples/document_fixture.malcolm
MALCOLM_FIXTURE_IR_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-fixture.ir.json
MALCOLM_FIXTURE_CONTRACT_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-fixture-contract.json
MALCOLM_FIXTURE_SEALED_DIR ?= $(ARTIFACT_ROOT)/malcolm-fixture-contract-sealed
MALCOLM_FIXTURE_PROVIDER ?= malcolm/examples/document_fixture_provider/provider.yaml
MALCOLM_FIXTURE_HANDOFF_OUTPUT ?= $(ARTIFACT_ROOT)/malcolm-fixture-handoff.json
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
NUBLAR_RUN_OUTPUT ?= $(ARTIFACT_ROOT)/nublar-run.json
NUBLAR_RUN_STORE ?= $(ARTIFACT_ROOT)/nublar-runs
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
WEBHOOK_CONTRACT ?= examples/webhook-validation-lab/contract/contract.yaml
WEBHOOK_POLICY ?= examples/webhook-validation-lab/policy/isolation.yaml
WEBHOOK_SUBJECT_POLICY ?= examples/webhook-validation-lab/policy/subject.yaml
WEBHOOK_SUBJECT_ADDR ?= 127.0.0.1:8090
WEBHOOK_SUBJECT_URL ?= http://127.0.0.1:8090
WEBHOOK_SUBJECT_BINARY_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-subject
WEBHOOK_SUBJECT_BINARY ?= $(WEBHOOK_SUBJECT_BINARY_DIR)/webhook-validation
WEBHOOK_ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-oracle
WEBHOOK_ORACLE_OUTPUT ?= $(WEBHOOK_ORACLE_OUTPUT_DIR)/oracle.json
WEBHOOK_RUN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-run
WEBHOOK_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-ci-result.json
WEBHOOK_MUTATION_CATALOGUE ?= examples/webhook-validation-lab/mutations/catalogue.yaml
WEBHOOK_MUTATION_PLAN_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-mutation-plan.json
WEBHOOK_MUTATION_PROVIDER ?= examples/webhook-validation-lab/mutations/provider.yaml
WEBHOOK_MUTATION_PROVIDER_REQUIRE_PLAN_BINDING ?= false
WEBHOOK_MUTATION_PROVIDER_BINDING_FLAG = $(if $(filter true 1 yes,$(WEBHOOK_MUTATION_PROVIDER_REQUIRE_PLAN_BINDING)),--require-plan-binding,)
WEBHOOK_MUTATION_PROVIDER_REVIEW_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-provider-review.json
WEBHOOK_MUTATION_PROVIDER_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-provider-review-ci-result.json
WEBHOOK_MUTATION_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-mutation
WEBHOOK_MUTATION_RESULT_OUTPUT ?= $(WEBHOOK_MUTATION_OUTPUT_DIR)/campaign-result.json
WEBHOOK_MUTATION_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-mutation-ci-result.json
WEBHOOK_DEFECT_ADDR ?= 127.0.0.1:8091
WEBHOOK_DEFECT_BINARY ?= $(WEBHOOK_SUBJECT_BINARY_DIR)/webhook-validation-defect
WEBHOOK_GO_PROVIDER_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-go-provider
WEBHOOK_GO_PROVIDER_BINARY_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-subject/go-mutations
WEBHOOK_GO_PROVIDER ?= $(WEBHOOK_GO_PROVIDER_OUTPUT_DIR)/provider.yaml
WEBHOOK_GO_PROVIDER_SUMMARY ?= $(WEBHOOK_GO_PROVIDER_OUTPUT_DIR)/preparation.json
WEBHOOK_GO_PROVIDER_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-go-provider-ci-result.json
WEBHOOK_GO_PREPARATION_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-go-preparation-ci-result.json
WEBHOOK_GO_CAMPAIGN_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/webhook-validation-go-campaign
WEBHOOK_GO_CAMPAIGN_RESULT_OUTPUT ?= $(WEBHOOK_GO_CAMPAIGN_OUTPUT_DIR)/campaign-result.json
WEBHOOK_GO_CAMPAIGN_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-go-campaign-ci-result.json
WEBHOOK_NUBLAR_WORKFLOW ?= nublar/workflows/webhook-validation.yaml
WEBHOOK_NUBLAR_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-nublar-result.json
WEBHOOK_NUBLAR_RUN_OUTPUT ?= $(ARTIFACT_ROOT)/webhook-validation-nublar-run.json
WEBHOOK_NUBLAR_RUN_STORE ?= $(ARTIFACT_ROOT)/webhook-validation-nublar-runs
SENTINEL_WORKSPACE ?= herdr-sentinel/workspaces/webhook-validation.yaml
SENTINEL_RUN_OUTPUT ?= $(ARTIFACT_ROOT)/sentinel-webhook-run.json
SENTINEL_CAPABILITY_OUTPUT ?= $(ARTIFACT_ROOT)/sentinel-webhook-capability-plan.json
SENTINEL_ORACLE_PROBE ?= examples/webhook-validation-lab/contract/contract.yaml
SENTINEL_VERIFIER_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/sentinel-webhook-verifier
SENTINEL_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/sentinel-webhook-ci-result.json
SENTINEL_NUBLAR_WORKFLOW ?= nublar/workflows/sentinel-webhook.yaml
SENTINEL_NUBLAR_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/sentinel-webhook-nublar-result.json
SENTINEL_NUBLAR_RUN_OUTPUT ?= $(ARTIFACT_ROOT)/sentinel-webhook-nublar-run.json
SENTINEL_NUBLAR_RUN_STORE ?= $(ARTIFACT_ROOT)/sentinel-webhook-nublar-runs
SENTINEL_FAILURE_SUBJECT_ADDR ?= $(WEBHOOK_DEFECT_ADDR)
SENTINEL_FAILURE_SUBJECT_URL ?= http://$(SENTINEL_FAILURE_SUBJECT_ADDR)
SENTINEL_FAILURE_SUBJECT_BINARY ?= $(WEBHOOK_DEFECT_BINARY)
SENTINEL_FAILURE_SUBJECT_VARIANT ?= webhook-duplicate-idempotency-defect
SANDBOX_ROOT ?= .
SANDBOX_PROBE_PATH ?= examples/document-pipeline-lab/contract/contract.yaml
ORACLE_OUTPUT_DIR ?= $(ARTIFACT_ROOT)/document-pipeline-oracle
REPLAY_BASE_URL ?=
REPLAY_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-ci-result.json
REPLAY_DEFECT_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-defect-ci-result.json
REPLAY_STATEFUL_DEFECT_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-stateful-defect-ci-result.json
REPLAY_PROCESS_DEFECT_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-process-defect-ci-result.json
REPLAY_PERSISTENCE_DEFECT_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-persistence-defect-ci-result.json
REPLAY_REMOVE_NAME_DEFECT_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-remove-name-defect-ci-result.json
REPLAY_ACCEPTS_PNG_DEFECT_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-accepts-png-defect-ci-result.json
REPLAY_MATRIX_CI_RESULT_OUTPUT ?= $(ARTIFACT_ROOT)/document-pipeline-replay-matrix-ci-result.json
REPLAY_MATRIX_MANIFEST ?= examples/document-pipeline-lab/replay/matrix.yaml
REPLAY_MATRIX_SOURCE_ROOT ?= .
REPLAY_FIXTURE_STARTUP_TIMEOUT ?= 10s
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
TYPESCRIPT_PROVIDER_DIR ?= sorna/providers/typescript
TYPESCRIPT_PROVIDER_PLAN ?= $(TYPESCRIPT_PROVIDER_DIR)/fixtures/plan.json
TYPESCRIPT_PROVIDER_OUTPUT ?= $(ARTIFACT_ROOT)/typescript-provider/provider.json
TYPESCRIPT_PROVIDER_REVIEW_OUTPUT ?= $(ARTIFACT_ROOT)/typescript-provider/provider-review-ci-result.json
TYPESCRIPT_PROVIDER_NUBLAR_OUTPUT ?= $(ARTIFACT_ROOT)/typescript-provider/nublar-result.json
TYPESCRIPT_PROVIDER_NUBLAR_STORE ?= $(ARTIFACT_ROOT)/typescript-provider/nublar-runs
TYPESCRIPT_PROVIDER_NUBLAR_RUN_ID ?= typescript-provider-run
TYPESCRIPT_PROVIDER_NUBLAR_RUN_OUTPUT ?= $(ARTIFACT_ROOT)/typescript-provider/nublar-run.json
TYPESCRIPT_PROVIDER_NUBLAR_SHOW_OUTPUT ?= $(ARTIFACT_ROOT)/typescript-provider/nublar-run-show.json
TYPESCRIPT_PROVIDER_NUBLAR_DECISION_OUTPUT ?= $(ARTIFACT_ROOT)/typescript-provider/nublar-decision.json
TYPESCRIPT_PROVIDER_ID ?= typescript-fixture-provider
TYPESCRIPT_PROVIDER_COMMAND ?= /bin/echo
TYPESCRIPT_CMD ?= /opt/homebrew/bin/bun
SORNA_RELEASE_WORKSPACE ?=

.DEFAULT_GOAL := help

.PHONY: malcolm-sorna-contract malcolm-sorna-flow-contract malcolm-sorna-seal malcolm-sorna-oracle-freeze malcolm-sorna-run malcolm-sorna-flow-seal malcolm-sorna-flow-oracle-freeze malcolm-sorna-flow-run malcolm-sorna-flow-event-defect-build malcolm-sorna-flow-event-defect-run malcolm-sorna-boundary-contract malcolm-sorna-boundary-seal malcolm-sorna-boundary-oracle-freeze malcolm-sorna-boundary-run malcolm-sorna-interpolation-contract malcolm-sorna-interpolation-seal malcolm-sorna-interpolation-oracle-freeze malcolm-sorna-interpolation-run malcolm-sorna-mutation-catalogue malcolm-sorna-provenance-contract malcolm-sorna-fixture-contract malcolm-sorna-fixture-handoff

.PHONY: help build test test-race vet check alpha-interface-check nublar-check nublar-consumer-check nublar-freeze-check \
	contract-validate contract-seal policy-validate subject-policy-validate subject-test subject-run subject-build sorna-build sorna-release-artifacts sorna-release-verify sorna-release-provenance defect-build sorna-run \
	sorna-external-run evidence-verify sorna-replay sorna-replay-ci-result sorna-replay-fresh sorna-replay-defect-fresh sorna-replay-stateful-defect-fresh sorna-replay-process-defect-fresh sorna-replay-persistence-defect-fresh sorna-replay-remove-name-defect-fresh sorna-replay-accepts-png-defect-fresh sorna-replay-regression sorna-replay-matrix-ci-result sorna-replay-matrix-verify sorna-replay-matrix-ci-result-fresh sorna-alpha-check sorna-release-check oracle-evidence-verify sorna-gate sorna-ci-result nublar-aggregate nublar-run-collect nublar-run-collect-fresh nublar-aggregate-fresh sorna-oracle-freeze \
	subject-defect-run sorna-defect-run mutation-catalogue-validate mutation-plan mutation-provider-validate mutation-provider-conformance mutation-typescript-provider-conformance mutation-provider-inspect mutation-provider-ci-result mutation-campaign-run mutation-campaign-verify mutation-campaign-ci-result mutation-go-provider-build mutation-go-provider-ci-result mutation-go-preparation-ci-result mutation-go-campaign-run mutation-go-campaign-verify mutation-go-campaign-ci-result mutation-go-campaign-ci-result-fresh mutation-go-survivor-run mutation-go-survivor-ci-result mutation-go-survivor-ci-result-fresh sandbox-contract-read defect-remove-name-build defect-unsupported-type-build defect-process-stays-queued-build defect-persistence-wrong-key-build defect-accepts-png-build webhook-contract-validate webhook-policy-validate webhook-subject-policy-validate webhook-subject-test webhook-subject-build webhook-oracle-freeze webhook-run webhook-ci-result webhook-alpha webhook-mutation-catalogue-validate webhook-defect-build webhook-mutation-plan webhook-mutation-provider-validate webhook-mutation-provider-inspect webhook-mutation-provider-ci-result webhook-mutation-run webhook-mutation-verify webhook-mutation-ci-result webhook-mutation-alpha webhook-go-provider-build webhook-go-provider-ci-result webhook-go-preparation-ci-result webhook-go-campaign-run webhook-go-campaign-verify webhook-go-campaign-ci-result webhook-go-mutation-alpha nublar-webhook-aggregate nublar-webhook-run-collect nublar-webhook-aggregate-fresh sentinel-workspace-validate sentinel-run-bootstrap sentinel-capability-plan sentinel-adapter-oracle-probe sentinel-adapter-verifier-probe sentinel-ci-result nublar-sentinel-aggregate nublar-sentinel-run-collect nublar-sentinel-run-collect-fresh nublar-sentinel-run-collect-external-root-fresh sentinel-adapter-verifier-failure-probe sentinel-ci-result-failure nublar-sentinel-run-collect-failure nublar-sentinel-run-collect-failure-fresh

.PHONY: nublar-sentinel-run-collect-external-root-failure-fresh sentinel-herdr-probe-fixture sentinel-herdr-contract-status-check sentinel-herdr-host-envelope-check

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

alpha-interface-check: ## Run tests and analysis for the current Sentinel/Sorna/Nublar alpha boundary
	$(GO_CMD) test ./core/... ./herdr-sentinel/... ./sorna/... ./examples/... ./nublar/...
	$(GO_CMD) vet ./core/... ./herdr-sentinel/... ./sorna/... ./examples/... ./nublar/...

nublar-check: ## Run Nublar tests, analysis, and schema syntax checks
	$(GO_CMD) test -race ./nublar/...
	$(GO_CMD) vet ./nublar/...
	jq empty core/ciresult-v1.schema.json nublar/spec/*.json

nublar-consumer-check: ## Verify the provider-neutral Nublar CI gate example
	bash nublar/examples/consumer/check.sh

nublar-freeze-check: nublar-check nublar-consumer-check ## Run the complete local Nublar contract freeze gate
	git diff --check

contract-validate: ## Validate the document-pipeline contract
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(CONTRACT)"

malcolm-sorna-contract: ## Compile the Malcolm healthcheck example into a Sorna contract and validate it
	mkdir -p "$(dir $(MALCOLM_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_EXAMPLE)" > "$(MALCOLM_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_IR_OUTPUT)" --output "$(MALCOLM_SORNA_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_SORNA_CONTRACT_OUTPUT)"

malcolm-sorna-flow-contract: ## Compile the Malcolm request-body/stateful example into a Sorna contract and validate it
	mkdir -p "$(dir $(MALCOLM_FLOW_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_FLOW_EXAMPLE)" > "$(MALCOLM_FLOW_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_FLOW_IR_OUTPUT)" --output "$(MALCOLM_FLOW_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_FLOW_CONTRACT_OUTPUT)"

malcolm-sorna-mutation-catalogue: ## Compile the Malcolm mutation example into a Sorna contract and validate its mutation catalogue
	mkdir -p "$(dir $(MALCOLM_MUTATION_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_MUTATION_EXAMPLE)" --output "$(MALCOLM_MUTATION_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_MUTATION_IR_OUTPUT)" --output "$(MALCOLM_MUTATION_CONTRACT_OUTPUT)" --mutations-output "$(MALCOLM_MUTATION_CATALOGUE_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_MUTATION_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation validate "$(MALCOLM_MUTATION_CATALOGUE_OUTPUT)" --contract "$(MALCOLM_MUTATION_CONTRACT_OUTPUT)"

malcolm-sorna-provenance-contract: ## Compile the Malcolm provenance example, validate its Sorna contract, and run the provenance boundary proof
	mkdir -p "$(dir $(MALCOLM_PROVENANCE_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_PROVENANCE_EXAMPLE)" --output "$(MALCOLM_PROVENANCE_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_PROVENANCE_IR_OUTPUT)" --output "$(MALCOLM_PROVENANCE_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_PROVENANCE_CONTRACT_OUTPUT)"
	$(GO_CMD) test ./sorna/internal/runner -run TestExecuteChecksDeclaredAmberExecutionID

malcolm-sorna-fixture-contract: ## Compile, validate, and seal the Malcolm fixture declaration example
	mkdir -p "$(dir $(MALCOLM_FIXTURE_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_FIXTURE_EXAMPLE)" --output "$(MALCOLM_FIXTURE_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_FIXTURE_IR_OUTPUT)" --output "$(MALCOLM_FIXTURE_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_FIXTURE_CONTRACT_OUTPUT)"
	mkdir -p "$(MALCOLM_FIXTURE_SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(MALCOLM_FIXTURE_CONTRACT_OUTPUT)" --output-dir "$(MALCOLM_FIXTURE_SEALED_DIR)"

malcolm-sorna-fixture-handoff: malcolm-sorna-fixture-contract ## Verify provider-supplied fixture files against the sealed Malcolm contract
	$(GO_CMD) run ./sorna/cmd/sorna fixture bind "$(MALCOLM_FIXTURE_SEALED_DIR)/canonical.json" --provider "$(MALCOLM_FIXTURE_PROVIDER)" --output "$(MALCOLM_FIXTURE_HANDOFF_OUTPUT)"
	jq -e '(.schema == "ingen.fixture-handoff/v1") and (.status == "verified") and (.fixtures | length == 1) and (.fixtures[0].id == "welcome-document") and (.fixtures[0].sha256 == "637cecb53db658da5231f933fad366fce94f982501e59d7ddc7688cfaf81b825")' "$(MALCOLM_FIXTURE_HANDOFF_OUTPUT)"

malcolm-sorna-flow-seal: malcolm-sorna-flow-contract ## Seal the Malcolm request-body/stateful contract
	mkdir -p "$(MALCOLM_FLOW_SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(MALCOLM_FLOW_CONTRACT_OUTPUT)" --output-dir "$(MALCOLM_FLOW_SEALED_DIR)"

malcolm-sorna-flow-oracle-freeze: malcolm-sorna-flow-seal ## Freeze an oracle from the Malcolm request-body/stateful contract
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(MALCOLM_FLOW_CONTRACT_OUTPUT)" --policy "$(MALCOLM_FLOW_ORACLE_POLICY)" --root . --output-dir "$(MALCOLM_FLOW_ORACLE_OUTPUT_DIR)"

malcolm-sorna-flow-run: malcolm-sorna-flow-oracle-freeze subject-build ## Run the Malcolm request-body/stateful/negative-setup/event oracle against the managed document subject
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(MALCOLM_FLOW_ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(MALCOLM_FLOW_ORACLE_POLICY)" --subject-policy "$(MALCOLM_FLOW_SUBJECT_POLICY)" --subject-root . --base-url "$(SUBJECT_URL)" --subject-command "$(SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant malcolm-flow --output-dir "$(MALCOLM_FLOW_RUN_OUTPUT_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(MALCOLM_FLOW_RUN_OUTPUT_DIR)"

malcolm-sorna-flow-event-defect-build: ## Build the controlled Malcolm event-removal defect subject
	mkdir -p "$(dir $(MALCOLM_FLOW_EVENT_DEFECT_BINARY))"
	$(GO_CMD) build -trimpath -o "$(MALCOLM_FLOW_EVENT_DEFECT_BINARY)" ./examples/document-pipeline-lab/defects/omit-accepted-event/cmd/document-pipeline-defect

malcolm-sorna-flow-event-defect-run: malcolm-sorna-flow-run malcolm-sorna-flow-event-defect-build ## Prove the Malcolm event assertion kills an event-removal mutation
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(MALCOLM_FLOW_ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(MALCOLM_FLOW_ORACLE_POLICY)" --subject-policy "$(MALCOLM_FLOW_SUBJECT_POLICY)" --subject-root . --baseline-evidence "$(MALCOLM_FLOW_RUN_OUTPUT_DIR)" --base-url "$(MALCOLM_FLOW_EVENT_DEFECT_URL)" --subject-command "$(MALCOLM_FLOW_EVENT_DEFECT_BINARY)" --subject-arg=-addr --subject-arg "$(MALCOLM_FLOW_EVENT_DEFECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant malcolm-flow-event-removed --mutation-id malcolm-flow-remove-accepted-event --mutation-plane implementation --mutation-description "remove the document.accepted event signal" --expected-rule create_document.requirement.3 --output-dir "$(MALCOLM_FLOW_EVENT_DEFECT_RUN_OUTPUT_DIR)"
	jq -e '(.mutation.outcome == "killed") and any(.rules[]; (.rule_id == "create_document.requirement.3") and (.status == "fail") and any(.assertions[]; (.path == "events.document.accepted") and (.status == "fail"))) and any(.rules[]; (.rule_id == "create_document.requirement.5") and (.status == "fail") and any(.assertions[]; (.path == "events.order") and (.status == "fail")))' "$(MALCOLM_FLOW_EVENT_DEFECT_RUN_OUTPUT_DIR)/run.json"
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(MALCOLM_FLOW_EVENT_DEFECT_RUN_OUTPUT_DIR)"

malcolm-sorna-boundary-contract: ## Compile the Malcolm generated-body example into a Sorna contract and validate it
	mkdir -p "$(dir $(MALCOLM_BOUNDARY_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_BOUNDARY_EXAMPLE)" > "$(MALCOLM_BOUNDARY_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_BOUNDARY_IR_OUTPUT)" --output "$(MALCOLM_BOUNDARY_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_BOUNDARY_CONTRACT_OUTPUT)"

malcolm-sorna-boundary-seal: malcolm-sorna-boundary-contract ## Seal the Malcolm generated-body contract
	mkdir -p "$(MALCOLM_BOUNDARY_SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(MALCOLM_BOUNDARY_CONTRACT_OUTPUT)" --output-dir "$(MALCOLM_BOUNDARY_SEALED_DIR)"

malcolm-sorna-boundary-oracle-freeze: malcolm-sorna-boundary-seal ## Freeze the generated-body oracle and verify materialization
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(MALCOLM_BOUNDARY_CONTRACT_OUTPUT)" --policy "$(MALCOLM_BOUNDARY_ORACLE_POLICY)" --root . --output-dir "$(MALCOLM_BOUNDARY_ORACLE_OUTPUT_DIR)"
	jq -e '(.cases | length == 2) and (.cases[0].given.body.content | (type == "string" and length == 4097)) and (.cases[1].given.body.content | (type == "string" and length == 4097))' "$(MALCOLM_BOUNDARY_ORACLE_OUTPUT_DIR)/oracle.json"

malcolm-sorna-boundary-run: malcolm-sorna-boundary-oracle-freeze subject-build ## Run the generated-body oracle against the managed document subject
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(MALCOLM_BOUNDARY_ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(MALCOLM_BOUNDARY_ORACLE_POLICY)" --subject-policy "$(MALCOLM_BOUNDARY_SUBJECT_POLICY)" --subject-root . --base-url "$(SUBJECT_URL)" --subject-command "$(SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant malcolm-boundary --output-dir "$(MALCOLM_BOUNDARY_RUN_OUTPUT_DIR)"
	jq -e 'all(.rules[]; .status == "pass")' "$(MALCOLM_BOUNDARY_RUN_OUTPUT_DIR)/run.json"
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(MALCOLM_BOUNDARY_RUN_OUTPUT_DIR)"

malcolm-sorna-interpolation-contract: ## Compile the Malcolm capture-interpolation example into a Sorna contract and validate it
	mkdir -p "$(dir $(MALCOLM_INTERPOLATION_IR_OUTPUT))"
	cargo run --manifest-path malcolm/Cargo.toml -- "$(MALCOLM_INTERPOLATION_EXAMPLE)" > "$(MALCOLM_INTERPOLATION_IR_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna-malcolm "$(MALCOLM_INTERPOLATION_IR_OUTPUT)" --output "$(MALCOLM_INTERPOLATION_CONTRACT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(MALCOLM_INTERPOLATION_CONTRACT_OUTPUT)"

malcolm-sorna-interpolation-seal: malcolm-sorna-interpolation-contract ## Seal the Malcolm capture-interpolation contract
	mkdir -p "$(MALCOLM_INTERPOLATION_SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(MALCOLM_INTERPOLATION_CONTRACT_OUTPUT)" --output-dir "$(MALCOLM_INTERPOLATION_SEALED_DIR)"

malcolm-sorna-interpolation-oracle-freeze: malcolm-sorna-interpolation-seal ## Freeze the capture-interpolation oracle
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(MALCOLM_INTERPOLATION_CONTRACT_OUTPUT)" --policy "$(MALCOLM_INTERPOLATION_ORACLE_POLICY)" --root . --output-dir "$(MALCOLM_INTERPOLATION_ORACLE_OUTPUT_DIR)"
	jq -e '(.cases | length == 2) and (.cases[0].given.body.name == "{document_name}") and (.cases[1].given.body.name == "{document_name}")' "$(MALCOLM_INTERPOLATION_ORACLE_OUTPUT_DIR)/oracle.json"

malcolm-sorna-interpolation-run: malcolm-sorna-interpolation-oracle-freeze subject-build ## Run the capture-interpolation oracle against the managed document subject
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(MALCOLM_INTERPOLATION_ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(MALCOLM_INTERPOLATION_ORACLE_POLICY)" --subject-policy "$(MALCOLM_INTERPOLATION_SUBJECT_POLICY)" --subject-root . --base-url "$(SUBJECT_URL)" --subject-command "$(SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant malcolm-interpolation --output-dir "$(MALCOLM_INTERPOLATION_RUN_OUTPUT_DIR)"
	jq -e 'all(.rules[]; .status == "pass")' "$(MALCOLM_INTERPOLATION_RUN_OUTPUT_DIR)/run.json"
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(MALCOLM_INTERPOLATION_RUN_OUTPUT_DIR)"

malcolm-sorna-seal: malcolm-sorna-contract ## Seal the Malcolm-generated Sorna contract
	mkdir -p "$(MALCOLM_SORNA_SEALED_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna contract seal "$(MALCOLM_SORNA_CONTRACT_OUTPUT)" --output-dir "$(MALCOLM_SORNA_SEALED_DIR)"

malcolm-sorna-oracle-freeze: malcolm-sorna-seal ## Freeze a Sorna oracle from the Malcolm-generated contract
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(MALCOLM_SORNA_CONTRACT_OUTPUT)" --policy "$(MALCOLM_ORACLE_POLICY)" --root . --output-dir "$(MALCOLM_SORNA_ORACLE_OUTPUT_DIR)"

malcolm-sorna-run: malcolm-sorna-oracle-freeze subject-build ## Run the frozen Malcolm oracle against the managed document subject
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(MALCOLM_SORNA_ORACLE_OUTPUT_DIR)/oracle.json" --policy "$(MALCOLM_ORACLE_POLICY)" --subject-policy "$(MALCOLM_SUBJECT_POLICY)" --subject-root . --base-url "$(SUBJECT_URL)" --subject-command "$(SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --ready-path "$(SUBJECT_READY_PATH)" --subject-variant malcolm-healthz --output-dir "$(MALCOLM_SORNA_RUN_OUTPUT_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna evidence verify "$(MALCOLM_SORNA_RUN_OUTPUT_DIR)"

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

sorna-build: ## Build the standalone Sorna CLI with release metadata
	mkdir -p "$(SORNA_BINARY_DIR)"
	$(GO_CMD) build -trimpath -ldflags "$(SORNA_LDFLAGS)" -o "$(SORNA_BINARY)" ./sorna/cmd/sorna

sorna-release-artifacts: sorna-release-check ## Build cross-platform Sorna archives and a checksum manifest
	GOCACHE="$(GO_CACHE)" GOMODCACHE="$(GO_MOD_CACHE)" \
	SORNA_VERSION="$(SORNA_VERSION)" SORNA_COMMIT="$(SORNA_COMMIT)" SORNA_BUILD_DATE="$(SORNA_BUILD_DATE)" \
	SORNA_RELEASE_DIR="$(SORNA_RELEASE_DIR)" SORNA_RELEASE_TARGETS="$(SORNA_RELEASE_TARGETS)" \
	sh ./sorna/release.sh

sorna-release-verify: sorna-build ## Verify Sorna release archives against their manifest
	"$(SORNA_BINARY)" release verify --manifest "$(SORNA_RELEASE_MANIFEST)" --directory "$(SORNA_RELEASE_DIR)" --format json

sorna-release-provenance: sorna-build ## Verify a Sorna release and emit signing-ready provenance
	"$(SORNA_BINARY)" release verify --manifest "$(SORNA_RELEASE_MANIFEST)" --directory "$(SORNA_RELEASE_DIR)" --format json > "$(SORNA_RELEASE_VERIFICATION)"
	"$(SORNA_BINARY)" release provenance create --manifest "$(SORNA_RELEASE_MANIFEST)" --verification "$(SORNA_RELEASE_VERIFICATION)" --repository "$(SORNA_RELEASE_REPOSITORY)" --ref "$(SORNA_RELEASE_REF)" --tag "$(SORNA_RELEASE_TAG)" --commit "$(SORNA_COMMIT)" --workflow "$(SORNA_RELEASE_WORKFLOW)" --run-id "$(SORNA_RELEASE_RUN_ID)" --run-attempt "$(SORNA_RELEASE_RUN_ATTEMPT)" --runner "$(SORNA_RELEASE_RUNNER)" --build-date "$(SORNA_BUILD_DATE)" --output "$(SORNA_RELEASE_PROVENANCE)"

webhook-contract-validate: ## Validate the webhook-validation contract
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$(WEBHOOK_CONTRACT)"

webhook-policy-validate: ## Validate the webhook oracle policy
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$(WEBHOOK_POLICY)"

webhook-subject-policy-validate: ## Validate the managed webhook subject policy
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$(WEBHOOK_SUBJECT_POLICY)"

webhook-subject-test: ## Test the webhook-validation subject only
	$(GO_CMD) test ./examples/webhook-validation-lab/subject/...

webhook-subject-build: ## Build the clean webhook-validation subject binary
	mkdir -p "$(WEBHOOK_SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -trimpath -o "$(WEBHOOK_SUBJECT_BINARY)" ./examples/webhook-validation-lab/subject/cmd/webhook-validation

webhook-oracle-freeze: webhook-contract-validate webhook-policy-validate ## Freeze the webhook-validation oracle
	mkdir -p "$(WEBHOOK_ORACLE_OUTPUT_DIR)"
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "$(WEBHOOK_CONTRACT)" --policy "$(WEBHOOK_POLICY)" --root . --output-dir "$(WEBHOOK_ORACLE_OUTPUT_DIR)"

webhook-run: webhook-oracle-freeze webhook-subject-policy-validate webhook-subject-build ## Run the managed webhook-validation subject against its frozen oracle
	$(GO_CMD) run ./sorna/cmd/sorna run --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --policy "$(WEBHOOK_POLICY)" --subject-policy "$(WEBHOOK_SUBJECT_POLICY)" --subject-root . --base-url "$(WEBHOOK_SUBJECT_URL)" --subject-command "$(WEBHOOK_SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(WEBHOOK_SUBJECT_ADDR)" --ready-path /healthz --subject-variant clean-baseline --output-dir "$(WEBHOOK_RUN_OUTPUT_DIR)"

webhook-ci-result: webhook-run ## Run the webhook-validation baseline and write a CI result envelope
	mkdir -p "$(dir $(WEBHOOK_CI_RESULT_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna gate --format ci-result "$(WEBHOOK_RUN_OUTPUT_DIR)" > "$(WEBHOOK_CI_RESULT_OUTPUT)"

webhook-alpha: webhook-contract-validate webhook-policy-validate webhook-subject-policy-validate webhook-subject-test webhook-ci-result ## Validate and run the first webhook-validation Sorna slice

webhook-mutation-catalogue-validate: ## Validate the webhook-validation mutation catalogue
	$(GO_CMD) run ./sorna/cmd/sorna mutation validate "$(WEBHOOK_MUTATION_CATALOGUE)" --contract "$(WEBHOOK_CONTRACT)"

webhook-defect-build: ## Build the controlled webhook duplicate-idempotency defect binary
	mkdir -p "$(WEBHOOK_SUBJECT_BINARY_DIR)"
	$(GO_CMD) build -trimpath -o "$(WEBHOOK_DEFECT_BINARY)" ./examples/webhook-validation-lab/defects/accepts-duplicate/cmd/webhook-validation-defect

webhook-mutation-plan: webhook-run webhook-mutation-catalogue-validate ## Build a ready webhook mutation campaign plan
	mkdir -p "$(dir $(WEBHOOK_MUTATION_PLAN_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna mutation plan "$(WEBHOOK_MUTATION_CATALOGUE)" --contract "$(WEBHOOK_CONTRACT)" --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --baseline-evidence "$(WEBHOOK_RUN_OUTPUT_DIR)" --subject-policy "$(WEBHOOK_SUBJECT_POLICY)" --output "$(WEBHOOK_MUTATION_PLAN_OUTPUT)"

webhook-mutation-provider-validate: ## Validate the webhook-validation mutation provider manifest
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider validate "$(WEBHOOK_MUTATION_PROVIDER)"

webhook-mutation-provider-inspect: webhook-mutation-plan webhook-mutation-provider-validate ## Review webhook fixture capabilities against the ready mutation plan
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_MUTATION_PROVIDER)" $(WEBHOOK_MUTATION_PROVIDER_BINDING_FLAG) --format json --output "$(WEBHOOK_MUTATION_PROVIDER_REVIEW_OUTPUT)"

webhook-mutation-provider-ci-result: webhook-mutation-plan webhook-mutation-provider-validate ## Write the webhook provider preflight as a shared CI result
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_MUTATION_PROVIDER)" $(WEBHOOK_MUTATION_PROVIDER_BINDING_FLAG) --format ci-result --output "$(WEBHOOK_MUTATION_PROVIDER_CI_RESULT_OUTPUT)"

webhook-mutation-run: webhook-mutation-plan webhook-defect-build webhook-mutation-provider-validate ## Execute the webhook duplicate-idempotency mutation
	$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_MUTATION_PROVIDER)" $(WEBHOOK_MUTATION_PROVIDER_BINDING_FLAG) --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --policy "$(WEBHOOK_POLICY)" --subject-policy "$(WEBHOOK_SUBJECT_POLICY)" --base-address "$(WEBHOOK_DEFECT_ADDR)" --output-dir "$(WEBHOOK_MUTATION_OUTPUT_DIR)" --output "$(WEBHOOK_MUTATION_RESULT_OUTPUT)"

webhook-mutation-verify: ## Verify webhook mutation evidence and result bindings
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(WEBHOOK_MUTATION_RESULT_OUTPUT)"

webhook-mutation-ci-result: webhook-mutation-provider-ci-result webhook-defect-build ## Verify the webhook campaign and write a shared CI result
	-$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_MUTATION_PROVIDER)" $(WEBHOOK_MUTATION_PROVIDER_BINDING_FLAG) --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --policy "$(WEBHOOK_POLICY)" --subject-policy "$(WEBHOOK_SUBJECT_POLICY)" --base-address "$(WEBHOOK_DEFECT_ADDR)" --output-dir "$(WEBHOOK_MUTATION_OUTPUT_DIR)" --output "$(WEBHOOK_MUTATION_RESULT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(WEBHOOK_MUTATION_RESULT_OUTPUT)" --format ci-result --source-root . --output "$(WEBHOOK_MUTATION_CI_RESULT_OUTPUT)"

webhook-mutation-alpha: webhook-mutation-ci-result ## Run the first webhook mutation campaign slice

webhook-go-provider-build: webhook-mutation-plan ## Prepare the webhook mutation through the reusable Go source provider
	$(GO_CMD) run ./sorna/cmd/sorna-go-provider --subject webhook-validation --plan "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --source-root . --output-dir "$(WEBHOOK_GO_PROVIDER_OUTPUT_DIR)" --binary-dir "$(WEBHOOK_GO_PROVIDER_BINARY_DIR)" --provider "$(WEBHOOK_GO_PROVIDER)" --summary-output "$(WEBHOOK_GO_PROVIDER_SUMMARY)"

webhook-go-provider-ci-result: webhook-go-provider-build ## Review the webhook Go provider with mandatory exact plan binding
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_GO_PROVIDER)" --require-plan-binding --format ci-result --output "$(WEBHOOK_GO_PROVIDER_CI_RESULT_OUTPUT)"

webhook-go-preparation-ci-result: webhook-go-provider-build ## Expose webhook Go-provider preparation as a shared CI result
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider preparation "$(WEBHOOK_GO_PROVIDER_SUMMARY)" --provider "$(WEBHOOK_GO_PROVIDER)" --source-root . --format ci-result --output "$(WEBHOOK_GO_PREPARATION_CI_RESULT_OUTPUT)"

webhook-go-campaign-run: webhook-go-provider-build ## Execute the webhook campaign using the source-level Go provider
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider validate "$(WEBHOOK_GO_PROVIDER)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_GO_PROVIDER)" --require-plan-binding --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --policy "$(WEBHOOK_POLICY)" --subject-policy "$(WEBHOOK_SUBJECT_POLICY)" --base-address "$(WEBHOOK_DEFECT_ADDR)" --output-dir "$(WEBHOOK_GO_CAMPAIGN_OUTPUT_DIR)" --output "$(WEBHOOK_GO_CAMPAIGN_RESULT_OUTPUT)"

webhook-go-campaign-verify: ## Verify the webhook source-provider campaign result and evidence bindings
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(WEBHOOK_GO_CAMPAIGN_RESULT_OUTPUT)"

webhook-go-campaign-ci-result: webhook-go-provider-ci-result webhook-go-preparation-ci-result ## Verify the strict webhook Go campaign and write a shared CI result
	-$(GO_CMD) run ./sorna/cmd/sorna mutation run "$(WEBHOOK_MUTATION_PLAN_OUTPUT)" --provider "$(WEBHOOK_GO_PROVIDER)" --require-plan-binding --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --policy "$(WEBHOOK_POLICY)" --subject-policy "$(WEBHOOK_SUBJECT_POLICY)" --base-address "$(WEBHOOK_DEFECT_ADDR)" --output-dir "$(WEBHOOK_GO_CAMPAIGN_OUTPUT_DIR)" --output "$(WEBHOOK_GO_CAMPAIGN_RESULT_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation verify "$(WEBHOOK_GO_CAMPAIGN_RESULT_OUTPUT)" --format ci-result --source-root . --output "$(WEBHOOK_GO_CAMPAIGN_CI_RESULT_OUTPUT)"

webhook-go-mutation-alpha: webhook-go-campaign-ci-result ## Run the first webhook source-provider mutation slice

nublar-webhook-aggregate: webhook-ci-result webhook-go-provider-ci-result webhook-go-preparation-ci-result webhook-go-campaign-ci-result ## Aggregate the webhook Sorna envelopes through Nublar
	mkdir -p "$(dir $(WEBHOOK_NUBLAR_RESULT_OUTPUT))"
	$(GO_CMD) run ./nublar/cmd/nublar aggregate --workflow "$(WEBHOOK_NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --output "$(WEBHOOK_NUBLAR_RESULT_OUTPUT)"

nublar-webhook-run-collect: webhook-ci-result webhook-go-provider-ci-result webhook-go-preparation-ci-result webhook-go-campaign-ci-result ## Collect the webhook workflow as a durable Nublar run
	mkdir -p "$(dir $(WEBHOOK_NUBLAR_RUN_OUTPUT))" "$(WEBHOOK_NUBLAR_RUN_STORE)"
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "$(WEBHOOK_NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --store "$(WEBHOOK_NUBLAR_RUN_STORE)" --output "$(WEBHOOK_NUBLAR_RUN_OUTPUT)"

nublar-webhook-aggregate-fresh: ## Run the complete webhook workflow in a fresh temporary workspace
	workspace=$$(mktemp -d /private/tmp/ingen-webhook-workspace.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\n" "$$workspace" "$$workspace/.artifacts"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" nublar-webhook-aggregate; status=$$?; \
	exit $$status

sentinel-workspace-validate: ## Validate the Sentinel webhook contract workspace manifest
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel workspace validate "$(SENTINEL_WORKSPACE)"

sentinel-run-bootstrap: ## Create a Sentinel lifecycle receipt for the webhook workspace
	mkdir -p "$(dir $(SENTINEL_RUN_OUTPUT))"
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run bootstrap --workspace "$(SENTINEL_WORKSPACE)" --output "$(SENTINEL_RUN_OUTPUT)"

sentinel-capability-plan: ## Compile the Sentinel webhook capability handoff
	mkdir -p "$(dir $(SENTINEL_CAPABILITY_OUTPUT))"
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel workspace capabilities --workspace "$(SENTINEL_WORKSPACE)" --output "$(SENTINEL_CAPABILITY_OUTPUT)"

sentinel-herdr-probe-fixture: ## Exercise and machine-check the Herdr 0.9.0 compatibility probe
	@probe_state="$$(mktemp -d /private/tmp/ingen-herdr-probe-state.XXXXXX)"; \
	HERDR_PLUGIN_STATE_DIR="$$probe_state" \
	HERDR_PLUGIN_EVENT="pane.agent_status_changed" \
	HERDR_WORKSPACE_ID="w1" \
	HERDR_TAB_ID="w1:t1" \
	HERDR_PANE_ID="w1:p1" \
	HERDR_PLUGIN_CONTEXT_JSON='{"workspace_id":"w1","pane_id":"w1:p1"}' \
	HERDR_PLUGIN_EVENT_JSON='{"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed","workspace_id":"w1","pane_id":"w1:p1","agent_status":"working"}}' \
	sh herdr-sentinel/plugin/record-event.sh; \
	sh herdr-sentinel/plugin/verify-captures.sh "$$probe_state"; \
	printf 'probe_state=%s\n' "$$probe_state"

sentinel-herdr-contract-status-check: ## Validate the Herdr host-contract status and sanitized live fixture
	jq -e '.schema == "ingen.herdr-host-contract/v1" and .binding_status == "blocked" and .host.version == "0.9.0" and .observed.envelope_fields == ["event", "data"] and .required_for_sentinel.event_id.status == "missing" and .required_for_sentinel.event_timestamp.status == "missing"' herdr-sentinel/plugin/host-contract-status.json >/dev/null
	sh herdr-sentinel/plugin/verify-captures.sh herdr-sentinel/plugin/fixtures/live-pane-agent-status-changed

sentinel-herdr-host-envelope-check: ## Inspect the sanitized raw Herdr envelope without normalizing it
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel adapter herdr-host-envelope --event herdr-sentinel/plugin/fixtures/live-pane-agent-status-changed/capture.live-pane-agent-status-changed/event.json

sentinel-adapter-oracle-probe: sentinel-run-bootstrap ## Delegate a contract-read probe through Sorna's host adapter
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel adapter oracle --workspace "$(SENTINEL_WORKSPACE)" --root . --receipt "$(SENTINEL_RUN_OUTPUT)" -- /bin/cat "$(SENTINEL_ORACLE_PROBE)"

sentinel-adapter-verifier-probe: sentinel-run-bootstrap webhook-oracle-freeze webhook-subject-build ## Run the managed webhook subject through Sentinel's verifier handoff
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel adapter verifier --workspace "$(SENTINEL_WORKSPACE)" --root . --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --base-url "$(WEBHOOK_SUBJECT_URL)" --subject-command "$(WEBHOOK_SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(WEBHOOK_SUBJECT_ADDR)" --ready-path /healthz --subject-variant clean-baseline --output-dir "$(SENTINEL_VERIFIER_OUTPUT_DIR)" --receipt "$(SENTINEL_RUN_OUTPUT)"

sentinel-ci-result: sentinel-adapter-verifier-probe ## Adapt the Sentinel verifier receipt to the shared CI envelope
	mkdir -p "$(dir $(SENTINEL_CI_RESULT_OUTPUT))"
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run ci-result --receipt "$(SENTINEL_RUN_OUTPUT)" --source-root . --output "$(SENTINEL_CI_RESULT_OUTPUT)"

nublar-sentinel-aggregate: sentinel-ci-result ## Aggregate the Sentinel verifier envelope through Nublar
	mkdir -p "$(dir $(SENTINEL_NUBLAR_RESULT_OUTPUT))"
	$(GO_CMD) run ./nublar/cmd/nublar aggregate --workflow "$(SENTINEL_NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --output "$(SENTINEL_NUBLAR_RESULT_OUTPUT)"

nublar-sentinel-run-collect: sentinel-ci-result ## Collect the Sentinel verifier envelope as a durable Nublar run
	mkdir -p "$(dir $(SENTINEL_NUBLAR_RUN_OUTPUT))" "$(SENTINEL_NUBLAR_RUN_STORE)"
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "$(SENTINEL_NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --store "$(SENTINEL_NUBLAR_RUN_STORE)" --output "$(SENTINEL_NUBLAR_RUN_OUTPUT)"

sentinel-adapter-verifier-failure-probe: sentinel-run-bootstrap webhook-oracle-freeze webhook-defect-build ## Run the controlled webhook defect through Sentinel's verifier handoff
	-$(GO_CMD) run ./herdr-sentinel/cmd/sentinel adapter verifier --workspace "$(SENTINEL_WORKSPACE)" --root . --oracle "$(WEBHOOK_ORACLE_OUTPUT)" --base-url "$(SENTINEL_FAILURE_SUBJECT_URL)" --subject-command "$(SENTINEL_FAILURE_SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SENTINEL_FAILURE_SUBJECT_ADDR)" --ready-path /healthz --subject-variant "$(SENTINEL_FAILURE_SUBJECT_VARIANT)" --output-dir "$(SENTINEL_VERIFIER_OUTPUT_DIR)" --receipt "$(SENTINEL_RUN_OUTPUT)"

sentinel-ci-result-failure: sentinel-adapter-verifier-failure-probe ## Emit Sentinel's expected failed envelope after a verifier failure
	mkdir -p "$(dir $(SENTINEL_CI_RESULT_OUTPUT))"
	-$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run ci-result --receipt "$(SENTINEL_RUN_OUTPUT)" --source-root . --output "$(SENTINEL_CI_RESULT_OUTPUT)"

nublar-sentinel-run-collect-failure: sentinel-ci-result-failure ## Collect Sentinel's expected failed envelope and assert Nublar records failure
	mkdir -p "$(dir $(SENTINEL_NUBLAR_RUN_OUTPUT))" "$(SENTINEL_NUBLAR_RUN_STORE)"
	set +e; \
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "$(SENTINEL_NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --store "$(SENTINEL_NUBLAR_RUN_STORE)" --output "$(SENTINEL_NUBLAR_RUN_OUTPUT)"; status=$$?; \
	if [ "$$status" -ne 1 ]; then printf 'expected Nublar failed decision (exit 1), got %s\n' "$$status" >&2; exit "$$status"; fi; \
	printf 'expected failed Nublar decision: %s\n' "$(SENTINEL_NUBLAR_RUN_OUTPUT)"

nublar-sentinel-run-collect-failure-fresh: ## Run the Sentinel failure proof in a fresh temporary workspace
	workspace=$$(mktemp -d /private/tmp/ingen-sentinel-failure-workspace.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\nrun store: %s\n" "$$workspace" "$$workspace/.artifacts" "$$workspace/.artifacts/sentinel-webhook-nublar-runs"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" ARTIFACT_ROOT=.artifacts GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" nublar-sentinel-run-collect-failure; status=$$?; \
	exit $$status

nublar-sentinel-run-collect-fresh: ## Run the Sentinel verifier workflow in a fresh temporary workspace
	workspace=$$(mktemp -d /private/tmp/ingen-sentinel-workspace.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\nrun store: %s\n" "$$workspace" "$$workspace/.artifacts" "$$workspace/.artifacts/sentinel-webhook-nublar-runs"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" ARTIFACT_ROOT=.artifacts GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" nublar-sentinel-run-collect; status=$$?; \
	exit $$status

nublar-sentinel-run-collect-external-root-fresh: ## Run the Sentinel verifier workflow from outside its project root
	workspace=$$(mktemp -d /private/tmp/ingen-sentinel-external-root.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\nrun store: %s\n" "$$workspace" "$$workspace/.artifacts" "$$workspace/.artifacts/sentinel-webhook-nublar-runs"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	mkdir -p "$$workspace/.artifacts" && \
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run bootstrap --workspace "herdr-sentinel/workspaces/webhook-validation.yaml" --root "$$workspace" --output "$$workspace/.artifacts/sentinel-webhook-run.json" && \
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$$workspace/examples/webhook-validation-lab/contract/contract.yaml" && \
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$$workspace/examples/webhook-validation-lab/policy/isolation.yaml" && \
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "examples/webhook-validation-lab/contract/contract.yaml" --policy "examples/webhook-validation-lab/policy/isolation.yaml" --root "$$workspace" --output-dir ".artifacts/webhook-validation-oracle" && \
	$(GO_CMD) build -trimpath -o "$$workspace/.artifacts/webhook-validation-subject/webhook-validation" ./examples/webhook-validation-lab/subject/cmd/webhook-validation && \
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel adapter verifier --workspace "herdr-sentinel/workspaces/webhook-validation.yaml" --root "$$workspace" --oracle ".artifacts/webhook-validation-oracle/oracle.json" --base-url "$(WEBHOOK_SUBJECT_URL)" --subject-command ".artifacts/webhook-validation-subject/webhook-validation" --subject-arg=-addr --subject-arg "$(WEBHOOK_SUBJECT_ADDR)" --ready-path /healthz --subject-variant clean-baseline --output-dir ".artifacts/sentinel-webhook-verifier" --receipt "$$workspace/.artifacts/sentinel-webhook-run.json" && \
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run ci-result --receipt "$$workspace/.artifacts/sentinel-webhook-run.json" --source-root "$$workspace" --output "$$workspace/.artifacts/sentinel-webhook-ci-result.json" && \
	mkdir -p "$$workspace/.artifacts/sentinel-webhook-nublar-runs" && \
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "nublar/workflows/sentinel-webhook.yaml" --root "$$workspace/.artifacts" --store "$$workspace/.artifacts/sentinel-webhook-nublar-runs" --output "$$workspace/.artifacts/sentinel-webhook-nublar-run.json"; status=$$?; \
	exit $$status

nublar-sentinel-run-collect-external-root-failure-fresh: ## Run the Sentinel failure proof from outside its project root
	workspace=$$(mktemp -d /private/tmp/ingen-sentinel-external-failure-root.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\nrun store: %s\n" "$$workspace" "$$workspace/.artifacts" "$$workspace/.artifacts/sentinel-webhook-nublar-runs"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	mkdir -p "$$workspace/.artifacts" && \
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run bootstrap --workspace "herdr-sentinel/workspaces/webhook-validation.yaml" --root "$$workspace" --output "$$workspace/.artifacts/sentinel-webhook-run.json" && \
	$(GO_CMD) run ./sorna/cmd/sorna contract validate "$$workspace/examples/webhook-validation-lab/contract/contract.yaml" && \
	$(GO_CMD) run ./sorna/cmd/sorna policy validate "$$workspace/examples/webhook-validation-lab/policy/isolation.yaml" && \
	$(GO_CMD) run ./sorna/cmd/sorna oracle freeze --contract "examples/webhook-validation-lab/contract/contract.yaml" --policy "examples/webhook-validation-lab/policy/isolation.yaml" --root "$$workspace" --output-dir ".artifacts/webhook-validation-oracle" && \
	$(GO_CMD) build -trimpath -o "$$workspace/.artifacts/webhook-validation-subject/webhook-validation-defect" ./examples/webhook-validation-lab/defects/accepts-duplicate/cmd/webhook-validation-defect && \
	set +e; \
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel adapter verifier --workspace "herdr-sentinel/workspaces/webhook-validation.yaml" --root "$$workspace" --oracle ".artifacts/webhook-validation-oracle/oracle.json" --base-url "$(SENTINEL_FAILURE_SUBJECT_URL)" --subject-command ".artifacts/webhook-validation-subject/webhook-validation-defect" --subject-arg=-addr --subject-arg "$(SENTINEL_FAILURE_SUBJECT_ADDR)" --ready-path /healthz --subject-variant "$(SENTINEL_FAILURE_SUBJECT_VARIANT)" --output-dir ".artifacts/sentinel-webhook-verifier" --receipt "$$workspace/.artifacts/sentinel-webhook-run.json"; verifier_status=$$?; \
	if [ "$$verifier_status" -ne 1 ]; then printf 'expected Sentinel verifier failure (exit 1), got %s\n' "$$verifier_status" >&2; exit "$$verifier_status"; fi; \
	$(GO_CMD) run ./herdr-sentinel/cmd/sentinel run ci-result --receipt "$$workspace/.artifacts/sentinel-webhook-run.json" --source-root "$$workspace" --output "$$workspace/.artifacts/sentinel-webhook-ci-result.json"; ci_status=$$?; \
	if [ "$$ci_status" -ne 1 ]; then printf 'expected Sentinel CI-result failure (exit 1), got %s\n' "$$ci_status" >&2; exit "$$ci_status"; fi; \
	mkdir -p "$$workspace/.artifacts/sentinel-webhook-nublar-runs" && \
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "nublar/workflows/sentinel-webhook.yaml" --root "$$workspace/.artifacts" --store "$$workspace/.artifacts/sentinel-webhook-nublar-runs" --output "$$workspace/.artifacts/sentinel-webhook-nublar-run.json"; nublar_status=$$?; \
	if [ "$$nublar_status" -ne 1 ]; then printf 'expected Nublar failed decision (exit 1), got %s\n' "$$nublar_status" >&2; exit "$$nublar_status"; fi; \
	printf 'expected failed external-root Nublar decision: %s\n' "$$workspace/.artifacts/sentinel-webhook-nublar-run.json"

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

sorna-replay: ## Replay RUN_OUTPUT_DIR against an explicitly supplied equivalent subject; set REPLAY_BASE_URL
	$(GO_CMD) run ./sorna/cmd/sorna evidence replay --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --base-url "$(REPLAY_BASE_URL)" "$(RUN_OUTPUT_DIR)"

sorna-replay-ci-result: ## Replay and write the shared CI result; set REPLAY_BASE_URL
	mkdir -p "$(dir $(REPLAY_CI_RESULT_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna evidence replay --format ci-result --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --base-url "$(REPLAY_BASE_URL)" --output "$(REPLAY_CI_RESULT_OUTPUT)" "$(RUN_OUTPUT_DIR)"

sorna-replay-fresh: sorna-run subject-build ## Create a clean baseline, start a fresh subject separately, and replay it
	$(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(SUBJECT_BINARY)" --subject-arg=-addr --subject-arg "$(SUBJECT_ADDR)" --base-url "$(SUBJECT_URL)" --ready-path "$(SUBJECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_CI_RESULT_OUTPUT)"

sorna-replay-defect-fresh: sorna-run defect-unsupported-type-build ## Confirm a known defect becomes a failed replay CI result
	mkdir -p "$(dir $(REPLAY_DEFECT_CI_RESULT_OUTPUT))"
	status=0; $(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(DEFECT_UNSUPPORTED_TYPE_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --base-url "$(DEFECT_URL)" --ready-path "$(DEFECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_DEFECT_CI_RESULT_OUTPUT)" || status=$$?; test "$$status" -eq 1
	jq -e '.status == "failed" and .report.status == "drifted" and (.report.differences | length) > 0' "$(REPLAY_DEFECT_CI_RESULT_OUTPUT)" >/dev/null

sorna-replay-stateful-defect-fresh: sorna-run defect-build ## Confirm an incomplete stateful replay remains an explicit CI error
	mkdir -p "$(dir $(REPLAY_STATEFUL_DEFECT_CI_RESULT_OUTPUT))"
	status=0; $(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(DEFECT_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --base-url "$(DEFECT_URL)" --ready-path "$(DEFECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_STATEFUL_DEFECT_CI_RESULT_OUTPUT)" || status=$$?; test "$$status" -eq 1
	jq -e '.status == "error" and .report.status == "inconclusive" and .report.replay_verdict.status == "fail" and (.report.differences | length) > 0' "$(REPLAY_STATEFUL_DEFECT_CI_RESULT_OUTPUT)" >/dev/null

sorna-replay-process-defect-fresh: sorna-run defect-process-stays-queued-build ## Confirm a process-state defect remains an explicit CI error
	mkdir -p "$(dir $(REPLAY_PROCESS_DEFECT_CI_RESULT_OUTPUT))"
	status=0; $(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(DEFECT_PROCESS_STAYS_QUEUED_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --base-url "$(DEFECT_URL)" --ready-path "$(DEFECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_PROCESS_DEFECT_CI_RESULT_OUTPUT)" || status=$$?; test "$$status" -eq 1
	jq -e '.status == "error" and .report.status == "inconclusive" and .report.replay_verdict.status == "fail" and (.report.differences | length) > 0' "$(REPLAY_PROCESS_DEFECT_CI_RESULT_OUTPUT)" >/dev/null

sorna-replay-persistence-defect-fresh: sorna-run defect-persistence-wrong-key-build ## Confirm a persistence defect remains an explicit CI error
	mkdir -p "$(dir $(REPLAY_PERSISTENCE_DEFECT_CI_RESULT_OUTPUT))"
	status=0; $(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(DEFECT_PERSISTENCE_WRONG_KEY_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --base-url "$(DEFECT_URL)" --ready-path "$(DEFECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_PERSISTENCE_DEFECT_CI_RESULT_OUTPUT)" || status=$$?; test "$$status" -eq 1
	jq -e '.status == "error" and .report.status == "inconclusive" and .report.replay_verdict.status == "fail" and (.report.differences | length) > 0' "$(REPLAY_PERSISTENCE_DEFECT_CI_RESULT_OUTPUT)" >/dev/null

sorna-replay-remove-name-defect-fresh: sorna-run defect-remove-name-build ## Confirm a response-shape defect becomes a failed replay CI result
	mkdir -p "$(dir $(REPLAY_REMOVE_NAME_DEFECT_CI_RESULT_OUTPUT))"
	status=0; $(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(DEFECT_REMOVE_NAME_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --base-url "$(DEFECT_URL)" --ready-path "$(DEFECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_REMOVE_NAME_DEFECT_CI_RESULT_OUTPUT)" || status=$$?; test "$$status" -eq 1
	jq -e '.status == "failed" and .report.status == "drifted" and (.report.differences | length) > 0' "$(REPLAY_REMOVE_NAME_DEFECT_CI_RESULT_OUTPUT)" >/dev/null

sorna-replay-accepts-png-defect-fresh: sorna-run defect-accepts-png-build ## Confirm an input-validation defect becomes a failed replay CI result
	mkdir -p "$(dir $(REPLAY_ACCEPTS_PNG_DEFECT_CI_RESULT_OUTPUT))"
	status=0; $(GO_CMD) run ./examples/document-pipeline-lab/replay/cmd/replay-fixture --project-root "$(CURDIR)" --subject-command "$(DEFECT_ACCEPTS_PNG_BINARY)" --subject-arg=-addr --subject-arg "$(DEFECT_ADDR)" --base-url "$(DEFECT_URL)" --ready-path "$(DEFECT_READY_PATH)" --startup-timeout "$(REPLAY_FIXTURE_STARTUP_TIMEOUT)" --oracle "$(ORACLE_OUTPUT_DIR)/oracle.json" --evidence "$(RUN_OUTPUT_DIR)" --output "$(REPLAY_ACCEPTS_PNG_DEFECT_CI_RESULT_OUTPUT)" || status=$$?; test "$$status" -eq 1
	jq -e '.status == "failed" and .report.status == "drifted" and (.report.differences | length) > 0' "$(REPLAY_ACCEPTS_PNG_DEFECT_CI_RESULT_OUTPUT)" >/dev/null

sorna-replay-regression: sorna-replay-defect-fresh sorna-replay-stateful-defect-fresh sorna-replay-process-defect-fresh sorna-replay-persistence-defect-fresh sorna-replay-remove-name-defect-fresh sorna-replay-accepts-png-defect-fresh ## Run complete-drift and incomplete-stateful replay regressions

sorna-replay-matrix-ci-result: sorna-replay-regression ## Aggregate the replay regression envelopes into one Sorna CI result
	mkdir -p "$(dir $(REPLAY_MATRIX_CI_RESULT_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna evidence replay matrix --manifest "$(REPLAY_MATRIX_MANIFEST)" --source-root "$(REPLAY_MATRIX_SOURCE_ROOT)" --output "$(REPLAY_MATRIX_CI_RESULT_OUTPUT)"

sorna-replay-matrix-verify: ## Verify the saved replay matrix and all available member inputs
	$(GO_CMD) run ./sorna/cmd/sorna evidence replay matrix verify --source-root "$(REPLAY_MATRIX_SOURCE_ROOT)" "$(REPLAY_MATRIX_CI_RESULT_OUTPUT)"

sorna-replay-matrix-ci-result-fresh: ## Rebuild and independently verify the replay matrix in a fresh temporary workspace
	workspace=$$(mktemp -d /private/tmp/ingen-sorna-replay-matrix-workspace.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\n" "$$workspace" "$$workspace/.artifacts"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" ARTIFACT_ROOT=.artifacts GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" sorna-replay-matrix-ci-result; status=$$?; \
	if test "$$status" -eq 0; then \
		$(MAKE) -C "$$workspace" ARTIFACT_ROOT=.artifacts GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" sorna-replay-matrix-verify; status=$$?; \
	fi; \
	exit $$status

sorna-alpha-check: ## Run the complete Sorna package, schema, and fresh replay readiness checks
	$(GO_CMD) test ./sorna/...
	$(GO_CMD) vet ./sorna/...
	jq empty sorna/spec/*.json
	$(MAKE) sorna-replay-matrix-ci-result-fresh

sorna-release-check: sorna-alpha-check mutation-provider-conformance ## Run the complete Sorna checkpoint plus provider and downstream CI proofs
	if test -n "$(SORNA_RELEASE_WORKSPACE)"; then \
		workspace="$(SORNA_RELEASE_WORKSPACE)"; \
		mkdir -p "$$workspace"; \
		if test -n "$$(find "$$workspace" -mindepth 1 -maxdepth 1 -print -quit)"; then \
			echo "Sorna release workspace is not empty: $$workspace" >&2; \
			exit 2; \
		fi; \
	else \
		workspace=$$(mktemp -d /private/tmp/ingen-sorna-release-check.XXXXXX); \
	fi; \
	trap 'printf "workspace: %s\nartifact root: %s\n" "$$workspace" "$$workspace"' EXIT; \
	$(MAKE) ARTIFACT_ROOT="$$workspace" mutation-typescript-provider-conformance

sorna-gate: ## Apply the default CI gate to RUN_OUTPUT_DIR; set GATE_MIN_OBSERVATION_COVERAGE for a strict minimum
	$(GO_CMD) run ./sorna/cmd/sorna gate $(if $(GATE_MIN_OBSERVATION_COVERAGE),--minimum-observation-coverage "$(GATE_MIN_OBSERVATION_COVERAGE)",) "$(RUN_OUTPUT_DIR)"

sorna-ci-result: sorna-run ## Run the clean baseline and write its language-neutral CI result envelope
	mkdir -p "$(dir $(CI_RESULT_OUTPUT))"
	$(GO_CMD) run ./sorna/cmd/sorna gate --format ci-result $(if $(GATE_MIN_OBSERVATION_COVERAGE),--minimum-observation-coverage "$(GATE_MIN_OBSERVATION_COVERAGE)",) "$(RUN_OUTPUT_DIR)" > "$(CI_RESULT_OUTPUT)"

nublar-aggregate: sorna-ci-result mutation-go-provider-ci-result mutation-go-preparation-ci-result mutation-go-campaign-ci-result ## Aggregate Sorna behavioral, preparation, strict provider-preflight, and Go mutation CI results through Nublar
	mkdir -p "$(dir $(NUBLAR_RESULT_OUTPUT))"
	$(GO_CMD) run ./nublar/cmd/nublar aggregate --workflow "$(NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --output "$(NUBLAR_RESULT_OUTPUT)"

nublar-run-collect: sorna-ci-result mutation-go-provider-ci-result mutation-go-preparation-ci-result mutation-go-campaign-ci-result ## Collect and persist the document-pipeline Nublar run artifact
	mkdir -p "$(dir $(NUBLAR_RUN_OUTPUT))" "$(NUBLAR_RUN_STORE)"
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "$(NUBLAR_WORKFLOW)" --root "$(ARTIFACT_ROOT)" --store "$(NUBLAR_RUN_STORE)" --output "$(NUBLAR_RUN_OUTPUT)"

nublar-run-collect-fresh: ## Run the complete document-pipeline workflow and persist its Nublar run in a fresh workspace
	workspace=$$(mktemp -d /private/tmp/ingen-nublar-workspace.XXXXXX); \
	trap 'printf "workspace: %s\nartifact root: %s\nrun store: %s\n" "$$workspace" "$$workspace/.artifacts" "$$workspace/.artifacts/nublar-runs"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" nublar-run-collect; status=$$?; \
	exit $$status

nublar-aggregate-fresh: ## Run the complete document-pipeline workflow in a fresh temporary workspace
	workspace=$$(mktemp -d /private/tmp/ingen-workspace.XXXXXX); \
	trap 'printf "workspace: %s\\nartifact root: %s\\n" "$$workspace" "$$workspace/.artifacts"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" nublar-aggregate; status=$$?; \
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

mutation-provider-conformance: ## Run language-neutral provider conformance fixtures
	$(GO_CMD) test ./sorna/internal/campaign

mutation-typescript-provider-conformance: ## Emit a TypeScript provider manifest and validate it with Sorna
	$(TYPESCRIPT_CMD) test "$(TYPESCRIPT_PROVIDER_DIR)/test/provider.test.mjs"
	mkdir -p "$(dir $(TYPESCRIPT_PROVIDER_OUTPUT))"
	$(TYPESCRIPT_CMD) run "$(TYPESCRIPT_PROVIDER_DIR)/bin/emit-provider.mjs" -- --plan "$(TYPESCRIPT_PROVIDER_PLAN)" --output "$(TYPESCRIPT_PROVIDER_OUTPUT)" --id "$(TYPESCRIPT_PROVIDER_ID)" --command "$(TYPESCRIPT_PROVIDER_COMMAND)" --arg --address --arg '$${SORA_ADDR}'
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider validate "$(TYPESCRIPT_PROVIDER_OUTPUT)"
	$(GO_CMD) run ./sorna/cmd/sorna mutation provider inspect "$(TYPESCRIPT_PROVIDER_PLAN)" --provider "$(TYPESCRIPT_PROVIDER_OUTPUT)" --require-plan-binding --format ci-result --output "$(TYPESCRIPT_PROVIDER_REVIEW_OUTPUT)"
	$(GO_CMD) run ./nublar/cmd/nublar aggregate --workflow "$(TYPESCRIPT_PROVIDER_DIR)/fixtures/nublar-workflow.yaml" --root "$(dir $(TYPESCRIPT_PROVIDER_REVIEW_OUTPUT))" --output "$(TYPESCRIPT_PROVIDER_NUBLAR_OUTPUT)"
	jq -e '.schema == "ingen.nublar-result/v1" and .status == "passed" and (.results | length) == 1 and .results[0].result.tool == "sorna" and .results[0].result.kind == "mutation-provider-review"' "$(TYPESCRIPT_PROVIDER_NUBLAR_OUTPUT)"
	$(GO_CMD) run ./nublar/cmd/nublar run collect --workflow "$(TYPESCRIPT_PROVIDER_DIR)/fixtures/nublar-workflow.yaml" --root "$(dir $(TYPESCRIPT_PROVIDER_REVIEW_OUTPUT))" --store "$(TYPESCRIPT_PROVIDER_NUBLAR_STORE)" --run-id "$(TYPESCRIPT_PROVIDER_NUBLAR_RUN_ID)" --external-system typescript-smoke --external-id provider-handoff --attempt 1 --output "$(TYPESCRIPT_PROVIDER_NUBLAR_RUN_OUTPUT)"
	$(GO_CMD) run ./nublar/cmd/nublar run show --store "$(TYPESCRIPT_PROVIDER_NUBLAR_STORE)" --run-id "$(TYPESCRIPT_PROVIDER_NUBLAR_RUN_ID)" --output "$(TYPESCRIPT_PROVIDER_NUBLAR_SHOW_OUTPUT)"
	$(GO_CMD) run ./nublar/cmd/nublar run decision --store "$(TYPESCRIPT_PROVIDER_NUBLAR_STORE)" --run-id "$(TYPESCRIPT_PROVIDER_NUBLAR_RUN_ID)" --output "$(TYPESCRIPT_PROVIDER_NUBLAR_DECISION_OUTPUT)"
	jq -e '.schema == "ingen.nublar-run/v1" and .status == "passed" and .correlation.system == "typescript-smoke" and .correlation.id == "provider-handoff" and .correlation.attempt == 1 and .checks[0].result.artifact.kind == "mutation-provider-review"' "$(TYPESCRIPT_PROVIDER_NUBLAR_RUN_OUTPUT)"
	jq -e '.schema == "ingen.nublar-run/v1" and .run_id == "$(TYPESCRIPT_PROVIDER_NUBLAR_RUN_ID)" and .checks[0].result.artifact.report.status == "ready"' "$(TYPESCRIPT_PROVIDER_NUBLAR_SHOW_OUTPUT)"
	jq -e '.schema == "ingen.nublar-decision/v1" and .status == "passed" and (.checks[0].result.sha256 | length) == 64' "$(TYPESCRIPT_PROVIDER_NUBLAR_DECISION_OUTPUT)"

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

mutation-go-campaign-ci-result-fresh: ## Run the Go mutation campaign in a fresh workspace
	workspace=$$(mktemp -d /private/tmp/ingen-sorna-mutation-campaign-workspace.XXXXXX); \
	trap 'printf "workspace: %s\\nartifact root: %s\\n" "$$workspace" "$$workspace/.artifacts"' EXIT; \
	rsync -a --exclude='.git' --exclude='.artifacts' --exclude='.cache' ./ "$$workspace/" && \
	$(MAKE) -C "$$workspace" ARTIFACT_ROOT=.artifacts GO_CACHE="$(abspath $(GO_CACHE))" GO_MOD_CACHE="$(abspath $(GO_MOD_CACHE))" mutation-go-campaign-ci-result; campaign_exit=$$?; \
	exit $$campaign_exit

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
