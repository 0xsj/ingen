package custody

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

func newLineageStores(t *testing.T) (*Filesystem, *store.Filesystem, string) {
	t.Helper()
	root := t.TempDir()
	records, err := NewFilesystem(filepath.Join(root, "records"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.NewFilesystem(filepath.Join(root, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	return records, artifacts, root
}

func putLineageArtifact(t *testing.T, artifacts *store.Filesystem, data string) artifact.Reference {
	t.Helper()
	ref, err := artifacts.Put(bytes.NewBufferString(data), store.PutOptions{
		MediaType:   "application/octet-stream",
		LogicalName: "lineage.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func lineageBlobPath(root string, ref artifact.Reference) string {
	hex := strings.TrimPrefix(ref.Digest, "sha256:")
	return filepath.Join(root, "artifacts", "blobs", "sha256", hex[:2], hex[2:4], hex)
}

func TestVerifyLineageRequiresAcceptedParent(t *testing.T) {
	records, artifacts, _ := newLineageStores(t)
	parent := testRecord(t, "lockwood-lineage-parent")
	parent.Parents = nil
	parent.Artifact = putLineageArtifact(t, artifacts, "parent-bytes")
	child := testRecord(t, "lockwood-lineage-child")
	child.Parents = []Lineage{{Relation: DerivedFrom, Digest: parent.Artifact.Digest}}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLineage(records, artifacts, child); err == nil || !strings.Contains(err.Error(), "unresolved lineage parent") {
		t.Fatalf("VerifyLineage error = %v, want unresolved parent", err)
	}
	if err := records.Put(parent); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLineage(records, artifacts, child); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyLineageDoesNotAcceptQuarantinedParent(t *testing.T) {
	records, artifacts, _ := newLineageStores(t)
	parent := testRecord(t, "lockwood-lineage-quarantined")
	parent.Parents = nil
	parent.Artifact = putLineageArtifact(t, artifacts, "quarantined-parent")
	parent.Status = Quarantined
	parent.Integrity.Status = IntegrityNotChecked
	parent.Integrity.VerifiedAt = nil
	child := testRecord(t, "lockwood-lineage-child-quarantined")
	child.Parents = []Lineage{{Relation: References, Digest: parent.Artifact.Digest}}
	if err := records.Put(parent); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLineage(records, artifacts, child); err == nil {
		t.Fatal("VerifyLineage accepted a quarantined parent")
	}
}

func TestVerifyLineageChecksReachableParentBlob(t *testing.T) {
	records, artifacts, root := newLineageStores(t)
	parent := testRecord(t, "lockwood-lineage-corrupt-parent")
	parent.Parents = nil
	parent.Artifact = putLineageArtifact(t, artifacts, "parent-bytes")
	child := testRecord(t, "lockwood-lineage-corrupt-child")
	child.Parents = []Lineage{{Relation: References, Digest: parent.Artifact.Digest}}
	if err := records.Put(parent); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lineageBlobPath(root, parent.Artifact), []byte("tampered-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLineage(records, artifacts, child); err == nil || !strings.Contains(err.Error(), "verify parent artifact") {
		t.Fatalf("VerifyLineage error = %v, want parent blob failure", err)
	}
}

func TestVerifyLineageRejectsCycles(t *testing.T) {
	records, artifacts, _ := newLineageStores(t)
	a := testRecord(t, "lockwood-lineage-cycle-a")
	a.Artifact = putLineageArtifact(t, artifacts, "cycle-a")
	b := testRecord(t, "lockwood-lineage-cycle-b")
	b.Artifact = putLineageArtifact(t, artifacts, "cycle-b")
	a.Parents = []Lineage{{Relation: DerivedFrom, Digest: b.Artifact.Digest}}
	b.Parents = []Lineage{{Relation: DerivedFrom, Digest: a.Artifact.Digest}}
	if err := records.Put(a); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(b); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLineage(records, artifacts, a); err == nil || !strings.Contains(err.Error(), "lineage cycle detected") {
		t.Fatalf("VerifyLineage error = %v, want cycle failure", err)
	}
}

func TestVerifyLineageAllowsDuplicateAcceptedCustodyRecords(t *testing.T) {
	records, artifacts, _ := newLineageStores(t)
	parent := testRecord(t, "lockwood-lineage-duplicate-a")
	parent.Parents = nil
	parent.Artifact = putLineageArtifact(t, artifacts, "duplicate-parent")
	duplicate := parent
	duplicate.CustodyID = "lockwood-lineage-duplicate-b"
	child := testRecord(t, "lockwood-lineage-duplicate-child")
	child.Parents = []Lineage{{Relation: References, Digest: parent.Artifact.Digest}}
	if err := records.Put(parent); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(duplicate); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLineage(records, artifacts, child); err != nil {
		t.Fatal(err)
	}
}
