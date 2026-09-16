package sattler

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareLockwoodCustodyReportsArtifactAndSourceChanges(t *testing.T) {
	beforePath := writeCustodyFixture(t, custodyFixture("custody-before", "accepted", "sha256:"+strings.Repeat("a", 64), "run-before"))
	afterPath := writeCustodyFixture(t, custodyFixture("custody-after", "quarantined", "sha256:"+strings.Repeat("b", 64), "run-after"))

	report, err := CompareLockwoodCustodyFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compatible {
		t.Fatal("custody records with the same producer identity were marked incompatible")
	}
	if report.Before.ReceivedAt != "2026-09-16T12:00:00Z" {
		t.Fatalf("before received_at = %q, want fixture timestamp", report.Before.ReceivedAt)
	}
	if got, want := len(report.Changes), 3; got != want {
		t.Fatalf("change count = %d, want %d: %+v", got, want, report.Changes)
	}
	assertChange(t, report.Changes[0], "custody", "status")
	assertChange(t, report.Changes[1], "artifact", "artifact.digest")
	if report.Changes[1].Identity != ArtifactIdentityReplaced {
		t.Fatalf("artifact identity = %q, want %q", report.Changes[1].Identity, ArtifactIdentityReplaced)
	}
	assertChange(t, report.Changes[2], "context", "source.run_id")
}

func TestCompareLockwoodCustodyExplainsProducerDrift(t *testing.T) {
	beforePath := writeCustodyFixture(t, custodyFixture("custody-before", "accepted", "sha256:"+strings.Repeat("a", 64), "run-before"))
	after := custodyFixture("custody-after", "accepted", "sha256:"+strings.Repeat("a", 64), "run-after")
	after = strings.Replace(after, `"tool":"sorna"`, `"tool":"nublar"`, 1)
	afterPath := writeCustodyFixture(t, after)

	report, err := CompareLockwoodCustodyFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatible {
		t.Fatal("custody records with different producer tools were marked compatible")
	}
	if len(report.CompatibilityReasons) != 1 || !strings.Contains(report.CompatibilityReasons[0], "producer tool changed") {
		t.Fatalf("compatibility reasons = %+v, want producer-tool explanation", report.CompatibilityReasons)
	}
}

func TestCompareLockwoodCustodyRejectsWrongSchema(t *testing.T) {
	path := writeCustodyFixture(t, `{"schema":"lockwood.artifact/v1"}`)
	if _, err := CompareLockwoodCustodyFiles(path, path); err == nil || !strings.Contains(err.Error(), "Lockwood custody schema") {
		t.Fatalf("error = %v, want custody schema error", err)
	}
}

func TestWriteLockwoodTextNamesCustodyBoundary(t *testing.T) {
	path := writeCustodyFixture(t, custodyFixture("custody", "accepted", "sha256:"+strings.Repeat("a", 64), "run"))
	report, err := CompareLockwoodCustodyFiles(path, path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteLockwoodText(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler Lockwood custody comparison", "compatible: true", "none observable at the Lockwood boundary"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("text output = %q, missing %q", output.String(), fragment)
		}
	}
}

func writeCustodyFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "custody.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func custodyFixture(custodyID, status, digest, runID string) string {
	return `{"schema":"lockwood.custody/v1","custody_id":"` + custodyID + `","status":"` + status + `","artifact":{"schema":"lockwood.artifact/v1","digest":"` + digest + `"},"received_at":"2026-09-16T12:00:00Z","producer":{"tool":"sorna","kind":"behavioral-verification"},"source":{"run_id":"` + runID + `"},"integrity":{"status":"verified"}}`
}
