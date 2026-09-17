package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/attestation"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestCLIEndToEnd(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte(`{"schema":"ingen.ci-result/v1","status":"passed"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", "lockwood-cli-0001",
		"--media-type", "application/json",
		"--producer", "sorna",
		"--kind", "behavioral-verification",
		"--run-id", "run-0001",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	digest := record["artifact"].(map[string]any)["digest"].(string)

	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect", "--root", root, "lockwood-cli-0001"}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect exit code = %d", code)
	}
	if !strings.Contains(inspectOutput.String(), digest) {
		t.Fatalf("inspect output does not contain digest: %s", inspectOutput.String())
	}

	var recordDigestOutput bytes.Buffer
	if code := run([]string{"record-digest", "--root", root, "lockwood-cli-0001"}, strings.NewReader(""), &recordDigestOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("record-digest exit code = %d", code)
	}
	recordDigest := strings.TrimSpace(recordDigestOutput.String())
	if !strings.HasPrefix(recordDigest, "sha256:") || len(recordDigest) != len("sha256:")+64 {
		t.Fatalf("record digest = %q, want sha256 digest", recordDigest)
	}

	var findOutput bytes.Buffer
	if code := run([]string{"find", "--root", root, "--producer", "sorna"}, strings.NewReader(""), &findOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find exit code = %d", code)
	}
	if !strings.Contains(findOutput.String(), "lockwood-cli-0001") {
		t.Fatalf("find output does not contain custody ID: %s", findOutput.String())
	}
	var emptyFindOutput bytes.Buffer
	if code := run([]string{"find", "--root", root, "--producer", "does-not-exist"}, strings.NewReader(""), &emptyFindOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("empty find exit code = %d", code)
	}
	if emptyFindOutput.String() != "[]\n" {
		t.Fatalf("empty find output = %q, want deterministic empty array", emptyFindOutput.String())
	}

	var getOutput bytes.Buffer
	if code := run([]string{"get", "--root", root, digest}, strings.NewReader(""), &getOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("get exit code = %d", code)
	}
	if getOutput.String() != `{"schema":"ingen.ci-result/v1","status":"passed"}` {
		t.Fatalf("get output = %q", getOutput.String())
	}

	var verifyOutput bytes.Buffer
	if code := run([]string{"verify", "--root", root, digest}, strings.NewReader(""), &verifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify exit code = %d", code)
	}
	if strings.TrimSpace(verifyOutput.String()) != digest {
		t.Fatalf("verify output = %q", verifyOutput.String())
	}

	var custodyVerifyOutput bytes.Buffer
	if code := run([]string{"verify", "--root", root, "--id", "lockwood-cli-0001"}, strings.NewReader(""), &custodyVerifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("custody verify exit code = %d", code)
	}
	if !strings.Contains(custodyVerifyOutput.String(), "lockwood-cli-0001") {
		t.Fatalf("custody verify output = %s", custodyVerifyOutput.String())
	}

	var reconcileOutput bytes.Buffer
	if code := run([]string{"reconcile", "--root", root}, strings.NewReader(""), &reconcileOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile exit code = %d", code)
	}
	var report struct {
		Orphans            []any `json:"orphans"`
		DanglingReferences []any `json:"dangling_references"`
		CorruptBlobs       []any `json:"corrupt_blobs"`
	}
	if err := json.Unmarshal(reconcileOutput.Bytes(), &report); err != nil {
		t.Fatalf("decode reconcile output: %v", err)
	}
	if len(report.Orphans) != 0 || len(report.DanglingReferences) != 0 || len(report.CorruptBlobs) != 0 {
		t.Fatalf("reconcile report = %+v, want clean root", report)
	}
}

func TestCLIFindRejectsInvalidQueryBeforeOpeningStore(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"find", "--root", t.TempDir(), "--digest", "not-a-digest"}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code != 2 {
		t.Fatalf("invalid find exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "invalid query") || !strings.Contains(stderr.String(), "invalid artifact digest") {
		t.Fatalf("invalid find stderr = %q", stderr.String())
	}
}

func TestCLIVerifyReportContinuesAfterFailure(t *testing.T) {
	root := t.TempDir()
	goodPath := filepath.Join(t.TempDir(), "good.txt")
	badPath := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(goodPath, []byte("good report bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte("bad report bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const goodID = "lockwood-cli-verify-report-good"
	const badID = "lockwood-cli-verify-report-bad"
	missingSum := sha256.Sum256([]byte("missing verify-report parent"))
	missingDigest := "sha256:" + hex.EncodeToString(missingSum[:])
	put := func(id, path string, extra ...string) {
		t.Helper()
		args := []string{
			"put",
			"--root", root,
			"--id", id,
			"--media-type", "text/plain",
			"--producer", "example",
			"--kind", "verify-report-fixture",
		}
		args = append(args, extra...)
		args = append(args, path)
		if code := run(args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("put %s exit code = %d", id, code)
		}
	}
	put(goodID, goodPath)
	put(badID, badPath, "--parent", "references="+missingDigest)

	var reportOutput, reportErr bytes.Buffer
	if code := run([]string{
		"verify-report",
		"--root", root,
		"--producer", "example",
	}, strings.NewReader(""), &reportOutput, &reportErr); code != 1 {
		t.Fatalf("verify-report exit code = %d, want 1", code)
	}
	if reportErr.Len() != 0 {
		t.Fatalf("verify-report stderr = %q", reportErr.String())
	}
	var report custody.VerificationReport
	if err := json.Unmarshal(reportOutput.Bytes(), &report); err != nil {
		t.Fatalf("decode verify-report output: %v", err)
	}
	if report.Schema != custody.VerificationReportSchema || report.Checked != 2 || report.Verified != 1 || report.Failed != 1 || len(report.Results) != 2 {
		t.Fatalf("verify-report = %+v", report)
	}
	if report.Results[0].CustodyID != badID || report.Results[0].Status != custody.VerificationFailed {
		t.Fatalf("first verify-report result = %+v", report.Results[0])
	}
	if report.Results[1].CustodyID != goodID || report.Results[1].Status != custody.VerificationVerified {
		t.Fatalf("second verify-report result = %+v", report.Results[1])
	}
	if !strings.Contains(report.Results[0].Error, "unresolved lineage parent") {
		t.Fatalf("failed verify-report result = %+v", report.Results[0])
	}

	var filteredOutput bytes.Buffer
	if code := run([]string{"verify-report", "--root", root, "--id", goodID}, strings.NewReader(""), &filteredOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("filtered verify-report exit code = %d", code)
	}
	var filtered custody.VerificationReport
	if err := json.Unmarshal(filteredOutput.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered verify-report output: %v", err)
	}
	if filtered.Checked != 1 || filtered.Failed != 0 || len(filtered.Results) != 1 || filtered.Results[0].CustodyID != goodID {
		t.Fatalf("filtered verify-report = %+v", filtered)
	}

	var inspected custody.Record
	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect", "--root", root, badID}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect failed record exit code = %d", code)
	}
	if err := json.Unmarshal(inspectOutput.Bytes(), &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.Status != custody.Accepted {
		t.Fatalf("verify-report changed failed record status to %q", inspected.Status)
	}
}

func TestCLIGetRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(input, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var putOutput bytes.Buffer
	if code := run([]string{"put", "--root", root, "--id", "lockwood-cli-overwrite", "--media-type", "text/plain", "--producer", "example", "--kind", "fixture", input}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	digest := record["artifact"].(map[string]any)["digest"].(string)
	output := filepath.Join(t.TempDir(), "output.txt")
	if err := os.WriteFile(output, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{"get", "--root", root, "--output", output, digest}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("get overwrote an existing output")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing output changed to %q", data)
	}
}

func TestCLILineageStatusReportsUnresolvedWithoutChangingStatus(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "child.txt")
	if err := os.WriteFile(input, []byte("child"), 0o600); err != nil {
		t.Fatal(err)
	}
	missingSum := sha256.Sum256([]byte("missing-parent"))
	missingDigest := "sha256:" + hex.EncodeToString(missingSum[:])
	const custodyID = "lockwood-cli-lineage-status-unresolved"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "lineage-status-fixture",
		"--parent", "references=" + missingDigest,
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var stored custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &stored); err != nil {
		t.Fatal(err)
	}
	var statusOutput bytes.Buffer
	if code := run([]string{"lineage-status", "--root", root, custodyID}, strings.NewReader(""), &statusOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("lineage-status exit code = %d", code)
	}
	var report custody.LineageReport
	if err := json.Unmarshal(statusOutput.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != custody.LineageIncomplete || len(report.Issues) != 1 || report.Issues[0].Kind != custody.UnresolvedParentIssue {
		t.Fatalf("lineage-status report = %+v", report)
	}
	if stored.Status != custody.Accepted {
		t.Fatalf("lineage-status changed custody status to %q", stored.Status)
	}
}

func TestCLIHandlingEventsAreAppendOnly(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "handled.txt")
	if err := os.WriteFile(input, []byte("handled bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-handling-events-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "handling-events-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var original custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &original); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	originalRecordDigest, err := custody.CanonicalDigest(original)
	if err != nil {
		t.Fatal(err)
	}

	var retentionOutput bytes.Buffer
	if code := run([]string{
		"append-event",
		"--root", root,
		"--id", custodyID,
		"--event-id", "event-retention-0001",
		"--type", string(custody.RetentionClassifiedEvent),
		"--recorded-at", "2026-09-16T12:00:00Z",
		"--actor", "operator@example",
		"--reason", "classified under the standard retention schedule",
		"--retention-class", "standard-7y",
	}, strings.NewReader(""), &retentionOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("append retention event exit code = %d", code)
	}
	var retentionEvent custody.HandlingEvent
	if err := json.Unmarshal(retentionOutput.Bytes(), &retentionEvent); err != nil {
		t.Fatalf("decode retention event: %v", err)
	}
	if retentionEvent.Type != custody.RetentionClassifiedEvent || retentionEvent.RetentionClass != "standard-7y" {
		t.Fatalf("retention event = %+v", retentionEvent)
	}

	resultingDigest := "sha256:" + strings.Repeat("b", 64)
	var redactionOutput bytes.Buffer
	if code := run([]string{
		"append-event",
		"--root", root,
		"--id", custodyID,
		"--event-id", "event-redaction-0001",
		"--type", string(custody.RedactionEvent),
		"--recorded-at", "2026-09-16T12:01:00Z",
		"--actor", "operator@example",
		"--reason", "removed restricted fields",
		"--original-digest", original.Artifact.Digest,
		"--resulting-digest", resultingDigest,
	}, strings.NewReader(""), &redactionOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("append redaction event exit code = %d", code)
	}

	var listOutput bytes.Buffer
	if code := run([]string{"list-events", "--root", root, "--id", custodyID}, strings.NewReader(""), &listOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list-events exit code = %d", code)
	}
	var events []custody.HandlingEvent
	if err := json.Unmarshal(listOutput.Bytes(), &events); err != nil {
		t.Fatalf("decode handling events: %v", err)
	}
	if len(events) != 2 || events[0].EventID != "event-retention-0001" || events[1].EventID != "event-redaction-0001" {
		t.Fatalf("handling events = %+v", events)
	}
	var statusOutput bytes.Buffer
	if code := run([]string{"handling-status", "--root", root, "--id", custodyID}, strings.NewReader(""), &statusOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("handling-status exit code = %d", code)
	}
	var status custody.HandlingStatus
	if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil {
		t.Fatalf("decode handling status: %v", err)
	}
	if status.EventCount != 2 || status.RetentionClass != "standard-7y" || status.Redaction != "redacted" || len(status.Redactions) != 1 || len(status.ActiveLegalHoldIDs) != 0 {
		t.Fatalf("handling status = %+v", status)
	}
	appendEvent := func(eventArgs ...string) {
		args := []string{"append-event", "--root", root, "--id", custodyID, "--actor", "operator@example", "--reason", "legal hold state recorded"}
		args = append(args, eventArgs...)
		if code := run(args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("append legal hold event exit code = %d", code)
		}
	}
	appendEvent("--event-id", "event-hold-0001", "--type", string(custody.LegalHoldPlacedEvent), "--recorded-at", "2026-09-16T12:02:00Z", "--legal-hold-id", "hold-cli-0001")

	var heldGuardOutput bytes.Buffer
	if code := run([]string{"check-handling-guard", "--root", root, "--id", custodyID, "--action", string(custody.RedactAction)}, strings.NewReader(""), &heldGuardOutput, &bytes.Buffer{}); code == 0 {
		t.Fatal("check-handling-guard allowed redaction during legal hold")
	}
	var heldGuard custody.HandlingGuardDecision
	if err := json.Unmarshal(heldGuardOutput.Bytes(), &heldGuard); err != nil {
		t.Fatalf("decode held guard: %v", err)
	}
	if heldGuard.Status != custody.HandlingBlocked || len(heldGuard.ActiveLegalHoldIDs) != 1 || heldGuard.ActiveLegalHoldIDs[0] != "hold-cli-0001" {
		t.Fatalf("held guard = %+v", heldGuard)
	}
	appendEvent("--event-id", "event-hold-release-0001", "--type", string(custody.LegalHoldReleasedEvent), "--recorded-at", "2026-09-16T12:04:00Z", "--legal-hold-id", "hold-cli-0001")
	var clearGuardOutput bytes.Buffer
	if code := run([]string{"check-handling-guard", "--root", root, "--id", custodyID, "--action", string(custody.DeleteAction)}, strings.NewReader(""), &clearGuardOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("check-handling-guard rejected released legal hold: %d", code)
	}
	var clearGuard custody.HandlingGuardDecision
	if err := json.Unmarshal(clearGuardOutput.Bytes(), &clearGuard); err != nil {
		t.Fatalf("decode clear guard: %v", err)
	}
	if clearGuard.Status != custody.HandlingNotBlocked || len(clearGuard.ActiveLegalHoldIDs) != 0 {
		t.Fatalf("clear guard = %+v", clearGuard)
	}

	var conflictStderr bytes.Buffer
	if code := run([]string{
		"append-event",
		"--root", root,
		"--id", custodyID,
		"--event-id", "event-retention-0001",
		"--type", string(custody.RetentionClassifiedEvent),
		"--recorded-at", "2026-09-16T12:00:00Z",
		"--actor", "operator@example",
		"--reason", "changed reason",
		"--retention-class", "standard-7y",
	}, strings.NewReader(""), &bytes.Buffer{}, &conflictStderr); code == 0 {
		t.Fatal("append-event accepted conflicting event ID")
	}
	if !strings.Contains(conflictStderr.String(), "already exists with different contents") {
		t.Fatalf("conflict stderr = %q", conflictStderr.String())
	}

	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect", "--root", root, custodyID}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect exit code = %d", code)
	}
	var after custody.Record
	if err := json.Unmarshal(inspectOutput.Bytes(), &after); err != nil {
		t.Fatalf("decode inspected record: %v", err)
	}
	afterRecordDigest, err := custody.CanonicalDigest(after)
	if err != nil {
		t.Fatal(err)
	}
	if afterRecordDigest != originalRecordDigest || after.Handling != original.Handling {
		t.Fatalf("handling events changed custody record: before=%+v after=%+v", original, after)
	}
}

func TestCLIRegisterRedactionPreservesOriginal(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(t.TempDir(), "original.txt")
	if err := os.WriteFile(originalPath, []byte("original bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-register-redaction-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--received-at", "2026-09-15T12:00:00Z",
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "redaction-fixture",
		originalPath,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var original custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &original); err != nil {
		t.Fatalf("decode original record: %v", err)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	resulting, err := artifacts.Put(strings.NewReader("redacted bytes"), store.PutOptions{MediaType: "text/plain", LogicalName: "redacted.txt"})
	if err != nil {
		t.Fatal(err)
	}
	const eventID = "event-register-redaction-0001"
	args := []string{
		"register-redaction",
		"--root", root,
		"--id", custodyID,
		"--event-id", eventID,
		"--recorded-at", "2026-09-16T12:00:00Z",
		"--actor", "operator@example",
		"--reason", "removed restricted fields",
		"--original-digest", original.Artifact.Digest,
		"--resulting-digest", resulting.Digest,
	}
	var registrationOutput bytes.Buffer
	if code := run(args, strings.NewReader(""), &registrationOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("register-redaction exit code = %d", code)
	}
	var event custody.HandlingEvent
	if err := json.Unmarshal(registrationOutput.Bytes(), &event); err != nil {
		t.Fatalf("decode redaction event: %v", err)
	}
	if event.EventID != eventID || event.OriginalDigest != original.Artifact.Digest || event.ResultingDigest != resulting.Digest {
		t.Fatalf("registered event = %+v", event)
	}
	if code := run(args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("identical register-redaction was not idempotent: %d", code)
	}
	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect", "--root", root, custodyID}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect exit code = %d", code)
	}
	var after custody.Record
	if err := json.Unmarshal(inspectOutput.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Artifact.Digest != original.Artifact.Digest || after.Handling != original.Handling {
		t.Fatalf("redaction registration changed custody record: before=%+v after=%+v", original, after)
	}
	if err := artifacts.Verify(original.Artifact.Digest); err != nil {
		t.Fatalf("original artifact was not preserved: %v", err)
	}
	if err := artifacts.Verify(resulting.Digest); err != nil {
		t.Fatalf("resulting artifact is not verifiable: %v", err)
	}
	const promotedID = "lockwood-cli-promoted-redaction-0001"
	var promotionOutput bytes.Buffer
	if code := run([]string{
		"promote-redaction",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--id", promotedID,
		"--resulting-digest", resulting.Digest,
		"--size-bytes", strconv.FormatInt(resulting.SizeBytes, 10),
		"--media-type", resulting.MediaType,
		"--name", resulting.LogicalName,
		"--received-at", "2026-09-17T12:00:00Z",
		"--producer", "redactor",
		"--kind", "sanitized-export",
		"--run-id", "redaction-run-0001",
		"--source-path", "redacted.txt",
	}, strings.NewReader(""), &promotionOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("promote-redaction exit code = %d", code)
	}
	var promoted custody.Record
	if err := json.Unmarshal(promotionOutput.Bytes(), &promoted); err != nil {
		t.Fatalf("decode promoted record: %v", err)
	}
	if promoted.CustodyID != promotedID || promoted.Artifact.Digest != resulting.Digest || len(promoted.Parents) != 1 || promoted.Parents[0].Relation != custody.DerivedFrom || promoted.Parents[0].Digest != original.Artifact.Digest {
		t.Fatalf("promoted record = %+v", promoted)
	}
	if code := run([]string{
		"promote-redaction",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--id", promotedID,
		"--resulting-digest", resulting.Digest,
		"--size-bytes", strconv.FormatInt(resulting.SizeBytes, 10),
		"--media-type", resulting.MediaType,
		"--name", resulting.LogicalName,
		"--received-at", "2026-09-17T12:00:00Z",
		"--producer", "redactor",
		"--kind", "sanitized-export",
		"--run-id", "redaction-run-0001",
		"--source-path", "redacted.txt",
	}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("identical promote-redaction was not idempotent: %d", code)
	}
	if code := run([]string{"verify", "--root", root, "--id", promotedID}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify promoted record exit code = %d", code)
	}
	var redactionStatusOutput bytes.Buffer
	if code := run([]string{"redaction-status", "--root", root, "--source-id", custodyID, "--event-id", eventID}, strings.NewReader(""), &redactionStatusOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("redaction-status exit code = %d", code)
	}
	var redactionStatus custody.RedactionStatus
	if err := json.Unmarshal(redactionStatusOutput.Bytes(), &redactionStatus); err != nil {
		t.Fatalf("decode redaction status: %v", err)
	}
	if redactionStatus.Status != custody.RedactionComplete || !redactionStatus.OriginalArtifactVerified || !redactionStatus.ResultingArtifactVerified || len(redactionStatus.PromotedCustodyIDs) != 1 || redactionStatus.PromotedCustodyIDs[0] != promotedID {
		t.Fatalf("redaction status = %+v", redactionStatus)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{59}, ed25519.SeedSize))
	privateKeyPath := filepath.Join(t.TempDir(), "provenance.key")
	if err := os.WriteFile(privateKeyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var signProvenanceOutput bytes.Buffer
	if code := run([]string{
		"sign-redaction-provenance",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
		"--key-id", "provenance-key-2026-01",
		"--private-key", privateKeyPath,
	}, strings.NewReader(""), &signProvenanceOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("sign-redaction-provenance exit code = %d", code)
	}
	var provenancePublication attestation.RedactionProvenancePublication
	if err := json.Unmarshal(signProvenanceOutput.Bytes(), &provenancePublication); err != nil {
		t.Fatalf("decode redaction provenance publication: %v", err)
	}
	if provenancePublication.SourceCustodyID != custodyID || provenancePublication.EventID != eventID || provenancePublication.PromotedCustodyID != promotedID || provenancePublication.Artifact.MediaType != attestation.RedactionProvenanceMediaType {
		t.Fatalf("redaction provenance publication = %+v", provenancePublication)
	}
	provenancePath := filepath.Join(t.TempDir(), "redaction-provenance.json")
	provenanceBytes, err := attestation.MarshalCanonicalRedactionProvenance(provenancePublication.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provenancePath, provenanceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var importedProvenanceOutput bytes.Buffer
	if code := run([]string{
		"import-redaction-provenance",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
		"--expected-digest", provenancePublication.Artifact.Digest,
		provenancePath,
	}, strings.NewReader(""), &importedProvenanceOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-redaction-provenance exit code = %d", code)
	}
	var importedProvenance attestation.RedactionProvenancePublication
	if err := json.Unmarshal(importedProvenanceOutput.Bytes(), &importedProvenance); err != nil {
		t.Fatalf("decode imported redaction provenance publication: %v", err)
	}
	if importedProvenance.Artifact.Digest != provenancePublication.Artifact.Digest || importedProvenance.SourceCustodyID != custodyID || importedProvenance.PromotedCustodyID != promotedID {
		t.Fatalf("imported redaction provenance publication = %+v", importedProvenance)
	}
	var provenanceInspectionOutput bytes.Buffer
	if code := run([]string{"inspect-redaction-provenance", "--root", root, provenancePublication.Artifact.Digest}, strings.NewReader(""), &provenanceInspectionOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect-redaction-provenance exit code = %d", code)
	}
	var provenanceInspection attestation.RedactionProvenanceInspection
	if err := json.Unmarshal(provenanceInspectionOutput.Bytes(), &provenanceInspection); err != nil {
		t.Fatalf("decode redaction provenance inspection: %v", err)
	}
	if provenanceInspection.AttestationDigest != provenancePublication.Artifact.Digest || provenanceInspection.Envelope.Target != provenancePublication.Envelope.Target {
		t.Fatalf("redaction provenance inspection = %+v", provenanceInspection)
	}
	var provenanceLinkOutput bytes.Buffer
	if code := run([]string{
		"inspect-redaction-provenance-link",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
		provenancePublication.Artifact.Digest,
	}, strings.NewReader(""), &provenanceLinkOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect-redaction-provenance-link exit code = %d", code)
	}
	var provenanceLink attestation.RedactionProvenanceLinkInspection
	if err := json.Unmarshal(provenanceLinkOutput.Bytes(), &provenanceLink); err != nil {
		t.Fatalf("decode redaction provenance link inspection: %v", err)
	}
	if provenanceLink.Relation != attestation.RedactionProvenanceLinkRelation || provenanceLink.AttestationDigest != provenancePublication.Artifact.Digest || provenanceLink.SourceCustodyID != custodyID || provenanceLink.EventID != eventID || provenanceLink.PromotedCustodyID != promotedID {
		t.Fatalf("redaction provenance link inspection = %+v", provenanceLink)
	}
	publicKeyPath := filepath.Join(t.TempDir(), "provenance.pub")
	if err := os.WriteFile(publicKeyPath, []byte(base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var verifyProvenanceOutput bytes.Buffer
	if code := run([]string{
		"verify-redaction-provenance",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
		"--public-key", publicKeyPath,
		provenancePublication.Artifact.Digest,
	}, strings.NewReader(""), &verifyProvenanceOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-redaction-provenance exit code = %d", code)
	}
	var provenanceVerification attestation.RedactionProvenanceVerificationReceipt
	if err := json.Unmarshal(verifyProvenanceOutput.Bytes(), &provenanceVerification); err != nil {
		t.Fatalf("decode redaction provenance verification: %v", err)
	}
	if !provenanceVerification.Verified || provenanceVerification.SourceCustodyID != custodyID || provenanceVerification.PromotedCustodyID != promotedID || provenanceVerification.AttestationDigest != provenancePublication.Artifact.Digest {
		t.Fatalf("redaction provenance verification = %+v", provenanceVerification)
	}
	registry := attestation.TrustRegistry{
		Schema: attestation.TrustRegistrySchema,
		Keys: []attestation.TrustedKey{{
			KeyID:     "provenance-key-2026-01",
			Algorithm: attestation.Algorithm,
			PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			Status:    attestation.KeyActive,
		}},
	}
	registryBytes, err := attestation.MarshalCanonicalTrustRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(t.TempDir(), "provenance-trust.json")
	if err := os.WriteFile(registryPath, registryBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var trustedProvenanceOutput bytes.Buffer
	if code := run([]string{
		"verify-redaction-provenance-trusted",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
		"--registry", registryPath,
		"--at", "2026-09-17T13:00:00Z",
		provenancePublication.Artifact.Digest,
	}, strings.NewReader(""), &trustedProvenanceOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-redaction-provenance-trusted exit code = %d", code)
	}
	var trustedProvenance attestation.TrustedRedactionProvenanceVerificationReceipt
	if err := json.Unmarshal(trustedProvenanceOutput.Bytes(), &trustedProvenance); err != nil {
		t.Fatalf("decode trusted redaction provenance verification: %v", err)
	}
	if !trustedProvenance.Verified || !trustedProvenance.Trusted || trustedProvenance.RegistryDigest == "" || trustedProvenance.EvaluatedAt != "2026-09-17T13:00:00Z" {
		t.Fatalf("trusted redaction provenance verification = %+v", trustedProvenance)
	}
	var findProvenanceOutput bytes.Buffer
	if code := run([]string{
		"find-redaction-provenance",
		"--root", root,
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
		"--key-id", "provenance-key-2026-01",
	}, strings.NewReader(""), &findProvenanceOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find-redaction-provenance exit code = %d", code)
	}
	var foundProvenance []attestation.StoredRedactionProvenance
	if err := json.Unmarshal(findProvenanceOutput.Bytes(), &foundProvenance); err != nil {
		t.Fatalf("decode redaction provenance inventory: %v", err)
	}
	if len(foundProvenance) != 1 || foundProvenance[0].Artifact.Digest != provenancePublication.Artifact.Digest || foundProvenance[0].Envelope.KeyID != "provenance-key-2026-01" {
		t.Fatalf("redaction provenance inventory = %+v", foundProvenance)
	}
	var trustedFindProvenanceOutput bytes.Buffer
	if code := run([]string{
		"find-trusted-redaction-provenance",
		"--root", root,
		"--registry", registryPath,
		"--at", "2026-09-17T13:00:00Z",
		"--source-id", custodyID,
		"--event-id", eventID,
		"--promoted-id", promotedID,
	}, strings.NewReader(""), &trustedFindProvenanceOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find-trusted-redaction-provenance exit code = %d", code)
	}
	var trustedFoundProvenance []attestation.TrustedStoredRedactionProvenance
	if err := json.Unmarshal(trustedFindProvenanceOutput.Bytes(), &trustedFoundProvenance); err != nil {
		t.Fatalf("decode trusted redaction provenance inventory: %v", err)
	}
	if len(trustedFoundProvenance) != 1 || !trustedFoundProvenance[0].Verification.Trusted || trustedFoundProvenance[0].Artifact.Digest != provenancePublication.Artifact.Digest {
		t.Fatalf("trusted redaction provenance inventory = %+v", trustedFoundProvenance)
	}
	var reconcileOutput bytes.Buffer
	if code := run([]string{"reconcile", "--root", root}, strings.NewReader(""), &reconcileOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile after provenance publication exit code = %d", code)
	}
	var reconcileReport struct {
		Orphans []any `json:"orphans"`
	}
	if err := json.Unmarshal(reconcileOutput.Bytes(), &reconcileReport); err != nil {
		t.Fatal(err)
	}
	if len(reconcileReport.Orphans) != 0 {
		t.Fatalf("redaction provenance was classified as an orphan: %+v", reconcileReport.Orphans)
	}
}

func TestCLIHandlingEventAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "authenticated-handling.txt")
	if err := os.WriteFile(input, []byte("authenticated handling bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-authenticated-event-0001"
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "authenticated-handling-fixture",
		input,
	}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	const eventID = "event-authenticated-0001"
	if code := run([]string{
		"append-event",
		"--root", root,
		"--id", custodyID,
		"--event-id", eventID,
		"--type", string(custody.RetentionClassifiedEvent),
		"--recorded-at", "2026-09-16T12:00:00Z",
		"--actor", "operator@example",
		"--reason", "authenticated handling decision",
		"--retention-class", "regulated-7y",
	}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("append-event exit code = %d", code)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{46}, ed25519.SeedSize))
	privateKeyPath := filepath.Join(t.TempDir(), "handling-key.key")
	if err := os.WriteFile(privateKeyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var signOutput bytes.Buffer
	if code := run([]string{
		"sign-handling-event",
		"--root", root,
		"--id", custodyID,
		"--event-id", eventID,
		"--key-id", "handling-key-2026-01",
		"--private-key", privateKeyPath,
	}, strings.NewReader(""), &signOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("sign-handling-event exit code = %d", code)
	}
	var publication attestation.HandlingEventPublication
	if err := json.Unmarshal(signOutput.Bytes(), &publication); err != nil {
		t.Fatalf("decode handling event publication: %v", err)
	}
	if publication.CustodyID != custodyID || publication.EventID != eventID || publication.Artifact.MediaType != attestation.HandlingEventMediaType {
		t.Fatalf("handling event publication = %+v", publication)
	}
	publicKeyPath := filepath.Join(t.TempDir(), "handling-key.pub")
	if err := os.WriteFile(publicKeyPath, []byte(base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var verifyOutput bytes.Buffer
	if code := run([]string{
		"verify-handling-event",
		"--root", root,
		"--id", custodyID,
		"--event-id", eventID,
		"--public-key", publicKeyPath,
		publication.Artifact.Digest,
	}, strings.NewReader(""), &verifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-handling-event exit code = %d", code)
	}
	var verification attestation.HandlingEventVerificationReceipt
	if err := json.Unmarshal(verifyOutput.Bytes(), &verification); err != nil {
		t.Fatalf("decode handling event verification: %v", err)
	}
	if !verification.Verified || verification.CustodyID != custodyID || verification.EventID != eventID || verification.AttestationDigest != publication.Artifact.Digest {
		t.Fatalf("handling event verification = %+v", verification)
	}
	registry := attestation.TrustRegistry{
		Schema: attestation.TrustRegistrySchema,
		Keys: []attestation.TrustedKey{{
			KeyID:     "handling-key-2026-01",
			Algorithm: attestation.Algorithm,
			PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			Status:    attestation.KeyActive,
		}},
	}
	registryBytes, err := attestation.MarshalCanonicalTrustRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(t.TempDir(), "handling-trust.json")
	if err := os.WriteFile(registryPath, registryBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var trustedOutput bytes.Buffer
	if code := run([]string{
		"verify-handling-event-trusted",
		"--root", root,
		"--id", custodyID,
		"--event-id", eventID,
		"--registry", registryPath,
		"--at", "2026-09-16T12:01:00Z",
		publication.Artifact.Digest,
	}, strings.NewReader(""), &trustedOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-handling-event-trusted exit code = %d", code)
	}
	var trusted attestation.TrustedHandlingEventVerificationReceipt
	if err := json.Unmarshal(trustedOutput.Bytes(), &trusted); err != nil {
		t.Fatalf("decode trusted handling event verification: %v", err)
	}
	if !trusted.Verified || !trusted.Trusted || trusted.RegistryDigest == "" || trusted.EvaluatedAt != "2026-09-16T12:01:00Z" {
		t.Fatalf("trusted handling event verification = %+v", trusted)
	}
	policy := attestation.HandlingAuthorizationPolicy{
		Schema: attestation.HandlingAuthorizationPolicySchema,
		Rules: []attestation.HandlingAuthorizationRule{{
			KeyID:      "handling-key-2026-01",
			EventTypes: []custody.HandlingEventType{custody.RetentionClassifiedEvent},
		}},
	}
	policyBytes, err := attestation.MarshalCanonicalHandlingAuthorizationPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(t.TempDir(), "handling-policy.json")
	if err := os.WriteFile(policyPath, policyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var authorizedOutput bytes.Buffer
	if code := run([]string{
		"verify-handling-event-authorized",
		"--root", root,
		"--id", custodyID,
		"--event-id", eventID,
		"--registry", registryPath,
		"--policy", policyPath,
		"--at", "2026-09-16T12:01:00Z",
		publication.Artifact.Digest,
	}, strings.NewReader(""), &authorizedOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-handling-event-authorized exit code = %d", code)
	}
	var authorized attestation.AuthorizedHandlingEventVerificationReceipt
	if err := json.Unmarshal(authorizedOutput.Bytes(), &authorized); err != nil {
		t.Fatalf("decode authorized handling event verification: %v", err)
	}
	if !authorized.Verified || !authorized.Trusted || !authorized.Authorized || authorized.PolicyDigest == "" {
		t.Fatalf("authorized handling event verification = %+v", authorized)
	}
	var reconcileOutput bytes.Buffer
	if code := run([]string{"reconcile", "--root", root}, strings.NewReader(""), &reconcileOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile exit code = %d", code)
	}
	var report struct {
		Orphans []any `json:"orphans"`
	}
	if err := json.Unmarshal(reconcileOutput.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Orphans) != 0 {
		t.Fatalf("signed handling event was classified as an orphan: %+v", report.Orphans)
	}
}

func TestCLIReconcileClassifiesOldOrphansWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := artifacts.Put(strings.NewReader("orphan"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	hexDigest := strings.TrimPrefix(orphan.Digest, "sha256:")
	blobPath := filepath.Join(root, "blobs", "sha256", hexDigest[:2], hexDigest[2:4], hexDigest)
	oldTime := "2026-09-10T12:00:00Z"
	parsedOldTime, err := time.Parse(time.RFC3339, oldTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(blobPath, parsedOldTime, parsedOldTime); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"reconcile",
		"--root", root,
		"--orphan-grace", "24h",
		"--as-of", "2026-09-16T12:00:00Z",
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile exit code = %d", code)
	}
	var report struct {
		Orphans           []any `json:"orphans"`
		CleanupCandidates []struct {
			Digest string `json:"digest"`
		} `json:"cleanup_candidates"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Orphans) != 1 || len(report.CleanupCandidates) != 1 || report.CleanupCandidates[0].Digest != orphan.Digest {
		t.Fatalf("reconciliation output = %+v", report)
	}
	if err := artifacts.Verify(orphan.Digest); err != nil {
		t.Fatalf("reconcile removed or damaged orphan: %v", err)
	}
}

func TestCLICleanupPlanIsReadOnlyAndNeverAuthorizesDeletion(t *testing.T) {
	root := t.TempDir()
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	oldOrphan, err := artifacts.Put(strings.NewReader("old orphan"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	recentOrphan, err := artifacts.Put(strings.NewReader("recent orphan"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	oldTime, err := time.Parse(time.RFC3339, "2026-09-10T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	oldHex := strings.TrimPrefix(oldOrphan.Digest, "sha256:")
	oldPath := filepath.Join(root, "blobs", "sha256", oldHex[:2], oldHex[2:4], oldHex)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"cleanup-plan",
		"--root", root,
		"--orphan-grace", "24h",
		"--as-of", "2026-09-16T12:00:00Z",
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("cleanup-plan exit code = %d", code)
	}
	var plan custody.CleanupPlan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Schema != custody.CleanupPlanSchema || plan.OrphanGrace != "24h0m0s" || len(plan.Entries) != 2 {
		t.Fatalf("cleanup plan = %+v", plan)
	}
	entries := make(map[string]custody.CleanupPlanEntry, len(plan.Entries))
	for _, entry := range plan.Entries {
		entries[entry.Digest] = entry
		if entry.ActionStatus != custody.CleanupNotAuthorized || len(entry.Blockers) == 0 {
			t.Fatalf("cleanup entry authorizes deletion: %+v", entry)
		}
	}
	if entries[oldOrphan.Digest].State != custody.CleanupCandidateState || entries[recentOrphan.Digest].State != custody.ReportedOrphanState {
		t.Fatalf("cleanup states = %+v", entries)
	}
	if err := artifacts.Verify(oldOrphan.Digest); err != nil {
		t.Fatalf("cleanup-plan damaged old orphan: %v", err)
	}
	if err := artifacts.Verify(recentOrphan.Digest); err != nil {
		t.Fatalf("cleanup-plan damaged recent orphan: %v", err)
	}
}

func TestCLIPutRemoteV2Source(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "remote-result.json")
	if err := os.WriteFile(input, []byte("remote bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--schema", "lockwood.custody/v2",
		"--id", "lockwood-cli-remote-v2",
		"--media-type", "application/json",
		"--producer", "sorna",
		"--kind", "remote-result",
		"--source-uri", "s3://evidence.example/runs/run-0001/result.json",
		"--source-version", "version-0001",
		input,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put remote v2 exit code = %d", code)
	}
	var record struct {
		Schema string `json:"schema"`
		Source struct {
			URI     string `json:"uri"`
			Version string `json:"version"`
		} `json:"source"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Schema != "lockwood.custody/v2" || record.Source.URI == "" || record.Source.Version == "" {
		t.Fatalf("remote v2 record = %+v", record)
	}
}

func TestCLIPutRejectsOversizedInput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(input, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", "lockwood-cli-oversized",
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "fixture",
		"--max-bytes", "3",
		input,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("put accepted an oversized input")
	}
	if !strings.Contains(stderr.String(), "exceeds maximum size") {
		t.Fatalf("stderr = %q, want maximum-size rejection", stderr.String())
	}
}

func TestCLIImportSorna(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "sorna-run")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"manifest.json": []byte("{\"schema\":\"sorna.evidence/v1\",\"run_id\":\"run-cli-0001\"}\n"),
		"run.json":      []byte("{\"schema\":\"ingen.run/v1\",\"run_id\":\"run-cli-0001\"}\n"),
	}
	checksums := make([]string, 0, len(files))
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(bundle, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		checksums = append(checksums, hex.EncodeToString(digest[:])+"  "+name)
	}
	sort.Strings(checksums)
	if err := os.WriteFile(filepath.Join(bundle, "checksums.sha256"), []byte(strings.Join(checksums, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := run([]string{"import-sorna", "--root", root, "--id", "lockwood-cli-sorna-0001", bundle}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-sorna exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["producer"].(map[string]any)["kind"] != "evidence-bundle" {
		t.Fatalf("producer = %+v", record["producer"])
	}
}

func TestCLIImportCIResult(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "ci-result.json")
	contents := []byte(`{"schema":"ingen.ci-result/v1","tool":"paddock","kind":"architecture","status":"passed","exit_code":0,"created_at":"2026-09-15T12:00:00Z","source":{"root":"/workspace/service"},"report":{},"explanation":{}}`)
	if err := os.WriteFile(input, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{"import-ci-result", "--root", root, "--id", "lockwood-cli-ci-result-0001", input}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-ci-result exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["status"] != "accepted" || record["producer"].(map[string]any)["tool"] != "paddock" {
		t.Fatalf("imported custody record = %+v", record)
	}
	if record["artifact"].(map[string]any)["media_type"] != "application/vnd.ingen.ci-result+json" {
		t.Fatalf("imported media type = %+v", record["artifact"])
	}
}

func TestCLIVerifyAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "attested.txt")
	if err := os.WriteFile(input, []byte("attested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-attestation-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "attestation-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{18}, ed25519.SeedSize))
	envelope, err := attestation.Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := attestation.Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	targetDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	var findOutput bytes.Buffer
	if code := run([]string{"find-attestation", "--root", root, "--target-digest", targetDigest}, strings.NewReader(""), &findOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find-attestation exit code = %d", code)
	}
	var found []attestation.StoredAttestation
	if err := json.Unmarshal(findOutput.Bytes(), &found); err != nil {
		t.Fatalf("decode attestation find output: %v", err)
	}
	if len(found) != 1 || found[0].Artifact.Digest != ref.Digest || found[0].Envelope.KeyID != "review-key-2026-01" {
		t.Fatalf("attestation find results = %+v", found)
	}
	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect-attestation", "--root", root, ref.Digest}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect-attestation exit code = %d", code)
	}
	var inspection attestation.Inspection
	if err := json.Unmarshal(inspectOutput.Bytes(), &inspection); err != nil {
		t.Fatalf("decode attestation inspection: %v", err)
	}
	if inspection.AttestationDigest != ref.Digest || inspection.Envelope.KeyID != "review-key-2026-01" {
		t.Fatalf("attestation inspection = %+v", inspection)
	}
	var linkOutput bytes.Buffer
	if code := run([]string{
		"inspect-attestation-link",
		"--root", root,
		"--id", custodyID,
		ref.Digest,
	}, strings.NewReader(""), &linkOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect-attestation-link exit code = %d", code)
	}
	var link attestation.LinkInspection
	if err := json.Unmarshal(linkOutput.Bytes(), &link); err != nil {
		t.Fatalf("decode attestation link: %v", err)
	}
	if link.Relation != attestation.CustodyRecordLinkRelation || link.AttestationDigest != ref.Digest || link.CustodyID != custodyID || link.CustodyRecordDigest != targetDigest || link.PayloadArtifactDigest != record.Artifact.Digest {
		t.Fatalf("attestation link = %+v", link)
	}
	publicKeyPath := filepath.Join(t.TempDir(), "review-key.pub")
	encodedKey := base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)) + "\n"
	if err := os.WriteFile(publicKeyPath, []byte(encodedKey), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"verify-attestation",
		"--root", root,
		"--id", custodyID,
		"--public-key", publicKeyPath,
		ref.Digest,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-attestation exit code = %d", code)
	}
	var receipt attestation.VerificationReceipt
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
		t.Fatalf("decode verification receipt: %v", err)
	}
	if receipt.CustodyID != custodyID || receipt.AttestationDigest != ref.Digest || receipt.KeyID != "review-key-2026-01" || receipt.Algorithm != attestation.Algorithm || !receipt.Verified {
		t.Fatalf("verification receipt = %+v", receipt)
	}
	registry := attestation.TrustRegistry{
		Schema: attestation.TrustRegistrySchema,
		Keys: []attestation.TrustedKey{{
			KeyID:     "review-key-2026-01",
			Algorithm: attestation.Algorithm,
			PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			Status:    attestation.KeyActive,
		}},
	}
	registryBytes, err := attestation.MarshalCanonicalTrustRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(t.TempDir(), "attestation-trust.json")
	if err := os.WriteFile(registryPath, registryBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var trustedOutput bytes.Buffer
	if code := run([]string{
		"verify-attestation-trusted",
		"--root", root,
		"--id", custodyID,
		"--registry", registryPath,
		"--at", "2026-09-16T12:00:01Z",
		ref.Digest,
	}, strings.NewReader(""), &trustedOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-attestation-trusted exit code = %d", code)
	}
	var trustedReceipt attestation.TrustedVerificationReceipt
	if err := json.Unmarshal(trustedOutput.Bytes(), &trustedReceipt); err != nil {
		t.Fatalf("decode trusted verification receipt: %v", err)
	}
	if !trustedReceipt.Verified || !trustedReceipt.Trusted || trustedReceipt.RegistryDigest == "" || trustedReceipt.EvaluatedAt != "2026-09-16T12:00:01Z" {
		t.Fatalf("trusted verification receipt = %+v", trustedReceipt)
	}
	var trustedFindOutput bytes.Buffer
	if code := run([]string{
		"find-trusted-attestation",
		"--root", root,
		"--registry", registryPath,
		"--at", "2026-09-16T12:00:01Z",
		"--target-digest", targetDigest,
	}, strings.NewReader(""), &trustedFindOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find-trusted-attestation exit code = %d", code)
	}
	var trustedFound []attestation.TrustedStoredAttestation
	if err := json.Unmarshal(trustedFindOutput.Bytes(), &trustedFound); err != nil {
		t.Fatalf("decode trusted attestation inventory: %v", err)
	}
	if len(trustedFound) != 1 || !trustedFound[0].Verification.Trusted || trustedFound[0].Artifact.Digest != ref.Digest {
		t.Fatalf("trusted attestation inventory = %+v", trustedFound)
	}
	var reconcileOutput bytes.Buffer
	if code := run([]string{"reconcile", "--root", root}, strings.NewReader(""), &reconcileOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile after attestation exit code = %d", code)
	}
	var reconcileReport struct {
		Orphans []struct {
			Digest string `json:"digest"`
		} `json:"orphans"`
	}
	if err := json.Unmarshal(reconcileOutput.Bytes(), &reconcileReport); err != nil {
		t.Fatalf("decode reconcile after attestation: %v", err)
	}
	if len(reconcileReport.Orphans) != 0 {
		t.Fatalf("reconcile misclassified detached attestation as orphan: %+v", reconcileReport.Orphans)
	}

	w := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{19}, ed25519.SeedSize))
	if err := os.WriteFile(publicKeyPath, []byte(base64.StdEncoding.EncodeToString(w.Public().(ed25519.PublicKey))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{
		"verify-attestation",
		"--root", root,
		"--id", custodyID,
		"--public-key", publicKeyPath,
		ref.Digest,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("verify-attestation accepted a wrong public key")
	}
	if !strings.Contains(stderr.String(), "verification failed") {
		t.Fatalf("wrong-key stderr = %q", stderr.String())
	}
}

func TestCLISignAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "attested.txt")
	if err := os.WriteFile(input, []byte("attested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-sign-attestation-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "attestation-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{20}, ed25519.SeedSize))
	privateKeyPath := filepath.Join(t.TempDir(), "review-key.key")
	if err := os.WriteFile(privateKeyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var signOutput bytes.Buffer
	if code := run([]string{
		"sign-attestation",
		"--root", root,
		"--id", custodyID,
		"--key-id", "review-key-2026-01",
		"--private-key", privateKeyPath,
	}, strings.NewReader(""), &signOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("sign-attestation exit code = %d", code)
	}
	var result struct {
		CustodyID string `json:"custody_id"`
		Artifact  struct {
			Digest    string `json:"digest"`
			MediaType string `json:"media_type"`
		} `json:"artifact"`
		Envelope attestation.Envelope `json:"envelope"`
	}
	if err := json.Unmarshal(signOutput.Bytes(), &result); err != nil {
		t.Fatalf("decode sign-attestation output: %v", err)
	}
	if result.CustodyID != custodyID || result.Artifact.Digest == "" || result.Artifact.MediaType != attestation.MediaType {
		t.Fatalf("published artifact = %+v", result.Artifact)
	}
	if result.Envelope.KeyID != "review-key-2026-01" {
		t.Fatalf("published envelope = %+v", result.Envelope)
	}
	recordDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Target.Digest != recordDigest {
		t.Fatalf("target digest = %s, want %s", result.Envelope.Target.Digest, recordDigest)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	keys := attestation.PublicKeySet{"review-key-2026-01": privateKey.Public().(ed25519.PublicKey)}
	if err := attestation.VerifyPublished(record, result.Artifact.Digest, artifacts, keys); err != nil {
		t.Fatalf("published CLI attestation failed verification: %v", err)
	}

	if err := os.Chmod(privateKeyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{
		"sign-attestation",
		"--root", root,
		"--id", custodyID,
		"--key-id", "review-key-2026-01",
		"--private-key", privateKeyPath,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("sign-attestation accepted a private key file with open permissions")
	}
	if !strings.Contains(stderr.String(), "permissions are too open") {
		t.Fatalf("open-permission stderr = %q", stderr.String())
	}
}

func TestCLIImportAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "attested.txt")
	if err := os.WriteFile(input, []byte("attested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-import-attestation-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "attestation-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{22}, ed25519.SeedSize))
	envelope, err := attestation.Sign(record, "external-review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	envelopePath := filepath.Join(t.TempDir(), "attestation.json")
	envelopeBytes, err := attestation.MarshalCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envelopePath, envelopeBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	envelopeDigest, err := attestation.CanonicalDigest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"import-attestation",
		"--root", root,
		"--id", custodyID,
		"--expected-digest", envelopeDigest,
		envelopePath,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-attestation exit code = %d", code)
	}
	var publication attestation.Publication
	if err := json.Unmarshal(output.Bytes(), &publication); err != nil {
		t.Fatalf("decode import-attestation output: %v", err)
	}
	if publication.CustodyID != custodyID || publication.Artifact.Digest != envelopeDigest {
		t.Fatalf("import publication = %+v", publication)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	keys := attestation.PublicKeySet{"external-review-key-2026-01": privateKey.Public().(ed25519.PublicKey)}
	if err := attestation.VerifyPublished(record, publication.Artifact.Digest, artifacts, keys); err != nil {
		t.Fatalf("imported attestation failed verification: %v", err)
	}

	var stderr bytes.Buffer
	if code := run([]string{
		"import-attestation",
		"--root", root,
		"--id", custodyID,
		"--expected-digest", "sha256:" + strings.Repeat("0", 64),
		envelopePath,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("import-attestation accepted a mismatched expected digest")
	}
	if !strings.Contains(stderr.String(), "expected digest") {
		t.Fatalf("mismatched-digest stderr = %q", stderr.String())
	}
}

func TestCLIRecover(t *testing.T) {
	root := t.TempDir()
	seedPath := filepath.Join(t.TempDir(), "seed.txt")
	orphanPath := filepath.Join(t.TempDir(), "orphan.txt")
	if err := os.WriteFile(seedPath, []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-recover-0001"
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "seed",
		seedPath,
	}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed put exit code = %d", code)
	}

	pendingPath := filepath.Join(t.TempDir(), "pending.json")
	var putStderr bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "recovered",
		"--pending-record", pendingPath,
		orphanPath,
	}, strings.NewReader(""), &bytes.Buffer{}, &putStderr); code == 0 {
		t.Fatal("conflicting put unexpectedly succeeded")
	}
	if !strings.Contains(putStderr.String(), "pending record") {
		t.Fatalf("put stderr = %q, want pending-record confirmation", putStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "records", custodyID+".json")); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := run([]string{"recover", "--root", root, pendingPath}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("recover exit code = %d", code)
	}
	if !strings.Contains(output.String(), `"custody_id": "`+custodyID+`"`) {
		t.Fatalf("recover output = %s", output.String())
	}
	if code := run([]string{"verify", "--root", root, "--id", custodyID}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("recovered custody record failed verification")
	}
}
