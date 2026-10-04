package spec

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
)

func TestProducedCodexMatrixPreservesUncertainty(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := agentprobe.Compare(ctx, agentprobe.MatrixRequest{ExecutablePaths: []string{executable}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	schema := compileNativeBoundarySchema(t, "codex-compatibility-matrix-v1.schema.json")
	document := boundaryDocument(t, report)
	if err := schema.Validate(document); err != nil {
		t.Fatalf("produced canceled matrix rejected: %v", err)
	}
	document["status"] = "supported"
	if err := schema.Validate(document); err == nil {
		t.Fatal("accepted matrix support without a supported candidate")
	}
	document["status"] = report.Status
	document["credential"] = "undeclared"
	if err := schema.Validate(document); err == nil {
		t.Fatal("accepted an undeclared credential")
	}
}

func TestPublishedCodexMatrix(t *testing.T) {
	path := os.Getenv("INGEN_CODEX_MATRIX_REPORT")
	if path == "" {
		t.Skip("set INGEN_CODEX_MATRIX_REPORT to validate a produced compatibility matrix")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	if err := compileNativeBoundarySchema(t, "codex-compatibility-matrix-v1.schema.json").Validate(document); err != nil {
		t.Fatal(err)
	}
}
