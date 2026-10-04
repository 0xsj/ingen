package nativesession

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/run"
)

func TestAssessHeldAndMissingLeaseAreReadOnlyObservations(t *testing.T) {
	root, _, record, _ := spawnFixture(t, []string{"/usr/bin/true"})
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, journalPath); err != nil {
		t.Fatal(err)
	}
	lease, acquired, err := nativejournal.TryExecutionLease(root, record.Intent.ExecutionLeasePath)
	if err != nil || !acquired {
		t.Fatalf("prepare held lease: acquired=%v err=%v", acquired, err)
	}
	before := assessmentTree(t, root)
	assessment, err := Assess(root, journalPath, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if assessment.ExecutionLease.Status != "held" || assessment.Status != AssessmentAssessed || assessment.Receipt.Status != "matched" {
		t.Fatalf("held assessment = lease %q status %q receipt %q", assessment.ExecutionLease.Status, assessment.Status, assessment.Receipt.Status)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatalf("assessment changed project files:\nbefore %s\nafter  %s", before, after)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, record.Intent.ExecutionLeasePath)); err != nil {
		t.Fatal(err)
	}
	before = assessmentTree(t, root)
	assessment, err = Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.ExecutionLease.Status != "missing" || assessment.Status != AssessmentAssessed {
		t.Fatalf("missing lease assessment = lease %q status %q", assessment.ExecutionLease.Status, assessment.Status)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatalf("assessment created/changed a lease or another project file:\nbefore %s\nafter  %s", before, after)
	}
}

func TestAssessNeverProbesLeaseForNonterminalJournal(t *testing.T) {
	root, _, record, _ := spawnFixture(t, []string{"/usr/bin/true"})
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	leaseFile, err := os.OpenFile(filepath.Join(root, record.Intent.ExecutionLeasePath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := leaseFile.Close(); err != nil {
		t.Fatal(err)
	}
	called := false
	previousProbe := readOnlyLeaseProbe
	readOnlyLeaseProbe = func(string, string) (string, error) {
		called = true
		return "held", nil
	}
	t.Cleanup(func() { readOnlyLeaseProbe = previousProbe })
	assessment, err := Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if called || assessment.ExecutionLease.Status != "not-probed" || assessment.Status != AssessmentUncertain {
		t.Fatalf("nonterminal assessment probed lock=%v status=%q report=%q", called, assessment.ExecutionLease.Status, assessment.Status)
	}
}

func TestAssessCompletedTerminalEvidenceAndArtifactDrift(t *testing.T) {
	root, _, record, _ := spawnFixture(t, []string{"/bin/sh", "-c", "printf output; printf error >&2"})
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, journalPath); err != nil {
		t.Fatal(err)
	}
	assessment, err := Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.State != nativejournal.StateCompleted || assessment.TerminalEvidence.Status != "matched" || assessment.Receipt.Status != "matched" {
		t.Fatalf("completed assessment = state %q evidence %q receipt %q", assessment.State, assessment.TerminalEvidence.Status, assessment.Receipt.Status)
	}
	stdoutPath := filepath.Join(root, record.Intent.StdoutPath)
	if err := os.WriteFile(stdoutPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := assessmentTree(t, root)
	assessment, err = Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != AssessmentUncertain || assessment.TerminalEvidence.Status != "drift" || assessment.RecommendedAction != "manual-review" {
		t.Fatalf("drift assessment = status %q evidence %q action %q", assessment.Status, assessment.TerminalEvidence.Status, assessment.RecommendedAction)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatal("drift assessment changed project files")
	}
}

func TestAssessMismatchedReceiptAndUncertainLeaseFailClosed(t *testing.T) {
	root, receiptPath, record, _ := spawnFixture(t, []string{"/usr/bin/true"})
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil {
		t.Fatal(err)
	}
	receipt.RunID = "00000000-0000-4000-8000-000000000000"
	if err := run.SaveFile(filepath.Join(root, receiptPath), receipt); err != nil {
		t.Fatal(err)
	}
	before := assessmentTree(t, root)
	assessment, err := Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != AssessmentUncertain || assessment.Receipt.Status != "mismatched" || assessment.ExecutionLease.Status != "missing" {
		t.Fatalf("invalid evidence assessment = status %q receipt %q lease %q", assessment.Status, assessment.Receipt.Status, assessment.ExecutionLease.Status)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatal("uncertain assessment changed project files")
	}
}

func TestAssessRejectsDuplicateReceiptObjectKeys(t *testing.T) {
	root, receiptPath, record, _ := spawnFixture(t, []string{"/usr/bin/true"})
	receiptFile := filepath.Join(root, receiptPath)
	contents, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	contents = bytes.Replace(contents, []byte("{"), []byte(`{"run_id":"duplicate",`), 1)
	if err := os.WriteFile(receiptFile, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	before := assessmentTree(t, root)
	assessment, err := Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != AssessmentUncertain || assessment.Receipt.Status != "uncertain" {
		t.Fatalf("duplicate-key receipt status = report %q receipt %q", assessment.Status, assessment.Receipt.Status)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatal("duplicate receipt assessment changed project files")
	}
}

func TestAssessLegacyJournalReportsLeaseInspectionUnsupported(t *testing.T) {
	root, _, record, _ := spawnFixture(t, []string{"/usr/bin/true"})
	intent := record.Intent
	intent.SessionID = "legacy-read-only-assessment"
	intent.StdoutPath = filepath.Join(ArtifactDirectory, intent.SessionID+".stdout.log")
	intent.StderrPath = filepath.Join(ArtifactDirectory, intent.SessionID+".stderr.log")
	intent.ExecutionLeasePath = ""
	legacy, err := nativejournal.New(intent, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(ArtifactDirectory, intent.SessionID+".json")
	if err := nativejournal.Create(root, journalPath, legacy); err != nil {
		t.Fatal(err)
	}
	before := assessmentTree(t, root)
	assessment, err := Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != AssessmentUncertain || assessment.ExecutionLease.Status != "unsupported" {
		t.Fatalf("legacy assessment = status %q lease %q", assessment.Status, assessment.ExecutionLease.Status)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatal("legacy assessment changed project files")
	}
}

func TestAssessMissingTerminalCaptureIsUncertain(t *testing.T) {
	root, _, record, _ := spawnFixture(t, []string{"/usr/bin/true"})
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, journalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, record.Intent.StderrPath)); err != nil {
		t.Fatal(err)
	}
	before := assessmentTree(t, root)
	assessment, err := Assess(root, journalPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != AssessmentUncertain || assessment.TerminalEvidence.Status != "missing" || assessment.RecommendedAction != "manual-review" {
		t.Fatalf("missing capture assessment = status %q evidence %q action %q", assessment.Status, assessment.TerminalEvidence.Status, assessment.RecommendedAction)
	}
	if after := assessmentTree(t, root); after != before {
		t.Fatal("missing-capture assessment changed project files")
	}
}

// TestWriteNativeAssessmentFixtureForInstalledCLI is opt-in via two explicit
// environment variables. It creates a synthetic project and fake host client
// receipt/journal pair under an already-empty root, then runs /usr/bin/true
// through the real Sentinel wrapper. It performs no Herdr socket or network IO.
func TestWriteNativeAssessmentFixtureForInstalledCLI(t *testing.T) {
	root := os.Getenv("INGEN_NATIVE_ASSESS_FIXTURE_ROOT")
	executable := os.Getenv("INGEN_NATIVE_ASSESS_SENTINEL")
	if root == "" || executable == "" {
		return
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(executable) {
		t.Fatal("assessment fixture root and Sentinel executable must be absolute")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("assessment fixture root must be an empty directory")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.Initialize(project.Options{Root: root, ID: "native-assessment-acceptance"}); err != nil {
		t.Fatal(err)
	}
	receipt, err := run.NewUnderRoot(root, ".ingen/workspace.yaml", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := ".ingen/artifacts/sentinel-run.json"
	if err := run.SaveFile(filepath.Join(root, receiptPath), receipt); err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{}
	created, journalPath, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/private/tmp/synthetic-herdr.sock",
		SentinelExecutable: executable, Command: []string{"/usr/bin/true"},
	}, host)
	if err != nil {
		t.Fatalf("create synthetic assessment fixture: %v", err)
	}
	if host.created != 1 || host.runCalls != 1 {
		t.Fatalf("synthetic fake host calls = create %d run %d", host.created, host.runCalls)
	}
	wrapper := osexec.Command(executable, "session", "execute-native", "--root", root, "--path", journalPath)
	wrapper.Env = append(os.Environ(), "HERDR_ENV=1")
	if output, err := wrapper.CombinedOutput(); err != nil {
		t.Fatalf("execute synthetic wrapper: %v (%s)", err, output)
	}
	final, err := Load(root, journalPath)
	if err != nil || final.State != nativejournal.StateCompleted || final.Intent.SessionID != created.Intent.SessionID {
		t.Fatalf("synthetic fixture terminal journal = state %q err %v", final.State, err)
	}
}

func assessmentTree(t *testing.T, root string) string {
	t.Helper()
	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = hasher.Write([]byte(relative + "\x00"))
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileHash := sha256.Sum256(data)
		_, _ = hasher.Write(fileHash[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
