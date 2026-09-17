package custody

import (
	"bytes"
	"strings"
	"testing"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

func TestVerifyAllReportsEveryRecordWithoutChangingStatus(t *testing.T) {
	artifacts := store.NewMemory()
	records := NewMemory()
	ingestor, err := NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	good, err := ingestor.Accept(bytes.NewBufferString("good bytes"), IntakeRequest{
		CustodyID: "lockwood-verify-report-good",
		MediaType: "text/plain",
		Producer:  Producer{Tool: "example", Kind: "fixture"},
		Source:    Source{Path: "good.txt"},
		Handling:  Handling{Redaction: "none", RetentionClass: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := ingestor.Accept(bytes.NewBufferString("bad lineage"), IntakeRequest{
		CustodyID: "lockwood-verify-report-bad",
		MediaType: "text/plain",
		Producer:  Producer{Tool: "example", Kind: "fixture"},
		Source:    Source{Path: "bad.txt"},
		Parents: []Lineage{{
			Relation: References,
			Digest:   artifact.DigestBytes([]byte("missing parent")),
		}},
		Handling: Handling{Redaction: "none", RetentionClass: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := VerifyAll(records, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != VerificationReportSchema || report.Checked != 2 || report.Verified != 1 || report.Failed != 1 || len(report.Results) != 2 {
		t.Fatalf("verification report = %+v", report)
	}
	results := make(map[string]VerificationResult, len(report.Results))
	for _, result := range report.Results {
		results[result.CustodyID] = result
	}
	if results[good.CustodyID].Status != VerificationVerified {
		t.Fatalf("good result = %+v", results[good.CustodyID])
	}
	if results[bad.CustodyID].Status != VerificationFailed || !strings.Contains(results[bad.CustodyID].Error, "unresolved lineage parent") {
		t.Fatalf("bad result = %+v", results[bad.CustodyID])
	}
	storedBad, err := records.Get(bad.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if storedBad.Status != Accepted {
		t.Fatalf("batch verification changed custody status to %q", storedBad.Status)
	}
}

func TestVerifyRecordsRequiresStores(t *testing.T) {
	if _, err := VerifyAll(nil, store.NewMemory()); err == nil {
		t.Fatal("VerifyAll accepted a nil custody store")
	}
	if _, err := VerifyRecords(NewMemory(), nil, nil); err == nil {
		t.Fatal("VerifyRecords accepted a nil artifact store")
	}
}
