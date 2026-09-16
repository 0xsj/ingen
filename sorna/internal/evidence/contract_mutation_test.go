package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
)

func TestBuildContractMutationCIResultBindsReportContractAndOracle(t *testing.T) {
	root := t.TempDir()
	contractPath := filepath.Join(root, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(`contract:
  schema: ingen.contract/v1
  id: ci-contract
  version: 1
  status: draft
  interface:
    kind: http-json
  rules:
    - id: health
      strength: must
      subject: GET /healthz
      expect:
        status: 200
  unspecified: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	document, err := contract.LoadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := contract.SealAt(document, root)
	if err != nil {
		t.Fatal(err)
	}
	oracleArtifact, err := oracle.Generate(sealed, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	oraclePath := filepath.Join(root, "oracle.json")
	if _, err := oracle.WriteFile(oraclePath, oracleArtifact); err != nil {
		t.Fatal(err)
	}
	report, err := mutation.AnalyzeContractMutations(sealed, oracleArtifact, []mutation.Spec{{
		ID:       "weaken-health-status",
		Plane:    "contract",
		Operator: "contract.rule.expect.status.replace",
		Target:   "rule:health",
		Change:   map[string]any{"from": 200, "to": 204},
	}})
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "contract-mutations.json")
	if err := mutation.WriteContractMutationReport(reportPath, report); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildContractMutationCIResult(reportPath, contractPath, oraclePath, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "passed" || artifact.ExitCode != 0 || artifact.Kind != "contract-mutation-inspection" {
		t.Fatalf("artifact = %+v, want passed contract-mutation-inspection", artifact)
	}
	if artifact.Inputs["contract"].SHA256 != sealed.SHA256 || artifact.Inputs["oracle"].SHA256 == "" || artifact.Inputs["report"].SHA256 == "" {
		t.Fatalf("artifact inputs = %+v, want bound contract, oracle, and report hashes", artifact.Inputs)
	}
	var explanation ContractMutationExplanation
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.Schema != contractMutationExplanationSchema || len(explanation.VisibleMutations) != 1 || explanation.VisibleMutations[0] != "weaken-health-status" {
		t.Fatalf("explanation = %+v, want one visible mutation", explanation)
	}
}

func TestBuildContractMutationCIResultFailsForInvalidMutation(t *testing.T) {
	root := t.TempDir()
	contractPath := filepath.Join(root, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(`contract:
  schema: ingen.contract/v1
  id: ci-contract
  version: 1
  status: draft
  interface:
    kind: http-json
  rules:
    - id: health
      strength: must
      subject: GET /healthz
      expect:
        status: 200
  unspecified: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	document, err := contract.LoadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := contract.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	oracleArtifact, err := oracle.Generate(sealed, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	oraclePath := filepath.Join(root, "oracle.json")
	if _, err := oracle.WriteFile(oraclePath, oracleArtifact); err != nil {
		t.Fatal(err)
	}
	report, err := mutation.AnalyzeContractMutations(sealed, oracleArtifact, []mutation.Spec{{
		ID:       "unsupported",
		Plane:    "contract",
		Operator: "contract.unknown",
		Target:   "rule:health",
		Change:   map[string]any{"value": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "contract-mutations.json")
	if err := mutation.WriteContractMutationReport(reportPath, report); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildContractMutationCIResult(reportPath, contractPath, oraclePath, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.ExitCode != 1 {
		t.Fatalf("artifact = %+v, want failed with exit code 1", artifact)
	}
	var explanation ContractMutationExplanation
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if len(explanation.InvalidMutations) != 1 || explanation.InvalidMutations[0] != "unsupported" {
		t.Fatalf("explanation = %+v, want one invalid mutation", explanation)
	}
}
