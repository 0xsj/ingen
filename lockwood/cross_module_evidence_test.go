package lockwood

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/adapters/ciresult"
	"ingen/lockwood/internal/adapters/sorna"
	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestCrossModuleEvidenceFixturePath(t *testing.T) {
	artifacts := store.NewMemory()
	records := custody.NewMemory()
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}

	receivedAt := time.Date(2026, 9, 15, 12, 10, 0, 0, time.UTC)
	hammondContract := readCrossModuleFixture(t, "valid-hammond-contract-v1.json")
	hammondContractRecord := acceptCrossModuleArtifact(t, ingestor, hammondContract, custody.IntakeRequest{
		CustodyID:   "lockwood-hammond-contract-0001",
		MediaType:   "application/json",
		LogicalName: "valid-hammond-contract-v1.json",
		ReceivedAt:  receivedAt,
		Producer:    custody.Producer{Tool: "hammond", Kind: "contract-artifact"},
		Source:      custody.Source{Path: "lockwood/testdata/valid-hammond-contract-v1.json"},
	})

	hammondGovernance := readCrossModuleFixture(t, "valid-hammond-governance-v1.json")
	if !bytes.Contains(hammondGovernance, []byte(localDigestHex(hammondContractRecord.Artifact.Digest))) {
		t.Fatalf("Hammond governance reference does not bind contract digest %s", hammondContractRecord.Artifact.Digest)
	}
	hammondGovernanceRecord := acceptCrossModuleArtifact(t, ingestor, hammondGovernance, custody.IntakeRequest{
		CustodyID:   "lockwood-hammond-governance-0001",
		MediaType:   "application/json",
		LogicalName: "valid-hammond-governance-v1.json",
		ReceivedAt:  receivedAt,
		Producer:    custody.Producer{Tool: "hammond", Kind: "governance-record"},
		Source:      custody.Source{Path: "lockwood/testdata/valid-hammond-governance-v1.json"},
		Parents: []custody.Lineage{{
			Relation: custody.References,
			Digest:   hammondContractRecord.Artifact.Digest,
		}},
	})

	sornaBundle := filepath.Join(t.TempDir(), "sorna-run-0001")
	writeCrossModuleSornaFixture(t, sornaBundle)
	sornaImporter, err := sorna.NewImporter(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	sornaEvidence, err := sornaImporter.Import(sornaBundle, sorna.ImportRequest{
		CustodyID:  "lockwood-sorna-evidence-0001",
		ReceivedAt: receivedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondSornaEvidence, err := sornaImporter.Import(sornaBundle, sorna.ImportRequest{
		CustodyID:  "lockwood-sorna-evidence-0002",
		ReceivedAt: receivedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sornaEvidence.Artifact.Digest != secondSornaEvidence.Artifact.Digest {
		t.Fatalf("Sorna fixture archive is not deterministic: %s vs %s", sornaEvidence.Artifact.Digest, secondSornaEvidence.Artifact.Digest)
	}

	ciResult := readCrossModuleFixture(t, "valid-ci-result-failed-v1.json")
	ciPath := filepath.Join(t.TempDir(), "ci-result.json")
	if err := os.WriteFile(ciPath, ciResult, 0o600); err != nil {
		t.Fatal(err)
	}
	ciImporter, err := ciresult.NewImporter(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	ciRecord, err := ciImporter.Import(ciPath, ciresult.ImportRequest{
		CustodyID:  "lockwood-ci-result-0001",
		ReceivedAt: receivedAt,
		Source:     custody.Source{RunID: "nublar-cross-module-0001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ciRecord.Status != custody.Accepted {
		t.Fatalf("failed producer result was not accepted as intact evidence: %s", ciRecord.Status)
	}
	if !bytes.Contains(ciResult, []byte(`"status":"failed"`)) {
		t.Fatal("CI fixture no longer carries its producer-owned failed status")
	}

	nublarRun := readCrossModuleFixture(t, "valid-nublar-run-failed-v1.json")
	if !bytes.Contains(nublarRun, []byte(localDigestHex(ciRecord.Artifact.Digest))) {
		t.Fatalf("Nublar run does not bind CI result digest %s", ciRecord.Artifact.Digest)
	}
	nublarRunRecord := acceptCrossModuleArtifact(t, ingestor, nublarRun, custody.IntakeRequest{
		CustodyID:   "lockwood-nublar-run-0001",
		MediaType:   "application/json",
		LogicalName: "valid-nublar-run-failed-v1.json",
		ReceivedAt:  receivedAt,
		Producer:    custody.Producer{Tool: "nublar", Kind: "run"},
		Source:      custody.Source{RunID: "nublar-cross-module-0001"},
		Parents: []custody.Lineage{
			{Relation: custody.References, Digest: hammondGovernanceRecord.Artifact.Digest},
			{Relation: custody.Contains, Digest: sornaEvidence.Artifact.Digest},
			{Relation: custody.References, Digest: ciRecord.Artifact.Digest},
		},
	})

	deliveryReceipt := readCrossModuleFixture(t, "valid-nublar-delivery-receipt-failed-v1.json")
	deliveryRecord := acceptCrossModuleArtifact(t, ingestor, deliveryReceipt, custody.IntakeRequest{
		CustodyID:   "lockwood-nublar-delivery-receipt-0001",
		MediaType:   "application/json",
		LogicalName: "valid-nublar-delivery-receipt-failed-v1.json",
		ReceivedAt:  receivedAt,
		Producer:    custody.Producer{Tool: "nublar", Kind: "delivery-receipt"},
		Source:      custody.Source{RunID: "nublar-cross-module-0001"},
		Parents: []custody.Lineage{{
			Relation: custody.References,
			Digest:   nublarRunRecord.Artifact.Digest,
		}},
	})

	if _, err := custody.VerifyRecord(records, artifacts, deliveryRecord.CustodyID); err != nil {
		t.Fatalf("verify complete cross-module lineage: %v", err)
	}
	for _, fixture := range []struct {
		name   string
		record custody.Record
		want   []byte
	}{
		{name: "Hammond contract", record: hammondContractRecord, want: hammondContract},
		{name: "Hammond governance", record: hammondGovernanceRecord, want: hammondGovernance},
		{name: "CI result", record: ciRecord, want: ciResult},
		{name: "Nublar run", record: nublarRunRecord, want: nublarRun},
		{name: "Nublar receipt", record: deliveryRecord, want: deliveryReceipt},
	} {
		stored, err := artifacts.Get(fixture.record.Artifact.Digest)
		if err != nil {
			t.Fatalf("get %s: %v", fixture.name, err)
		}
		if !bytes.Equal(stored, fixture.want) {
			t.Fatalf("%s bytes changed during custody", fixture.name)
		}
	}

	t.Run("changed bytes fail expected digest", func(t *testing.T) {
		tamperedPath := filepath.Join(t.TempDir(), "ci-result-tampered.json")
		if err := os.WriteFile(tamperedPath, append(append([]byte(nil), ciResult...), '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ciImporter.Import(tamperedPath, ciresult.ImportRequest{
			CustodyID:      "lockwood-ci-result-tampered",
			ExpectedDigest: ciRecord.Artifact.Digest,
			ReceivedAt:     receivedAt,
			Source:         custody.Source{RunID: "nublar-cross-module-0001"},
		}); err == nil || !strings.Contains(err.Error(), "expected digest") {
			t.Fatalf("tampered CI import error = %v, want expected-digest failure", err)
		}
		if _, err := records.Get("lockwood-ci-result-tampered"); err == nil {
			t.Fatal("tampered CI result created a custody record")
		}
	})

	t.Run("malformed producer result fails before custody", func(t *testing.T) {
		malformedPath := filepath.Join(t.TempDir(), "ci-result-malformed.json")
		if err := os.WriteFile(malformedPath, []byte(`{"schema":"ingen.ci-result/v1","tool":"sorna"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ciImporter.Import(malformedPath, ciresult.ImportRequest{
			CustodyID: "lockwood-ci-result-malformed",
			Source:    custody.Source{RunID: "nublar-cross-module-0001"},
		}); err == nil || !strings.Contains(err.Error(), "validate CI result") {
			t.Fatalf("malformed CI import error = %v, want validation failure", err)
		}
		if _, err := records.Get("lockwood-ci-result-malformed"); err == nil {
			t.Fatal("malformed CI result created a custody record")
		}
	})

	t.Run("missing parent fails verification", func(t *testing.T) {
		missingDigest := artifact.DigestBytes([]byte("missing-cross-module-parent"))
		missingParentRecord := acceptCrossModuleArtifact(t, ingestor, []byte(`{"receipt":"missing-parent"}`), custody.IntakeRequest{
			CustodyID:   "lockwood-missing-parent-receipt",
			MediaType:   "application/json",
			LogicalName: "missing-parent-receipt.json",
			ReceivedAt:  receivedAt,
			Producer:    custody.Producer{Tool: "nublar", Kind: "delivery-receipt"},
			Source:      custody.Source{RunID: "nublar-cross-module-0001"},
			Parents: []custody.Lineage{{
				Relation: custody.References,
				Digest:   missingDigest,
			}},
		})
		if _, err := custody.VerifyRecord(records, artifacts, missingParentRecord.CustodyID); err == nil || !strings.Contains(err.Error(), "unresolved lineage parent") {
			t.Fatalf("missing parent verification error = %v, want unresolved lineage failure", err)
		}
	})
}

func readCrossModuleFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func localDigestHex(digest string) string {
	return strings.TrimPrefix(digest, artifact.SHA256Algorithm+":")
}

func acceptCrossModuleArtifact(t *testing.T, ingestor *custody.Ingestor, data []byte, request custody.IntakeRequest) custody.Record {
	t.Helper()
	if request.Handling.Redaction == "" {
		request.Handling.Redaction = "none"
	}
	if request.Handling.RetentionClass == "" {
		request.Handling.RetentionClass = "standard"
	}
	record, err := ingestor.Accept(bytes.NewReader(data), request)
	if err != nil {
		t.Fatalf("accept %s: %v", request.CustodyID, err)
	}
	return record
}

func writeCrossModuleSornaFixture(t *testing.T, directory string) {
	t.Helper()
	files := map[string][]byte{
		"run.json":               []byte(`{"schema":"ingen.run/v1","run_id":"sorna-run-0001"}`),
		"events/lifecycle.jsonl": []byte("{\"sequence\":1,\"event\":\"completed\"}\n"),
	}
	files["manifest.json"] = []byte(`{"schema":"sorna.evidence/v1","run_id":"sorna-run-0001"}` + "\n")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	checksums := make([]string, 0, len(files))
	for name, data := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		checksums = append(checksums, hex.EncodeToString(digest[:])+"  "+name)
	}
	sort.Strings(checksums)
	if err := os.WriteFile(filepath.Join(directory, "checksums.sha256"), []byte(strings.Join(checksums, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(fmt.Errorf("write Sorna checksums: %w", err))
	}
}
