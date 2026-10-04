package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"ingen/herdr-sentinel/internal/capability"
	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/nativesession"
	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/run"
)

func TestProducedNativeRecoveryAssessmentMatchesClosedSchema(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: "native-assessment-schema"}); err != nil {
		t.Fatal(err)
	}
	plan, err := capability.FromFileUnderRoot(root, ".ingen/workspace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var role capability.Role
	for _, candidate := range plan.Roles {
		if candidate.ID == "contract-author" {
			role = candidate
			break
		}
	}
	if role.ID == "" {
		t.Fatal("initialized workspace has no contract-author role")
	}
	receipt, err := run.NewUnderRoot(root, ".ingen/workspace.yaml", time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := ".ingen/artifacts/native-assessment-receipt.json"
	if err := run.SaveFile(filepath.Join(root, receiptPath), receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, nativesession.ArtifactDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	executableBytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	executableHash := sha256.Sum256(executableBytes)
	sessionID := "native-assessment-schema"
	intent := nativejournal.Intent{
		SessionID: sessionID, RunID: receipt.RunID, WorkspaceID: receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version, WorkspaceManifestSHA256: receipt.Workspace.File.SHA256,
		RoleID: role.ID, RoleKind: role.Kind, Workdir: role.Workspace, ReceiptPath: receiptPath,
		Argv:               []string{"/usr/bin/true"},
		StdoutPath:         filepath.Join(nativesession.ArtifactDirectory, sessionID+".stdout.log"),
		StderrPath:         filepath.Join(nativesession.ArtifactDirectory, sessionID+".stderr.log"),
		ExecutionLeasePath: filepath.Join(nativesession.ArtifactDirectory, sessionID+".lease"),
		HerdrSocket:        "/private/tmp/native-assessment-schema.sock", SentinelExecutable: executable,
		SentinelExecutableSHA256: hex.EncodeToString(executableHash[:]),
	}
	journal, err := nativejournal.New(intent, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(nativesession.ArtifactDirectory, sessionID+".json")
	if err := nativejournal.Create(root, journalPath, journal); err != nil {
		t.Fatal(err)
	}
	assessment, err := nativesession.Assess(root, journalPath, time.Date(2026, 10, 4, 12, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.NewCompiler().Compile("native-recovery-assessment-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	document := boundaryDocument(t, assessment)
	if err := schema.Validate(document); err != nil {
		t.Fatalf("produced read-only assessment fails its published schema: %v", err)
	}
	delete(document, "journal_sha256")
	if err := schema.Validate(document); err == nil {
		t.Fatal("assessment schema accepted a missing journal digest")
	}
}

func TestInstalledNativeRecoveryAssessmentMatchesSchema(t *testing.T) {
	path := os.Getenv("INGEN_NATIVE_ASSESS_REPORT")
	if path == "" {
		t.Skip("set INGEN_NATIVE_ASSESS_REPORT to validate installed CLI output")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.NewCompiler().Compile("native-recovery-assessment-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("installed native-assess output fails published schema: %v", err)
	}
}
