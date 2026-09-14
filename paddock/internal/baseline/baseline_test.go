package baseline_test

import (
	"path/filepath"
	"testing"

	"ingen/paddock/internal/baseline"
	"ingen/paddock/internal/model"
)

func TestBuildSaveLoadAndApply(t *testing.T) {
	result := &model.Result{
		Schema:     "paddock.report/v1",
		Policy:     "policy.yaml",
		ModulePath: "example.com/service",
		Findings: []*model.Finding{
			{
				RuleID:   "no-cycles",
				Kind:     "no-cycles",
				Severity: "error",
				From:     "internal/orders",
				To:       "internal/billing",
				File:     "internal/orders/order.go",
				Line:     12,
				Message:  "cycle",
			},
			{
				RuleID:   "legacy",
				Kind:     "deny-dependencies",
				Severity: "error",
				Waived:   true,
			},
		},
	}

	snapshot := baseline.Build(result)
	if len(snapshot.Entries) != 1 {
		t.Fatalf("baseline entries = %d, want 1", len(snapshot.Entries))
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := baseline.Save(path, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := baseline.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	current := &model.Result{
		ModulePath: "example.com/service",
		Findings: []*model.Finding{
			{
				RuleID: "no-cycles",
				Kind:   "no-cycles",
				From:   "internal/orders",
				To:     "internal/billing",
				File:   "internal/orders/order.go",
				Line:   99,
			},
		},
	}
	if err := baseline.Apply(current, loaded, path); err != nil {
		t.Fatal(err)
	}
	if !current.Findings[0].Baselined || !current.OK() {
		t.Fatalf("finding was not accepted by baseline: %#v", current.Findings[0])
	}
	if current.Baseline == nil || current.Baseline.Matched != 1 || len(current.Baseline.Stale) != 0 {
		t.Fatalf("unexpected baseline summary: %#v", current.Baseline)
	}
}

func TestApplyReportsStaleEntries(t *testing.T) {
	result := &model.Result{ModulePath: "example.com/service"}
	snapshot := baseline.Snapshot{
		Schema:     "paddock.baseline/v1",
		ModulePath: "example.com/service",
		Entries: []baseline.Entry{{
			RuleID: "old-rule",
			Kind:   "deny-dependencies",
			From:   "internal/old",
		}},
	}
	snapshot.Entries[0].Fingerprint = baseline.Fingerprint(&model.Finding{
		RuleID: snapshot.Entries[0].RuleID,
		Kind:   snapshot.Entries[0].Kind,
		From:   snapshot.Entries[0].From,
	})
	if err := baseline.Apply(result, snapshot, "baseline.json"); err != nil {
		t.Fatal(err)
	}
	if result.Baseline == nil || len(result.Baseline.Stale) != 1 {
		t.Fatalf("expected one stale entry: %#v", result.Baseline)
	}
}

func TestApplyRejectsDifferentModule(t *testing.T) {
	snapshot := baseline.Snapshot{
		Schema:     "paddock.baseline/v1",
		ModulePath: "example.com/original",
	}
	result := &model.Result{ModulePath: "example.com/other"}
	if err := baseline.Apply(result, snapshot, "baseline.json"); err == nil {
		t.Fatal("baseline.Apply succeeded, want module mismatch error")
	}
}
