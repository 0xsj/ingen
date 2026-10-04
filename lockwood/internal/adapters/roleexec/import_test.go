package roleexec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/herdr-sentinel/evidence"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
	"ingen/sorna/policy"
)

func TestImportPreservesExactEvidenceAndVerifiesReferencedLineage(t *testing.T) {
	projectRoot, reportPath, reportBytes := roleEvidenceFixture(t, "canceled")
	verified, err := evidence.Verify(projectRoot, reportPath, hash(reportBytes))
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := t.TempDir()
	artifacts, records, ingestor := newStores(t, storeRoot)
	importer, err := NewImporter(ingestor, records)
	if err != nil {
		t.Fatal(err)
	}
	record, err := importer.Import(ImportRequest{
		ProjectRoot: projectRoot, ReportPath: reportPath, ExpectedSHA256: verified.ReportSHA256,
		CustodyID: "role-evidence-canceled",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Producer.Tool != "sentinel" || record.Producer.Kind != "role-execution-report" || len(record.Parents) != len(verified.Files) {
		t.Fatalf("report custody record = %+v", record)
	}
	if record.Artifact.Digest != "sha256:"+verified.ReportSHA256 {
		t.Fatalf("report digest = %q, want verifier digest sha256:%s", record.Artifact.Digest, verified.ReportSHA256)
	}
	gotReport, err := artifacts.Get(record.Artifact.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gotReport, []byte(`"status":"canceled"`)) {
		t.Fatalf("canceled producer outcome was not preserved in report bytes: %s", gotReport)
	}
	if !bytes.Equal(gotReport, reportBytes) {
		t.Fatalf("report bytes changed during import: %q vs %q", gotReport, reportBytes)
	}
	for _, ref := range record.Parents {
		parentRecords, err := records.List()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, parent := range parentRecords {
			if parent.Artifact.Digest != ref.Digest {
				continue
			}
			found = true
			got, err := artifacts.Get(ref.Digest)
			if err != nil {
				t.Fatal(err)
			}
			match := false
			for _, file := range verified.Files {
				if "sha256:"+file.SHA256 == ref.Digest && bytes.Equal(got, file.Bytes) {
					match = true
				}
			}
			if !match {
				t.Fatalf("related artifact %s did not preserve any exact verifier-provided bytes", ref.Digest)
			}
		}
		if !found {
			t.Fatalf("lineage parent %s has no accepted custody record", ref.Digest)
		}
	}
	if _, err := custody.VerifyRecord(records, artifacts, record.CustodyID); err != nil {
		t.Fatalf("VerifyRecord rejected complete related evidence graph: %v", err)
	}
}

func TestImportRejectsProducerVerificationFailureBeforePublishing(t *testing.T) {
	projectRoot, reportPath, _ := roleEvidenceFixture(t, "failed")
	stdoutPath := filepath.Join(projectRoot, ".ingen/artifacts/role-executions/evidence-case.stdout")
	if err := os.WriteFile(stdoutPath, []byte("tampered after report creation"), 0o600); err != nil {
		t.Fatal(err)
	}
	storeRoot := t.TempDir()
	artifacts, records, ingestor := newStores(t, storeRoot)
	importer, err := NewImporter(ingestor, records)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(ImportRequest{ProjectRoot: projectRoot, ReportPath: reportPath, CustodyID: "role-evidence-rejected"}); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("Import error = %v, want producer verification failure", err)
	}
	stored, err := records.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("producer verification failure published custody records: %+v", stored)
	}
	blobs, err := artifacts.ListBlobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(blobs) != 0 {
		t.Fatalf("producer verification failure published blobs: %+v", blobs)
	}
}

func TestImportRejectsForgedOrModifiedVerifiedSnapshotBeforePublishing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*evidence.Verified, *ImportRequest)
	}{
		{name: "constructed", mutate: func(verified *evidence.Verified, _ *ImportRequest) {
			*verified = evidence.Verified{ReportBytes: []byte(`{"forged":true}`), ReportSHA256: hash([]byte(`{"forged":true}`))}
		}},
		{name: "removed files", mutate: func(verified *evidence.Verified, _ *ImportRequest) { verified.Files = nil }},
		{name: "changed report", mutate: func(verified *evidence.Verified, _ *ImportRequest) { verified.Report.Status = "failed" }},
		{name: "changed project root", mutate: func(_ *evidence.Verified, request *ImportRequest) { request.ProjectRoot = t.TempDir() }},
		{name: "changed report path", mutate: func(_ *evidence.Verified, request *ImportRequest) { request.ReportPath = ".ingen/another-report.json" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectRoot, reportPath, reportBytes := roleEvidenceFixture(t, "completed")
			verified, err := evidence.Verify(projectRoot, reportPath, hash(reportBytes))
			if err != nil {
				t.Fatal(err)
			}
			request := ImportRequest{ProjectRoot: projectRoot, ReportPath: reportPath, CustodyID: "role-evidence-forged"}
			test.mutate(&verified, &request)
			storeRoot := t.TempDir()
			artifacts, records, ingestor := newStores(t, storeRoot)
			importer, err := NewImporter(ingestor, records)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := importer.ImportVerified(request, verified); err == nil {
				t.Fatal("ImportVerified accepted a constructed or changed verifier snapshot")
			}
			stored, err := records.List()
			if err != nil {
				t.Fatal(err)
			}
			blobs, err := artifacts.ListBlobs()
			if err != nil {
				t.Fatal(err)
			}
			if len(stored) != 0 || len(blobs) != 0 {
				t.Fatalf("snapshot rejection published records=%d blobs=%d", len(stored), len(blobs))
			}
		})
	}
}

func TestVerifyRecordDetectsCorruptRelatedEvidenceBlob(t *testing.T) {
	projectRoot, reportPath, _ := roleEvidenceFixture(t, "failed")
	storeRoot := t.TempDir()
	artifacts, records, ingestor := newStores(t, storeRoot)
	importer, err := NewImporter(ingestor, records)
	if err != nil {
		t.Fatal(err)
	}
	record, err := importer.Import(ImportRequest{ProjectRoot: projectRoot, ReportPath: reportPath, CustodyID: "role-evidence-corrupt"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(mustGet(t, artifacts, record.Artifact.Digest), []byte(`"status":"failed"`)) {
		t.Fatal("failed producer outcome was not preserved in report artifact")
	}
	parent := record.Parents[0]
	if err := os.WriteFile(blobPathForDigest(storeRoot, parent.Digest), []byte("corrupted related evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := custody.VerifyRecord(records, artifacts, record.CustodyID); err == nil || !strings.Contains(err.Error(), "verify parent artifact") {
		t.Fatalf("VerifyRecord error = %v, want related parent corruption", err)
	}
}

func TestImportReportsPublishedRelatedRecordsAfterLaterParentFailure(t *testing.T) {
	projectRoot, reportPath, _ := roleEvidenceFixture(t, "completed")
	storeRoot := t.TempDir()
	artifacts, err := store.NewFilesystem(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	baseRecords, err := custody.NewFilesystem(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	partialRecords := &failSecondRecordPut{RecordStore: baseRecords}
	ingestor, err := custody.NewIngestor(artifacts, partialRecords)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewImporter(ingestor, partialRecords)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(ImportRequest{ProjectRoot: projectRoot, ReportPath: reportPath, CustodyID: "role-evidence-partial"}); err == nil || !strings.Contains(err.Error(), "inspect these custody IDs and digests before retrying") {
		t.Fatalf("partial publication error = %v, want recovery details", err)
	}
	accepted, err := baseRecords.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted partial records = %d, want first related file only", len(accepted))
	}
}

type failSecondRecordPut struct {
	custody.RecordStore
	puts int
}

func (store *failSecondRecordPut) Put(record custody.Record) error {
	store.puts++
	if store.puts == 2 {
		return fmt.Errorf("injected second-record publication failure")
	}
	return store.RecordStore.Put(record)
}

func newStores(t *testing.T, root string) (*store.Filesystem, *custody.Filesystem, *custody.Ingestor) {
	t.Helper()
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
	return artifacts, records, ingestor
}

func roleEvidenceFixture(t *testing.T, status string) (string, string, []byte) {
	t.Helper()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := ".ingen/workspace.yaml"
	manifest := []byte(`sentinel_workspace:
  schema: ingen.sentinel-workspace/v1
  id: roleexec-import-test
  version: 1
  project_root: .
  contract:
    path: .ingen/contract.json
  implementation_roots: [src]
  sorna:
    oracle_policy: .ingen/oracle-policy.json
    subject_policy: .ingen/subject-policy.json
  delivery:
    workflow: .github/workflows/ci.yml
  roles:
    - id: contract-author
      kind: contract-author
      workspace: .work/contract
      read_roots: []
      write_roots: [contracts]
      deny_roots: [src]
    - id: oracle-writer
      kind: oracle-writer
      workspace: .work/oracle
      read_roots: [.ingen/contract.json]
      write_roots: [oracle]
      deny_roots: [src]
    - id: implementation
      kind: implementation
      workspace: .work/implementation
      read_roots: []
      write_roots: [src]
      deny_roots: [private]
    - id: verifier
      kind: verifier
      workspace: .work/verifier
      read_roots: [src]
      write_roots: []
      deny_roots: []
    - id: mutation-runner
      kind: mutation-runner
      workspace: .work/mutation
      read_roots: [src]
      write_roots: []
      deny_roots: []
`)
	workspacePathAbs := filepath.Join(root, filepath.FromSlash(workspacePath))
	if err := os.MkdirAll(filepath.Dir(workspacePathAbs), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspacePathAbs, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	const executionID = "evidence-case"
	policyPath := ".ingen/artifacts/role-executions/" + executionID + ".policy.json"
	stdoutPath := ".ingen/artifacts/role-executions/" + executionID + ".stdout"
	stderrPath := ".ingen/artifacts/role-executions/" + executionID + ".stderr"
	stdout, stderr := []byte("role output\n"), []byte("diagnostic\n")
	scratch := filepath.Join(root, ".work/implementation", ".sentinel-roleexec", executionID)
	abs := func(relative string) string { return filepath.Join(root, filepath.FromSlash(relative)) }
	rootEntries := func(paths []string, reason string) []any {
		result := make([]any, 0, len(paths))
		for _, path := range paths {
			result = append(result, map[string]any{"path": path, "reason": reason})
		}
		return result
	}
	readRoots := []string{}
	writeRoots := []string{abs("src"), filepath.Join(scratch, "rw")}
	denyRoots := []string{abs("private")}
	document := policy.Document{Policy: map[string]any{
		"schema": policy.Schema, "id": "sentinel-role-" + executionID, "version": 1, "status": "draft",
		"purpose": "noninteractive Sentinel role execution", "enforcement": "host-enforced",
		"filesystem": map[string]any{
			"read":  rootEntries(append(readRoots, scratch), "manifest read capability or derived role workspace"),
			"write": rootEntries(writeRoots, "manifest write capability or private execution scratch"),
			"deny":  rootEntries(denyRoots, "manifest deny capability"),
		},
		"network": map[string]any{"mode": "disabled"},
		"process": map[string]any{"subject_id": "roleexec-import-test/implementation", "can_invoke_subject": false, "allowed_tools": []any{}},
	}}
	sealed, err := policy.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err := policy.CanonicalJSON(sealed.Document)
	if err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string][]byte{
		policyPath: policyBytes,
		stdoutPath: stdout,
		stderrPath: stderr,
	} {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	started := now.Add(-time.Second)
	reason := ""
	var exitCode *int
	if status != "completed" {
		reason = "preserved producer outcome"
	}
	if status == "completed" {
		zero := 0
		exitCode = &zero
	} else if status == "canceled" {
		interrupted := 130
		exitCode = &interrupted
	} else if status == "failed" {
		nonzero := 1
		exitCode = &nonzero
	}
	report := map[string]any{
		"schema": "ingen.sentinel-role-execution/v1", "execution_id": executionID,
		"workspace_id": "roleexec-import-test", "role_id": "implementation", "role_kind": "implementation",
		"manifest_sha256": hash(manifest), "policy_sha256": hash(policyBytes),
		"workspace_manifest_path": workspacePath, "policy_path": policyPath,
		"executable_path": "/usr/bin/true", "executable_sha256": strings.Repeat("a", 64),
		"allowed_tools": []string{}, "allowed_tool_sha256": []string{},
		"backend": "macos-seatbelt", "enforcement": "host-enforced", "assurance": "unverified",
		"enforcement_scope": "filesystem, tool, and network rules",
		"limitations":       []string{"executable digest is a prelaunch byte check, not running-image identity", "Darwin runtime bootstrap and ancestor metadata are available", "no process namespace", "no control over prior role context", "no host attestation", "noninteractive execution only"},
		"network_mode":      "disabled", "declared_read_roots": []string{}, "declared_write_roots": []string{abs("src")},
		"declared_deny_roots": []string{abs("private")}, "derived_read_roots": []string{scratch}, "derived_write_roots": []string{filepath.Join(scratch, "rw")},
		"stdout_path": stdoutPath, "stdout_sha256": hash(stdout), "stderr_path": stderrPath, "stderr_sha256": hash(stderr),
		"started_at": started.Format(time.RFC3339Nano), "finished_at": now.Format(time.RFC3339Nano),
		"status": status, "reason": reason, "exit_code": exitCode,
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := ".ingen/artifacts/role-executions/" + executionID + ".json"
	reportAbsolute := filepath.Join(root, filepath.FromSlash(reportPath))
	if err := os.MkdirAll(filepath.Dir(reportAbsolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportAbsolute, reportBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, reportPath, reportBytes
}

func hash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func mustGet(t *testing.T, artifacts *store.Filesystem, digest string) []byte {
	t.Helper()
	data, err := artifacts.Get(digest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func blobPathForDigest(root, digest string) string {
	plain := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(root, "blobs", "sha256", plain[:2], plain[2:4], plain)
}
