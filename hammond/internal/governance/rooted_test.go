package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyApprovedRootedChecksApprovalAndReturnsExactArtifacts(t *testing.T) {
	fixture := newRootedApprovalFixture(t)
	verified, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, fixture.policyPath, fixture.contract)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Record.State != StateApproved || !verified.Contract.Identity().Equal(fixture.contract.Identity()) {
		t.Fatalf("verification identity = %+v", verified)
	}
	if len(verified.Artifacts) != 4 {
		t.Fatalf("verified artifact refs = %#v, want approval, contract, policy, authority", verified.Artifacts)
	}
}

func TestVerifyApprovedRootedRejectsMissingRejectedSupersededAndWrongIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*rootedApprovalFixture) error
		want   string
	}{
		{name: "missing", mutate: func(f *rootedApprovalFixture) error { return os.Remove(filepath.Join(f.root, f.approvalPath)) }, want: "read Hammond approval record"},
		{name: "rejected", mutate: func(f *rootedApprovalFixture) error {
			return f.rewriteRecord(func(record *Record) { record.State = StateRejected })
		}, want: "state is \"rejected\""},
		{name: "superseded", mutate: func(f *rootedApprovalFixture) error {
			return f.rewriteRecord(func(record *Record) { record.State = StateSuperseded })
		}, want: "state is \"superseded\""},
		{name: "wrong digest", mutate: func(f *rootedApprovalFixture) error { f.contract.Artifact.SHA256 = strings.Repeat("0", 64); return nil }, want: "does not match the expected"},
		{name: "wrong version", mutate: func(f *rootedApprovalFixture) error { f.contract.Version++; return nil }, want: "does not match the expected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRootedApprovalFixture(t)
			if err := test.mutate(&fixture); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, fixture.policyPath, fixture.contract); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyApprovedRooted error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestVerifyApprovedRootedRejectsStaleOrEscapingReferences(t *testing.T) {
	t.Run("active policy drift", func(t *testing.T) {
		fixture := newRootedApprovalFixture(t)
		if err := os.WriteFile(filepath.Join(fixture.root, "active-policy.json"), []byte(`{"schema":"ingen.hammond-review-policy/v1","id":"review","version":1,"minimum_approvals":2,"authority":{"id":"reviewers","version":1,"schema":"ingen.hammond-authority/v1","artifact":{"uri":".ingen/governance/authority.json","sha256":"`+fixture.authorityDigest+`"}}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, "active-policy.json", fixture.contract); err == nil || !strings.Contains(err.Error(), "active Hammond review policy bytes do not match") {
			t.Fatalf("active policy drift error = %v", err)
		}
	})
	t.Run("policy traversal", func(t *testing.T) {
		fixture := newRootedApprovalFixture(t)
		if err := fixture.rewriteRecord(func(record *Record) { record.Policy.Artifact.URI = "../escape.json" }); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, fixture.policyPath, fixture.contract); err == nil || !strings.Contains(err.Error(), "not normalized") {
			t.Fatalf("policy traversal error = %v", err)
		}
	})
	t.Run("authority URL", func(t *testing.T) {
		fixture := newRootedApprovalFixture(t)
		if err := os.WriteFile(filepath.Join(fixture.root, "url-policy.json"), fixture.policyBytesWithAuthority("https://example.test/authority.json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := fixture.rewriteRecord(func(record *Record) {
			record.Policy.Artifact.URI = "url-policy.json"
			record.Policy.Artifact.SHA256 = fileDigest(t, filepath.Join(fixture.root, "url-policy.json"))
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, "url-policy.json", fixture.contract); err == nil || !strings.Contains(err.Error(), "project-relative") {
			t.Fatalf("authority URL error = %v", err)
		}
	})
	t.Run("authority tamper", func(t *testing.T) {
		fixture := newRootedApprovalFixture(t)
		if err := os.WriteFile(filepath.Join(fixture.root, fixture.authorityPath), []byte(`{"schema":"ingen.hammond-authority/v1","id":"reviewers","version":1,"actors":{"reviewer":["other-role"]}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, fixture.policyPath, fixture.contract); err == nil || !strings.Contains(err.Error(), "authority bytes do not match reference") {
			t.Fatalf("authority tamper error = %v", err)
		}
	})
	t.Run("authority symlink escape", func(t *testing.T) {
		fixture := newRootedApprovalFixture(t)
		outside := filepath.Join(t.TempDir(), "authority.json")
		if err := os.WriteFile(outside, fixture.authorityBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(fixture.root, fixture.authorityPath)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(fixture.root, fixture.authorityPath)); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyApprovedRooted(fixture.root, fixture.approvalPath, fixture.policyPath, fixture.contract); err == nil {
			t.Fatal("authority symlink escape was accepted")
		}
	})
}

type rootedApprovalFixture struct {
	root            string
	approvalPath    string
	policyPath      string
	authorityPath   string
	contract        ContractReference
	authorityBytes  []byte
	policyBytes     []byte
	authorityDigest string
}

func newRootedApprovalFixture(t *testing.T) rootedApprovalFixture {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{".ingen/contract", ".ingen/governance"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	contractPath := ".ingen/contract/canonical.json"
	contractBytes := []byte(`{"contract":{"schema":"ingen.contract/v1","id":"demo","version":1,"status":"sealed"}}`)
	writeRootedFixture(t, root, contractPath, contractBytes)
	contract := ContractReference{ProjectID: "demo-project", ID: "demo", Version: 1, Schema: "ingen.contract/v1", Artifact: Artifact{URI: contractPath, SHA256: fileDigest(t, filepath.Join(root, contractPath))}}

	authorityPath := ".ingen/governance/authority.json"
	authorityBytes, err := json.Marshal(map[string]any{
		"schema": AuthoritySchema, "id": "reviewers", "version": 1,
		"actors": map[string][]string{"reviewer": {"product-reviewer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeRootedFixture(t, root, authorityPath, authorityBytes)
	authorityDigest := fileDigest(t, filepath.Join(root, authorityPath))
	policyPath := ".ingen/governance/review-policy.json"
	policyBytes := marshalPolicy(authorityPath, authorityDigest, 1)
	writeRootedFixture(t, root, policyPath, policyBytes)
	policyRef := PolicyReference{ID: "review", Version: 1, Schema: PolicySchema, Artifact: Artifact{URI: policyPath, SHA256: fileDigest(t, filepath.Join(root, policyPath))}}

	record := Record{
		Schema: Schema, RecordID: "approval-demo", Contract: contract, Policy: policyRef, State: StateApproved,
		Events: []Event{
			{ID: "registered", Type: EventRegistered, Actor: "owner", At: "2026-10-01T00:00:00Z"},
			{ID: "review", Type: EventReviewOpened, Actor: "owner", ReviewCycleID: "cycle-1", At: "2026-10-01T00:01:00Z"},
			{ID: "approved", Type: EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "cycle-1", Decision: DecisionApprove, ArtifactSHA256: contract.Artifact.SHA256, At: "2026-10-01T00:02:00Z"},
		},
	}
	approvalPath := ".ingen/governance/approval.json"
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeRootedFixture(t, root, approvalPath, encoded)
	return rootedApprovalFixture{root: root, approvalPath: approvalPath, policyPath: policyPath, authorityPath: authorityPath, contract: contract, authorityBytes: authorityBytes, policyBytes: policyBytes, authorityDigest: authorityDigest}
}

func (fixture rootedApprovalFixture) rewriteRecord(tweak func(*Record)) error {
	path := filepath.Join(fixture.root, fixture.approvalPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	tweak(&record)
	data, err = json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (fixture rootedApprovalFixture) policyBytesWithAuthority(uri string) []byte {
	return marshalPolicy(uri, fixture.authorityDigest, 1)
}

func marshalPolicy(authorityPath, authorityDigest string, minimum int) []byte {
	data, _ := json.Marshal(map[string]any{
		"schema": PolicySchema, "id": "review", "version": 1, "minimum_approvals": minimum,
		"authority": map[string]any{"id": "reviewers", "version": 1, "schema": AuthoritySchema, "artifact": map[string]string{"uri": authorityPath, "sha256": authorityDigest}},
	})
	return data
}

func writeRootedFixture(t *testing.T, root, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
