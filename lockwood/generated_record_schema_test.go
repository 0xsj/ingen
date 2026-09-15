package lockwood

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestGeneratedCustodyRecordConformsToDraftSchema(t *testing.T) {
	root := t.TempDir()
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ingestor.Accept(bytes.NewBufferString("generated record"), custody.IntakeRequest{
		CustodyID: "lockwood-generated-schema-0001",
		MediaType: "text/plain",
		Producer:  custody.Producer{Tool: "example", Kind: "schema-fixture"},
		Source:    custody.Source{Path: "generated.txt"},
		Handling:  custody.Handling{Redaction: "none", RetentionClass: "default"},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "records", "lockwood-generated-schema-0001.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	schema := compileLockwoodSchema(t, custodySchemaURL, "spec/lockwood.custody-v1.schema.json")
	if err := schema.Validate(document); err != nil {
		t.Fatalf("generated custody record rejected by draft schema: %v\n%s", err, raw)
	}
}
