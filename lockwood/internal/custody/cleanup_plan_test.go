package custody

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildCleanupPlanKeepsAllActionsUnauthorized(t *testing.T) {
	oldTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	recentTime := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	report := Reconciliation{
		Orphans: []OrphanArtifact{
			{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SizeBytes: 2, ModifiedAt: oldTime},
			{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 1, ModifiedAt: recentTime},
		},
		CleanupCandidates: []OrphanArtifact{
			{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SizeBytes: 2, ModifiedAt: oldTime},
		},
		DanglingReferences: []DanglingReference{{CustodyID: "record-b", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Error: "missing"}, {CustodyID: "record-a", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Error: "damaged"}},
		CorruptBlobs:       []CorruptBlob{{Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Error: "tampered"}},
	}
	plan, err := BuildCleanupPlan(report, CleanupPlanOptions{
		OrphanGrace: 24 * time.Hour,
		AsOf:        time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Schema != CleanupPlanSchema || plan.OrphanGrace != "24h0m0s" || len(plan.Entries) != 2 || len(plan.DanglingReferences) != 2 || len(plan.CorruptBlobs) != 1 {
		t.Fatalf("cleanup plan = %+v", plan)
	}
	if plan.Entries[0].Digest != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || plan.Entries[0].State != ReportedOrphanState || plan.Entries[1].State != CleanupCandidateState {
		t.Fatalf("cleanup entries = %+v", plan.Entries)
	}
	for _, entry := range plan.Entries {
		if entry.ActionStatus != CleanupNotAuthorized || len(entry.Blockers) == 0 {
			t.Fatalf("cleanup entry = %+v", entry)
		}
	}
	if plan.DanglingReferences[0].CustodyID != "record-a" {
		t.Fatalf("dangling references are not deterministic: %+v", plan.DanglingReferences)
	}
}

func TestBuildCleanupPlanRejectsNegativeGrace(t *testing.T) {
	if _, err := BuildCleanupPlan(Reconciliation{}, CleanupPlanOptions{OrphanGrace: -time.Second}); err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("negative grace error = %v", err)
	}
}

func TestBuildCleanupPlanUsesEmptyArraysForCleanRoots(t *testing.T) {
	plan, err := BuildCleanupPlan(Reconciliation{}, CleanupPlanOptions{AsOf: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"entries":[]`, `"dangling_references":[]`, `"corrupt_blobs":[]`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("cleanup plan JSON = %s, missing %s", data, field)
		}
	}
}

func TestCleanupPlanDigestNormalizesOrderingAndBindsPlanContents(t *testing.T) {
	plan := CleanupPlan{
		Schema:      CleanupPlanSchema,
		AsOf:        time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		OrphanGrace: "24h0m0s",
		Entries: []CleanupPlanEntry{
			{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SizeBytes: 2, ModifiedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), State: CleanupCandidateState, ActionStatus: CleanupNotAuthorized, Blockers: []string{"fresh checks required"}},
			{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 1, ModifiedAt: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), State: ReportedOrphanState, ActionStatus: CleanupNotAuthorized, Blockers: []string{"grace required"}},
		},
	}
	first, err := CleanupPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Entries[0], plan.Entries[1] = plan.Entries[1], plan.Entries[0]
	second, err := CleanupPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("cleanup plan digest changed with entry ordering: %q != %q", first, second)
	}
	plan.OrphanGrace = "48h0m0s"
	changed, err := CleanupPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("cleanup plan digest did not change with plan contents")
	}
	plan.Entries[0].ActionStatus = "authorized"
	if _, err := CleanupPlanDigest(plan); err == nil || !strings.Contains(err.Error(), "invalid action status") {
		t.Fatalf("cleanup plan with authorized action accepted: %v", err)
	}
}
