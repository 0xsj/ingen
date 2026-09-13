package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndValidateDocumentPipelineContract(t *testing.T) {
	document, err := LoadFile(filepath.Join("..", "..", "..", "examples", "document-pipeline-lab", "contract", "contract.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(document); len(problems) != 0 {
		t.Fatalf("unexpected validation problems: %v", problems)
	}
	if got := document.Contract["id"]; got != "document-pipeline" {
		t.Fatalf("contract id = %v, want document-pipeline", got)
	}
}

func TestCanonicalJSONDoesNotDependOnMapInsertionOrder(t *testing.T) {
	first := Document{Contract: map[string]any{
		"schema":      "ingen.contract/v1",
		"id":          "example",
		"version":     int64(1),
		"status":      "draft",
		"interface":   map[string]any{"kind": "http-json"},
		"rules":       []any{},
		"unspecified": []any{},
	}}
	second := Document{Contract: map[string]any{
		"unspecified": []any{},
		"rules":       []any{},
		"interface":   map[string]any{"kind": "http-json"},
		"status":      "draft",
		"version":     int64(1),
		"id":          "example",
		"schema":      "ingen.contract/v1",
	}}

	firstBytes, err := CanonicalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := CanonicalJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("canonical forms differ:\n%s\n%s", firstBytes, secondBytes)
	}
}

func TestSealLeavesInputDraftAndHashesCanonicalArtifact(t *testing.T) {
	document := minimalDocument()
	sealed, err := Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	if document.Contract["status"] != "draft" {
		t.Fatalf("input status changed to %v", document.Contract["status"])
	}
	if sealed.Document.Contract["status"] != "sealed" {
		t.Fatalf("sealed status = %v", sealed.Document.Contract["status"])
	}
	digest := sha256.Sum256(sealed.CanonicalJSON)
	if sealed.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("hash = %s, want hash of canonical bytes", sealed.SHA256)
	}
	if !strings.Contains(string(sealed.CanonicalJSON), `"status":"sealed"`) {
		t.Fatalf("sealed canonical document does not contain sealed status: %s", sealed.CanonicalJSON)
	}
}

func TestSealAttachesAndVerifiesFixtureHash(t *testing.T) {
	directory := t.TempDir()
	fixturePath := filepath.Join(directory, "fixture.txt")
	if err := os.WriteFile(fixturePath, []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(directory, "contract.yaml")
	contract := `contract:
  schema: ingen.contract/v1
  id: fixture-example
  version: 1
  status: draft
  interface:
    kind: http-json
  rules: []
  unspecified: []
  fixtures:
    - path: fixture.txt
      purpose: known fixture
`
	if err := os.WriteFile(contractPath, []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}

	sealed, err := SealFile(contractPath, filepath.Join(directory, "sealed"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := sealed.Document.Contract["fixtures"].([]any)[0].(map[string]any)
	if fixture["sha256"] == "" {
		t.Fatal("fixture hash was not attached")
	}
}

func TestValidationRejectsDuplicateRulesAndUnboundedGenerator(t *testing.T) {
	document := minimalDocument()
	document.Contract["rules"] = []any{
		map[string]any{
			"id":       "same",
			"strength": "must",
			"subject":  "POST /documents",
			"given": map[string]any{
				"body": map[string]any{
					"content": map[string]any{
						"generated": map[string]any{"kind": "repeat", "value": "a"},
					},
				},
			},
		},
		map[string]any{
			"id":       "same",
			"strength": "must",
			"subject":  "POST /documents",
		},
	}

	problems := Validate(document)
	if !containsProblem(problems, "duplicates") || !containsProblem(problems, "generated.count") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestValidationRequiresSetupForStatefulRule(t *testing.T) {
	document := minimalDocument()
	document.Contract["rules"] = []any{map[string]any{
		"id":       "stateful-rule",
		"strength": "must",
		"subject":  "GET /documents/{document_id}",
		"given":    map[string]any{"state": "document_accepted"},
	}}

	problems := Validate(document)
	if !containsProblem(problems, "given.setup is required") {
		t.Fatalf("problems = %v, want missing setup problem", problems)
	}
}

func minimalDocument() Document {
	return Document{Contract: map[string]any{
		"schema":      "ingen.contract/v1",
		"id":          "example",
		"version":     int64(1),
		"status":      "draft",
		"interface":   map[string]any{"kind": "http-json"},
		"rules":       []any{},
		"unspecified": []any{},
	}}
}

func containsProblem(problems []string, fragment string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, fragment) {
			return true
		}
	}
	return false
}
