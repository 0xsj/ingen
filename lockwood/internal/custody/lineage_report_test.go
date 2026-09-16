package custody

import (
	"os"
	"strings"
	"testing"

	"ingen/lockwood/internal/store"
)

func TestAnalyzeLineageReportsCompleteProjection(t *testing.T) {
	records, artifacts, _ := newLineageStores(t)
	parent := testRecord(t, "lockwood-lineage-report-parent")
	parent.Parents = []Lineage{}
	parent.Artifact = putLineageArtifact(t, artifacts, "report-parent")
	child := testRecord(t, "lockwood-lineage-report-child")
	child.Artifact = putLineageArtifact(t, artifacts, "report-child")
	child.Parents = []Lineage{{Relation: DerivedFrom, Digest: parent.Artifact.Digest}}
	if err := records.Put(parent); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}

	report, err := AnalyzeLineage(records, artifacts, child)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != LineageComplete || len(report.Issues) != 0 || len(report.ReachableDigests) != 2 {
		t.Fatalf("complete lineage report = %+v", report)
	}
	if report.ReachableDigests[0] >= report.ReachableDigests[1] {
		t.Fatalf("reachable digests are not deterministic: %+v", report.ReachableDigests)
	}
}

func TestAnalyzeLineageReportsUnresolvedParentWithoutChangingRecord(t *testing.T) {
	records, artifacts, _ := newLineageStores(t)
	child := testRecord(t, "lockwood-lineage-report-unresolved")
	child.Artifact = putLineageArtifact(t, artifacts, "report-unresolved")
	missing := putLineageArtifact(t, artifacts, "missing-record")
	child.Parents = []Lineage{{Relation: References, Digest: missing.Digest}}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}

	report, err := AnalyzeLineage(records, artifacts, child)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != LineageIncomplete || len(report.Issues) != 1 || report.Issues[0].Kind != UnresolvedParentIssue {
		t.Fatalf("unresolved lineage report = %+v", report)
	}
	stored, err := records.Get(child.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != child.Status || len(stored.Parents) != 1 || stored.Parents[0].Digest != missing.Digest {
		t.Fatalf("lineage analysis changed custody record: %+v", stored)
	}
}

func TestAnalyzeLineageReportsCyclesAndDamagedArtifacts(t *testing.T) {
	records, artifacts, root := newLineageStores(t)
	a := testRecord(t, "lockwood-lineage-report-cycle-a")
	a.Artifact = putLineageArtifact(t, artifacts, "report-cycle-a")
	b := testRecord(t, "lockwood-lineage-report-cycle-b")
	b.Artifact = putLineageArtifact(t, artifacts, "report-cycle-b")
	a.Parents = []Lineage{{Relation: DerivedFrom, Digest: b.Artifact.Digest}}
	b.Parents = []Lineage{{Relation: DerivedFrom, Digest: a.Artifact.Digest}}
	if err := records.Put(a); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(b); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeLineage(records, artifacts, a)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != LineageIncomplete || len(report.Issues) == 0 {
		t.Fatalf("cycle lineage report = %+v", report)
	}
	var sawCycle bool
	for _, issue := range report.Issues {
		if issue.Kind == CycleIssue {
			sawCycle = true
		}
	}
	if !sawCycle {
		t.Fatalf("cycle issue missing from report: %+v", report.Issues)
	}

	damaged := testRecord(t, "lockwood-lineage-report-damaged")
	damaged.Artifact = putLineageArtifact(t, artifacts, "report-damaged")
	child := testRecord(t, "lockwood-lineage-report-damaged-child")
	child.Artifact = putLineageArtifact(t, artifacts, "report-damaged-child")
	child.Parents = []Lineage{{Relation: References, Digest: damaged.Artifact.Digest}}
	if err := records.Put(damaged); err != nil {
		t.Fatal(err)
	}
	if err := records.Put(child); err != nil {
		t.Fatal(err)
	}
	hexDigest := strings.TrimPrefix(damaged.Artifact.Digest, "sha256:")
	damagedPath := lineageBlobPath(root, damaged.Artifact)
	if damagedPath == "" || hexDigest == "" {
		t.Fatal("failed to construct damaged artifact path")
	}
	if err := os.WriteFile(damagedPath, []byte("tampered-report-artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	damagedReport, err := AnalyzeLineage(records, artifacts, child)
	if err != nil {
		t.Fatal(err)
	}
	var sawDamage bool
	for _, issue := range damagedReport.Issues {
		if issue.Kind == ArtifactIntegrityIssue {
			sawDamage = true
		}
	}
	if !sawDamage {
		t.Fatalf("artifact-integrity issue missing from report: %+v", damagedReport.Issues)
	}
}

func TestAnalyzeLineageRequiresValidInputs(t *testing.T) {
	if _, err := AnalyzeLineage(nil, store.NewMemory(), Record{}); err == nil || !strings.Contains(err.Error(), "custody record store") {
		t.Fatalf("missing-record-store error = %v", err)
	}
	if _, err := AnalyzeLineage(NewMemory(), nil, Record{}); err == nil || !strings.Contains(err.Error(), "artifact store") {
		t.Fatalf("missing-artifact-store error = %v", err)
	}
}
