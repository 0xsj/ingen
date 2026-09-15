package custody

import (
	"strings"
	"testing"
)

func TestSchemaV2AcceptsRemoteSourceProvenance(t *testing.T) {
	record := testRecord(t, "lockwood-source-v2")
	record.Schema = SchemaV2
	record.Source = Source{
		RunID:   "run-remote-0001",
		URI:     "s3://evidence.example/runs/run-remote-0001/result.json",
		Version: "version-0001",
	}
	record.Parents = []Lineage{}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaV1RejectsRemoteSourceFields(t *testing.T) {
	record := testRecord(t, "lockwood-source-v1")
	record.Source.URI = "https://evidence.example/result.json"
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "require custody schema") {
		t.Fatalf("Validate error = %v, want v2 requirement", err)
	}
}

func TestRemoteSourceRejectsCredentialsAndUnpairedVersion(t *testing.T) {
	withCredentials := testRecord(t, "lockwood-source-credentials")
	withCredentials.Schema = SchemaV2
	withCredentials.Source = Source{URI: "https://user:secret@evidence.example/result.json"}
	if err := withCredentials.Validate(); err == nil || !strings.Contains(err.Error(), "credential-free") {
		t.Fatalf("credential URI error = %v, want credential-free rejection", err)
	}

	withoutURI := testRecord(t, "lockwood-source-version")
	withoutURI.Schema = SchemaV2
	withoutURI.Source = Source{RunID: "run-0001", Version: "version-0001"}
	if err := withoutURI.Validate(); err == nil || !strings.Contains(err.Error(), "requires source uri") {
		t.Fatalf("unpaired version error = %v, want URI requirement", err)
	}
}
