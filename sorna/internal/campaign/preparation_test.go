package campaign

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoadPreparationBytesAcceptsCanonicalSummary(t *testing.T) {
	summary := validPreparationSummary()
	contents, err := CanonicalPreparationJSON(summary)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPreparationBytes("preparation.json", contents)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schema != PreparationSummarySchema || loaded.ProviderID != summary.ProviderID || len(loaded.Variants) != 1 {
		t.Fatalf("loaded preparation summary = %+v, want canonical summary", loaded)
	}
}

func TestLoadPreparationBytesRejectsUnknownFields(t *testing.T) {
	contents, err := CanonicalPreparationJSON(validPreparationSummary())
	if err != nil {
		t.Fatal(err)
	}
	contents = bytes.Replace(contents, []byte("{\n"), []byte("{\n  \"unexpected\": true,\n"), 1)
	if _, err := LoadPreparationBytes("preparation.json", contents); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("LoadPreparationBytes() = %v, want unknown-field error", err)
	}
}

func TestLoadPreparationBytesRejectsTrailingJSON(t *testing.T) {
	contents, err := CanonicalPreparationJSON(validPreparationSummary())
	if err != nil {
		t.Fatal(err)
	}
	contents = append(contents, []byte("{}\n")...)
	if _, err := LoadPreparationBytes("preparation.json", contents); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("LoadPreparationBytes() = %v, want multiple-value error", err)
	}
}

func validPreparationSummary() PreparationSummary {
	digest := strings.Repeat("a", 64)
	provenance := ProviderProvenance{
		SourceDir:    "variants/001/source",
		SourceSHA256: digest,
		BinarySHA256: digest,
		Location:     "subject/server.go:createDocument:response.status",
		Before:       "http.StatusAccepted",
		After:        "http.StatusOK",
	}
	return PreparationSummary{
		Schema:     PreparationSummarySchema,
		ProviderID: "provider",
		PlanPath:   "plan.json",
		PlanSHA256: digest,
		Variants: []PreparationVariant{{
			Sequence:     1,
			MutationID:   "mutation",
			SourceDir:    "variants/001/source",
			SourceSHA256: digest,
			BinaryPath:   "variants/001/subject",
			BinarySHA256: digest,
			ChangedFiles: []string{"subject/server.go"},
			Provenance:   &provenance,
		}},
	}
}
