package policyreview_test

import (
	"path/filepath"
	"strings"
	"testing"

	"ingen/paddock/internal/policydiff"
	"ingen/paddock/internal/policyreview"
	"ingen/paddock/internal/policytest"
)

func TestNewRequiresPassingPolicyTests(t *testing.T) {
	diff := policydiff.Document{
		Schema: policydiff.Schema,
		Tests: &policytest.Document{
			Schema: policytest.DocumentSchema,
			Status: "PASS",
		},
	}
	document := policyreview.New(diff)
	if document.Schema != policyreview.Schema || document.Status != "PASS" {
		t.Fatalf("unexpected passing review: %#v", document)
	}

	diff.Tests.Status = "FAIL"
	document = policyreview.New(diff)
	if document.Status != "FAIL" {
		t.Fatalf("failed tests produced status %q, want FAIL", document.Status)
	}
}

func TestTextRequiresValidReview(t *testing.T) {
	document := policyreview.Document{Schema: policyreview.Schema, Status: "PASS"}
	if err := policyreview.Text(new(strings.Builder), document); err == nil {
		t.Fatal("invalid policy review unexpectedly rendered")
	}
}

func TestSaveAndLoadValidatesReview(t *testing.T) {
	document := policyreview.New(policydiff.Document{
		Schema: policydiff.Schema,
		Status: "unchanged",
		Before: policydiff.Input{Path: "before.yaml", SHA256: strings.Repeat("a", 64)},
		After:  policydiff.Input{Path: "after.yaml", SHA256: strings.Repeat("b", 64)},
		Tests: &policytest.Document{
			Schema:   policytest.DocumentSchema,
			Policy:   "policy.yaml",
			Status:   "PASS",
			Manifest: policytest.FileRef{Path: "tests.yaml", SHA256: strings.Repeat("a", 64)},
			Passed:   1,
			Cases: []policytest.CaseResult{{
				Name:     "good",
				Root:     "/service",
				Expected: "pass",
				Actual:   "pass",
				Status:   "PASS",
			}},
		},
	})
	path := filepath.Join(t.TempDir(), "review.json")
	if err := policyreview.Save(path, document); err != nil {
		t.Fatal(err)
	}
	loaded, err := policyreview.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schema != policyreview.Schema || loaded.Status != "PASS" {
		t.Fatalf("unexpected loaded review: %#v", loaded)
	}
}
