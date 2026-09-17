package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/hammond/internal/governance"
	"ingen/hammond/internal/store"
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

func TestCLISoloAuthorityWorkflow(t *testing.T) {
	storeDir := t.TempDir()
	recordPath := "../../examples/solo/record-v2.json"
	reviewPath := "../../examples/solo/event-review-opened.json"
	approvalPath := "../../examples/solo/event-approved.json"

	assertCLIExitCode(t, run([]string{"register", "--store", storeDir, "--record", recordPath}), "register solo record")
	registerRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", reviewPath, "--if-revision", registerRevision}), "open solo review")
	reviewRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", approvalPath, "--if-revision", reviewRevision}), "approve solo record")

	code, output := captureCLIOutput(t, []string{"show", "--store", storeDir, "--record", recordPath})
	if code != 0 {
		t.Fatalf("solo show exit code = %d, output = %s", code, output)
	}
	var record governance.Record
	if err := json.Unmarshal([]byte(output), &record); err != nil {
		t.Fatalf("decode solo record: %v", err)
	}
	if record.State != governance.StateApproved {
		t.Fatalf("solo record state = %q, want approved", record.State)
	}

	code, output = captureCLIOutput(t, []string{"lineage", "--store", storeDir})
	if code != 0 {
		t.Fatalf("solo lineage exit code = %d, output = %s", code, output)
	}
	if !strings.Contains(output, "document-pipeline/document-pipeline@2 state=approved") {
		t.Fatalf("solo lineage output = %q, want approved record", output)
	}
}

func TestCLISoloAuthorityAmendmentAndSupersession(t *testing.T) {
	storeDir := t.TempDir()
	fixtureDir := t.TempDir()
	recordPath := "../../examples/solo/record-v2.json"
	reviewPath := "../../examples/solo/event-review-opened.json"
	approvalPath := "../../examples/solo/event-approved.json"

	assertCLIExitCode(t, run([]string{"register", "--store", storeDir, "--record", recordPath}), "register solo predecessor")
	registerRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", reviewPath, "--if-revision", registerRevision}), "open solo predecessor review")
	reviewRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", recordPath, "--event", approvalPath, "--if-revision", reviewRevision}), "approve solo predecessor")

	predecessor, err := loadRecord(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	successor := predecessor
	successor.RecordID = "document-pipeline-v3-solo"
	successor.Contract.Version = 3
	successor.State = governance.StateRegistered
	successor.Events = []governance.Event{{
		ID: "solo-event-101", Type: governance.EventRegistered, Actor: "solo", At: "2026-09-15T00:03:00Z",
	}}
	successorPath := writeCLIJSON(t, fixtureDir, "successor.json", successor)

	predecessorRevision := cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{
		"amend", "--store", storeDir, "--record", recordPath, "--successor", successorPath,
		"--event-id", "solo-event-102", "--actor", "solo", "--at", "2026-09-15T00:04:00Z",
		"--kind", "clarifying", "--reason", "Clarify the document processing description.", "--if-revision", predecessorRevision,
	}), "create solo amendment")

	successorReviewPath := writeCLIJSON(t, fixtureDir, "successor-review.json", governance.Event{
		ID: "solo-event-103", Type: governance.EventReviewOpened, Actor: "solo", At: "2026-09-15T00:05:00Z", ReviewCycleID: "document-pipeline-v3-solo-review-1",
	})
	successorApprovalPath := writeCLIJSON(t, fixtureDir, "successor-approval.json", governance.Event{
		ID: "solo-event-104", Type: governance.EventApprovalRecorded, Actor: "solo", Role: "product-reviewer", ReviewCycleID: "document-pipeline-v3-solo-review-1", Decision: governance.DecisionApprove, ArtifactSHA256: successor.Contract.Artifact.SHA256, At: "2026-09-15T00:06:00Z",
	})

	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", successorPath, "--event", successorReviewPath}), "open solo successor review")
	successorRevision := cliRevision(t, storeDir, successorPath)
	assertCLIExitCode(t, run([]string{"append-event", "--store", storeDir, "--record", successorPath, "--event", successorApprovalPath, "--if-revision", successorRevision}), "approve solo successor")

	predecessorRevision = cliRevision(t, storeDir, recordPath)
	assertCLIExitCode(t, run([]string{
		"supersede", "--store", storeDir, "--record", recordPath, "--successor", successorPath,
		"--event-id", "solo-event-105", "--actor", "solo", "--at", "2026-09-15T00:07:00Z", "--if-revision", predecessorRevision,
	}), "supersede solo predecessor")

	code, output := captureCLIOutput(t, []string{"lineage", "--store", storeDir})
	if code != 0 {
		t.Fatalf("solo amendment lineage exit code = %d, output = %s", code, output)
	}
	if !strings.Contains(output, "document-pipeline/document-pipeline@2 state=superseded") || !strings.Contains(output, "document-pipeline/document-pipeline@3 state=approved") {
		t.Fatalf("solo amendment lineage output = %q, want superseded predecessor and approved successor", output)
	}
}

func TestCLIValidateChecksSoloArtifactsBeforeRegistration(t *testing.T) {
	code, output := captureCLIOutput(t, []string{"validate", "--record", "../../examples/solo/record-v2.json"})
	if code != 0 {
		t.Fatalf("validate exit code = %d, output = %s", code, output)
	}
	var response struct {
		Valid bool             `json:"valid"`
		State governance.State `json:"state"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode validate output: %v", err)
	}
	if !response.Valid || response.State != governance.StateRegistered {
		t.Fatalf("validate response = %#v, want valid registered record", response)
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

func TestCLIMembershipCurrentReadsAcceptedReference(t *testing.T) {
	storeDir := t.TempDir()
	versionStore, err := store.NewFileMembershipVersionStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := governance.MembershipSnapshot{
		Reference: governance.MembershipReference{
			ID:      "directory-reviewers",
			Version: 4,
			Schema:  governance.MembershipSchema,
			Artifact: governance.Artifact{
				URI:    "membership.json",
				SHA256: strings.Repeat("a", 64),
			},
		},
		IssuedAt: "2026-09-15T00:00:00Z",
		Grants:   []governance.AuthorityGrant{},
	}
	if err := versionStore.Accept(snapshot); err != nil {
		t.Fatal(err)
	}
	code, output := captureCLIOutput(t, []string{"membership-current", "--store", storeDir, "--id", "directory-reviewers"})
	if code != 0 {
		t.Fatalf("membership-current exit code = %d, output = %s", code, output)
	}
	var reference governance.MembershipReference
	if err := json.Unmarshal([]byte(output), &reference); err != nil {
		t.Fatalf("decode membership-current output: %v", err)
	}
	if !reference.Equal(snapshot.Reference) {
		t.Fatalf("membership-current reference = %#v, want %#v", reference, snapshot.Reference)
	}
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
