package integration

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	nublarrun "ingen/nublar/internal/run"
	nublarstore "ingen/nublar/internal/storage/filesystem"
)

func TestMixedProducerWorkflowComposesSharedStatuses(t *testing.T) {
	workflowPath := filepath.Join("..", "testdata", "workflows", "mixed-producers.yaml")
	artifactRoot := filepath.Join("..", "testdata", "ci-results")
	record, err := nublarrun.CollectWorkflowFile(workflowPath, artifactRoot, "mixed-run-01")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || record.ExitCode != 1 || len(record.Checks) != 2 {
		t.Fatalf("mixed-producer run = %+v, want failed run with two checks", record)
	}
	if record.Checks[0].Tool != "sorna" || record.Checks[0].Status != "passed" {
		t.Fatalf("Sorna check = %+v, want passed Sorna result", record.Checks[0])
	}
	if record.Checks[1].Tool != "paddock" || record.Checks[1].Status != "failed" {
		t.Fatalf("Paddock check = %+v, want failed Paddock result", record.Checks[1])
	}
	if compactJSON(record.Checks[0].Result.Artifact.Report) != `{"schema":"test.sorna-report/v1","cases":7}` {
		t.Fatalf("Sorna report = %s, want opaque producer report", record.Checks[0].Result.Artifact.Report)
	}
	if compactJSON(record.Checks[1].Result.Artifact.Report) != `{"schema":"test.paddock-report/v1","violations":[{"rule":"example-boundary","from":"app.domain","to":"app.adapters"}]}` {
		t.Fatalf("Paddock report = %s, want opaque producer report", record.Checks[1].Result.Artifact.Report)
	}
	if len(record.Checks[0].Result.Ref.SHA256) != 64 || len(record.Checks[1].Result.Ref.SHA256) != 64 {
		t.Fatalf("result hashes = %q, %q, want SHA-256 references", record.Checks[0].Result.Ref.SHA256, record.Checks[1].Result.Ref.SHA256)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}

	store, err := nublarstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "failed" || loaded.Checks[1].Result.Artifact.Tool != "paddock" {
		t.Fatalf("stored mixed-producer run = %+v, want preserved Paddock result", loaded)
	}
}

func compactJSON(value []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return string(value)
	}
	return compact.String()
}
