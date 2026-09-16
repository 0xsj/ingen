package sattler

import (
	"testing"
)

func TestSummarizeChangesCountsCategoriesDeterministically(t *testing.T) {
	summary := SummarizeChanges([]Change{
		{Category: "verdict"},
		{Category: "check"},
		{Category: "verdict"},
	})
	if summary.Total != 3 || summary.ByCategory["verdict"] != 2 || summary.ByCategory["check"] != 1 {
		t.Fatalf("summary = %+v, want total 3 with verdict 2 and check 1", summary)
	}
	if got, want := summary.String(), "total 3 (check=1, verdict=2)"; got != want {
		t.Fatalf("summary text = %q, want %q", got, want)
	}
}

func TestSummarizeChangesEmptyIsExplicit(t *testing.T) {
	summary := SummarizeChanges(nil)
	if summary.Total != 0 || summary.ByCategory != nil {
		t.Fatalf("empty summary = %+v, want zero summary", summary)
	}
	if got, want := summary.String(), "none"; got != want {
		t.Fatalf("empty summary text = %q, want %q", got, want)
	}
}
