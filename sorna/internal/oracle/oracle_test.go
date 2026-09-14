package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
)

func TestGenerateMaterializesAndBindsCasesToContractAndPolicy(t *testing.T) {
	sealed, err := contract.Seal(testContract())
	if err != nil {
		t.Fatal(err)
	}
	policyHash := strings.Repeat("b", 64)
	artifact, err := Generate(sealed, policyHash)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Schema != Schema || artifact.Status != "frozen" {
		t.Fatalf("artifact identity = %+v, want frozen %s", artifact, Schema)
	}
	if artifact.Contract.SHA256 != sealed.SHA256 || artifact.PolicySHA256 != policyHash {
		t.Fatalf("artifact references = %+v, want contract and policy hashes", artifact)
	}
	if len(artifact.Cases) != 1 || artifact.Cases[0].CaseID != "case-0001" {
		t.Fatalf("cases = %+v, want one stable case", artifact.Cases)
	}
	body := artifact.Cases[0].Given["body"].(map[string]any)
	if body["content"] != "ababab" {
		t.Fatalf("materialized body = %+v, want repeated content", body)
	}
}

func TestWriteAndLoadPreserveCanonicalOracleBytes(t *testing.T) {
	sealed, err := contract.Seal(testContract())
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := Generate(sealed, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "oracle.json")
	hash, err := WriteFile(path, artifact)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	if hash != hex.EncodeToString(digest[:]) {
		t.Fatalf("hash = %s, want hash of written bytes", hash)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalJSON(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != string(contents) {
		t.Fatalf("loaded artifact changed canonical bytes:\n%s\n%s", contents, canonical)
	}
}

func TestLoadRejectsValidButNonCanonicalOracleBytes(t *testing.T) {
	sealed, err := contract.Seal(testContract())
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := Generate(sealed, strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "oracle.json")
	canonical, err := CanonicalJSON(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append([]byte(" "), canonical...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "not canonical JSON") {
		t.Fatalf("LoadFile() = %v, want canonical JSON error", err)
	}
}

func testContract() contract.Document {
	return contract.Document{Contract: map[string]any{
		"schema":    "ingen.contract/v1",
		"id":        "oracle-test",
		"version":   int64(1),
		"status":    "draft",
		"interface": map[string]any{"kind": "http-json"},
		"rules": []any{map[string]any{
			"id":       "document.create.large",
			"strength": "must",
			"subject":  "POST /documents",
			"given": map[string]any{
				"body": map[string]any{
					"content": map[string]any{
						"generated": map[string]any{"kind": "repeat", "value": "ab", "count": int64(3)},
					},
				},
			},
			"expect": map[string]any{"status": int64(202)},
		}},
		"unspecified": []any{},
	}}
}
