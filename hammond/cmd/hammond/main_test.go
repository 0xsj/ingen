package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
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

func TestCLIGateEmitsHashBoundApprovedContractResultWithoutMutation(t *testing.T) {
	root := t.TempDir()
	copyHammondGateFixture(t, root, "../../examples/document-pipeline/contract-v3.canonical.json", "hammond/examples/document-pipeline/contract-v3.canonical.json")
	for _, name := range []string{"record-v3.json", "event-review-opened-v3.json", "event-approved-v3.json", "review-policy-v1.json", "review-authority-v1.json"} {
		copyHammondGateFixture(t, root, "../../examples/solo/"+name, "hammond/examples/solo/"+name)
	}
	storeRoot := filepath.Join(root, "hammond-store")
	approvalPath := filepath.Join(root, "hammond", "examples", "solo", "record-v3.json")
	if code := run([]string{"register", "--store", storeRoot, "--record", approvalPath}); code != 0 {
		t.Fatalf("register fixture exit = %d", code)
	}
	for _, event := range []string{"event-review-opened-v3.json", "event-approved-v3.json"} {
		eventPath := filepath.Join(root, "hammond", "examples", "solo", event)
		if code := run([]string{"append-event", "--store", storeRoot, "--record", approvalPath, "--event", eventPath}); code != 0 {
			t.Fatalf("append fixture %s exit = %d", event, code)
		}
	}
	identityRecord, err := loadRecord(filepath.Join(root, "hammond", "examples", "solo", "record-v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	fileStore, err := store.NewFileStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	approvedRecord, err := fileStore.Get(identityRecord.Contract.Identity())
	if err != nil {
		t.Fatal(err)
	}
	approvalPath = writeCLIJSON(t, root, "approved.json", approvedRecord)
	before, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatal(err)
	}
	contractPath := "hammond/examples/document-pipeline/contract-v3.canonical.json"
	contractBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(contractPath)))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contractBytes)
	digestText := hex.EncodeToString(digest[:])
	outputDir := filepath.Join(root, "reports")
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	output := "reports/hammond-approved.json"
	args := []string{"gate", "--root", root, "--approval", "approved.json", "--review-policy", "hammond/examples/solo/review-policy-v1.json", "--contract", contractPath, "--project-id", "document-pipeline", "--contract-id", "document-pipeline", "--contract-version", "3", "--contract-schema", "ingen.contract/v1", "--contract-sha256", digestText, "--output", output}
	if code := run(args); code != 0 {
		t.Fatalf("gate exit = %d", code)
	}
	result, err := ciresult.LoadFile(filepath.Join(root, output))
	if err != nil {
		t.Fatal(err)
	}
	if result.Tool != "hammond" || result.Kind != "approved-contract" || result.Status != "passed" || result.ExitCode != 0 {
		t.Fatalf("unexpected gate result: %+v", result)
	}
	if len(result.Inputs) < 4 || result.Inputs["sorna-contract"].SHA256 != digestText {
		t.Fatalf("gate inputs do not pin expected contract: %+v", result.Inputs)
	}
	after, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("gate changed the Hammond approval record")
	}
	badArgs := append([]string(nil), args...)
	badArgs[len(badArgs)-1] = "reports/rejected.json"
	badArgs[len(badArgs)-3] = strings.Repeat("0", 64)
	if code := run(badArgs); code != 2 {
		t.Fatalf("mismatched expected contract digest exit = %d, want 2", code)
	}
	failed, err := ciresult.LoadFile(filepath.Join(root, "reports", "rejected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "error" || failed.ExitCode != 2 || failed.Error == "" {
		t.Fatalf("mismatch did not produce an error envelope: %+v", failed)
	}
	if err := os.WriteFile(filepath.Join(root, "contract-alias.canonical.json"), contractBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	aliasArgs := append([]string(nil), args...)
	aliasArgs[8] = "contract-alias.canonical.json"
	aliasArgs[len(aliasArgs)-1] = "reports/alias-rejected.json"
	if code := run(aliasArgs); code != 2 {
		t.Fatalf("same-bytes alternate contract path exit = %d, want 2", code)
	}
	aliasResult, err := ciresult.LoadFile(filepath.Join(root, "reports", "alias-rejected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if aliasResult.Status != "error" {
		t.Fatalf("alternate contract URI was accepted: %+v", aliasResult)
	}
	missingPathArgs := append([]string(nil), args...)
	missingPathArgs[4] = "reports/missing-approval.json"
	missingPathArgs[len(missingPathArgs)-1] = "reports/missing-approval.json"
	if code := run(missingPathArgs); code != 2 {
		t.Fatalf("overlapping missing approval/output exit = %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "missing-approval.json")); !os.IsNotExist(err) {
		t.Fatalf("overlap created/replaced approval path: %v", err)
	}
	aliasedDir := filepath.Join(root, "alias-governance")
	if err := os.Symlink(filepath.Join(root, "hammond", "examples", "solo"), aliasedDir); err != nil {
		t.Fatal(err)
	}
	aliasedMissingArgs := append([]string(nil), args...)
	aliasedMissingArgs[4] = "hammond/examples/solo/missing-approval.json"
	aliasedMissingArgs[len(aliasedMissingArgs)-1] = "alias-governance/missing-approval.json"
	if code := run(aliasedMissingArgs); code != 2 {
		t.Fatalf("aliased missing approval/output exit = %d, want 2", code)
	}
	if _, err := os.Lstat(filepath.Join(aliasedDir, "missing-approval.json")); !os.IsNotExist(err) {
		t.Fatalf("aliased output created/replaced selected approval path: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "hammond", "examples", "solo", "missing-approval.json")); !os.IsNotExist(err) {
		t.Fatalf("aliased output changed physical approval path: %v", err)
	}
	existingOutput := filepath.Join(root, output)
	existingBytes, err := os.ReadFile(existingOutput)
	if err != nil {
		t.Fatal(err)
	}
	protectedArgs := append([]string(nil), args...)
	protectedArgs[4] = "hammond/examples/solo/missing-approval.json"
	if code := run(protectedArgs); code != 2 {
		t.Fatalf("existing output protection exit = %d, want 2", code)
	}
	afterExisting, err := os.ReadFile(existingOutput)
	if err != nil || string(existingBytes) != string(afterExisting) {
		t.Fatalf("existing output changed after failed gate: err=%v", err)
	}
	relativeRootArgs := append([]string(nil), args...)
	relativeRootArgs[2] = "."
	relativeRootArgs[len(relativeRootArgs)-1] = "reports/relative-root.json"
	if code := run(relativeRootArgs); code != 2 {
		t.Fatalf("relative --root exit = %d, want 2", code)
	}
}

func copyHammondGateFixture(t *testing.T, root, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(destination))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, contents, 0o600); err != nil {
		t.Fatal(err)
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
