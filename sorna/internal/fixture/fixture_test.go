package fixture

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
)

func TestBindVerifiesProviderFileAgainstSealedContract(t *testing.T) {
	root := t.TempDir()
	contents := []byte("Read the contract.\n")
	if err := os.WriteFile(filepath.Join(root, "welcome-document.txt"), contents, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := hashBytes(contents)
	document := sealedFixtureContract(digest)
	contractBytes, err := contract.CanonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	provider := ProviderManifest{
		Schema:  ProviderSchema,
		ID:      "document-fixtures",
		Version: 1,
		Fixtures: []ProviderFixture{{
			ID:     "welcome-document",
			Path:   "welcome-document.txt",
			SHA256: digest,
		}},
	}

	handoff, err := Bind(document, hashBytes(contractBytes), provider, root)
	if err != nil {
		t.Fatal(err)
	}
	if handoff.Status != "verified" || len(handoff.Fixtures) != 1 {
		t.Fatalf("handoff = %+v, want one verified fixture", handoff)
	}
	if got := handoff.Fixtures[0]; got.ID != "welcome-document" || got.Path != "welcome-document.txt" || got.SHA256 != digest || got.Bytes != int64(len(contents)) {
		t.Fatalf("bound fixture = %+v, want provider path, digest, and byte count", got)
	}
	if handoff.Provider.SHA256 == "" || handoff.Contract.SHA256 == "" {
		t.Fatalf("handoff references = %+v, want provider and contract hashes", handoff)
	}
}

func TestBindRejectsProviderBytesWithWrongDigest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "welcome-document.txt"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	declaredDigest := strings.Repeat("a", 64)
	document := sealedFixtureContract(declaredDigest)
	contractBytes, err := contract.CanonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	provider := validProvider(strings.Repeat("b", 64))
	if _, err := Bind(document, hashBytes(contractBytes), provider, root); err == nil || !strings.Contains(err.Error(), "provider digest") {
		t.Fatalf("Bind() = %v, want provider/contract digest mismatch", err)
	}

	provider = validProvider(declaredDigest)
	if _, err := Bind(document, hashBytes(contractBytes), provider, root); err == nil || !strings.Contains(err.Error(), "bytes digest") {
		t.Fatalf("Bind() with tampered bytes = %v, want observed digest mismatch", err)
	}
}

func TestBindRejectsSubjectOwnedAndUnsafeProviderFixtures(t *testing.T) {
	root := t.TempDir()
	digest := strings.Repeat("a", 64)
	document := sealedFixtureContract(digest)
	contractBytes, err := contract.CanonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	provider := validProvider(digest)
	provider.Fixtures[0].Path = "../outside.txt"
	if _, err := Bind(document, hashBytes(contractBytes), provider, root); err == nil || !strings.Contains(err.Error(), "path must be relative") {
		t.Fatalf("Bind() with unsafe path = %v, want provider path validation error", err)
	}

	document = sealedFixtureContract(digest)
	document.Contract["fixtures"].([]any)[0].(map[string]any)["owner"] = "subject"
	contractBytes, err = contract.CanonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	provider = validProvider(digest)
	if _, err := Bind(document, hashBytes(contractBytes), provider, root); err == nil || !strings.Contains(err.Error(), "owner must be oracle") {
		t.Fatalf("Bind() with subject fixture = %v, want ownership boundary error", err)
	}
}

func TestLoadProviderFileRequiresTheVersionedWrapper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.yaml")
	contents := `fixture_provider:
  schema: ingen.fixture-provider/v1
  id: document-fixtures
  version: 1
  fixtures:
    - id: welcome-document
      path: welcome-document.txt
      sha256: ` + strings.Repeat("a", 64) + `
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	provider, err := LoadProviderFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if provider.ID != "document-fixtures" || len(provider.Fixtures) != 1 {
		t.Fatalf("provider = %+v, want loaded fixture provider", provider)
	}
}

func TestHandoffRoundTripIsCanonical(t *testing.T) {
	handoff := Handoff{
		Schema:   HandoffSchema,
		Status:   "verified",
		Contract: ContractReference{ID: "document-fixture", Version: 1, SHA256: strings.Repeat("a", 64)},
		Provider: ProviderReference{ID: "document-fixtures", Version: 1, SHA256: strings.Repeat("b", 64)},
		Fixtures: []BoundFixture{{ID: "welcome-document", Path: "welcome-document.txt", SHA256: strings.Repeat("c", 64), Bytes: 18}},
	}
	path := filepath.Join(t.TempDir(), "handoff.json")
	if _, err := WriteFile(path, handoff); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, handoff) {
		t.Fatalf("loaded handoff = %+v, want %+v", loaded, handoff)
	}
}

func sealedFixtureContract(digest string) contract.Document {
	return contract.Document{Contract: map[string]any{
		"schema":    contract.Schema,
		"id":        "document-fixture",
		"version":   int64(1),
		"status":    "sealed",
		"interface": map[string]any{"kind": "http-json"},
		"rules": []any{map[string]any{
			"id":       "submit",
			"strength": "must",
			"subject":  "POST /documents",
		}},
		"unspecified": []any{},
		"fixtures": []any{map[string]any{
			"id":      "welcome-document",
			"owner":   "oracle",
			"purpose": "canonical document input bytes",
			"sha256":  digest,
		}},
	}}
}

func validProvider(digest string) ProviderManifest {
	return ProviderManifest{
		Schema:  ProviderSchema,
		ID:      "document-fixtures",
		Version: 1,
		Fixtures: []ProviderFixture{{
			ID:     "welcome-document",
			Path:   "welcome-document.txt",
			SHA256: digest,
		}},
	}
}
