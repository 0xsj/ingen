package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/hammond/internal/governance"
)

func TestCLIFullGovernanceLifecycle(t *testing.T) {
	storeDir := t.TempDir()
	fixtureDir := t.TempDir()
	digestOne := "6b40dfb15fa67f96c9f3bc79bc46206d45f6d44124197b344757499299e43445"
	digestTwo := digestOne

	predecessorPath := writeCLIJSON(t, fixtureDir, "predecessor.json", registeredRecord("document-pipeline-v1", 1, digestOne))
	successorPath := writeCLIJSON(t, fixtureDir, "successor.json", registeredRecord("document-pipeline-v2", 2, digestTwo))
	reviewOnePath := writeCLIJSON(t, fixtureDir, "review-one.json", governance.Event{
		ID: "event-002", Type: governance.EventReviewOpened, Actor: "owner", At: "2026-09-15T00:01:00Z", ReviewCycleID: "review-001",
	})
	approvalOnePath := writeCLIJSON(t, fixtureDir, "approval-one.json", governance.Event{
		ID: "event-003", Type: governance.EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: governance.DecisionApprove, ArtifactSHA256: digestOne, At: "2026-09-15T00:02:00Z",
	})
	reviewTwoPath := writeCLIJSON(t, fixtureDir, "review-two.json", governance.Event{
		ID: "event-002", Type: governance.EventReviewOpened, Actor: "owner", At: "2026-09-15T00:04:00Z", ReviewCycleID: "review-002",
	})
	approvalTwoPath := writeCLIJSON(t, fixtureDir, "approval-two.json", governance.Event{
		ID: "event-003", Type: governance.EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-002", Decision: governance.DecisionApprove, ArtifactSHA256: digestTwo, At: "2026-09-15T00:05:00Z",
	})

	assertCLIExitCode(t, run([]string{"register", "--store", storeDir, "--record", predecessorPath}), "register predecessor")
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", predecessorPath, "--event", reviewOnePath}), "open predecessor review")
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", predecessorPath, "--event", approvalOnePath}), "approve predecessor")
	predecessorRevision := cliRevision(t, storeDir, predecessorPath)
	assertCLIExitCode(t, run([]string{
		"amend", "--store", storeDir, "--record", predecessorPath, "--successor", successorPath,
		"--event-id", "event-004", "--actor", "owner", "--at", "2026-09-15T00:03:00Z",
		"--kind", "clarifying", "--reason", "Clarify the public description.", "--if-revision", predecessorRevision,
	}), "create amendment")
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", successorPath, "--event", reviewTwoPath}), "open successor review")
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", successorPath, "--event", approvalTwoPath}), "approve successor")
	predecessorRevision = cliRevision(t, storeDir, predecessorPath)
	assertCLIExitCode(t, run([]string{
		"supersede", "--store", storeDir, "--record", predecessorPath, "--successor", successorPath,
		"--event-id", "event-005", "--actor", "owner", "--at", "2026-09-15T00:06:00Z", "--if-revision", predecessorRevision,
	}), "supersede predecessor")

	code, output := captureCLIOutput(t, []string{"lineage", "--store", storeDir})
	if code != 0 {
		t.Fatalf("lineage exit code = %d, output = %s", code, output)
	}
	if !strings.Contains(output, "document-pipeline/document-pipeline@1 state=superseded") || !strings.Contains(output, "document-pipeline/document-pipeline@2 state=approved") {
		t.Fatalf("lineage output = %q, want both lifecycle states", output)
	}
}

func TestCLIConditionalAppendRejectsStaleRevision(t *testing.T) {
	storeDir := t.TempDir()
	fixtureDir := t.TempDir()
	digest := "6b40dfb15fa67f96c9f3bc79bc46206d45f6d44124197b344757499299e43445"
	recordPath := writeCLIJSON(t, fixtureDir, "record.json", registeredRecord("document-pipeline-v1", 1, digest))
	reviewPath := writeCLIJSON(t, fixtureDir, "review.json", governance.Event{
		ID: "event-002", Type: governance.EventReviewOpened, Actor: "owner", At: "2026-09-15T00:01:00Z", ReviewCycleID: "review-001",
	})
	approvalPath := writeCLIJSON(t, fixtureDir, "approval.json", governance.Event{
		ID: "event-003", Type: governance.EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: governance.DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:02:00Z",
	})

	assertCLIExitCode(t, run([]string{"register", "--store", storeDir, "--record", recordPath}), "register record")
	staleRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", reviewPath, "--if-revision", staleRevision}), "append review with revision")
	if code := run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", approvalPath, "--if-revision", staleRevision}); code == 0 {
		t.Fatal("stale conditional append succeeded")
	}

	freshRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", approvalPath, "--if-revision", freshRevision}), "append approval with fresh revision")
}

func registeredRecord(recordID string, version int, digest string) governance.Record {
	return governance.Record{
		Schema:   governance.Schema,
		RecordID: recordID,
		Policy:   governance.DefaultReviewPolicy().Reference,
		Contract: governance.ContractReference{
			ProjectID: "document-pipeline",
			ID:        "document-pipeline",
			Version:   version,
			Schema:    "ingen.contract/v1",
			Artifact: governance.Artifact{
				URI:    "hammond/examples/document-pipeline/contract-v2.canonical.json",
				SHA256: digest,
			},
		},
		State: governance.StateRegistered,
		Events: []governance.Event{
			{ID: "event-001", Type: governance.EventRegistered, Actor: "owner", At: "2026-09-15T00:00:00Z"},
		},
	}
}

func writeCLIJSON(t *testing.T, directory, name string, value any) string {
	t.Helper()
	path := filepath.Join(directory, name)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertCLIExitCode(t *testing.T, code int, operation string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("%s exit code = %d", operation, code)
	}
}

func cliRevision(t *testing.T, storeDir, recordPath string) string {
	t.Helper()
	code, output := captureCLIOutput(t, []string{"revision", "--store", storeDir, "--record", recordPath})
	if code != 0 {
		t.Fatalf("revision exit code = %d, output = %s", code, output)
	}
	var response struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode revision output: %v", err)
	}
	if response.Revision == "" {
		t.Fatal("revision output is empty")
	}
	return response.Revision
}

func captureCLIOutput(t *testing.T, args []string) (int, string) {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := run(args)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = original
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}
