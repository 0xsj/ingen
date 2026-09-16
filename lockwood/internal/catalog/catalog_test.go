package catalog

import (
	"os"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

var fixedTime = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

func TestCatalogInspectAndFind(t *testing.T) {
	root := t.TempDir()
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	first := catalogRecord("lockwood-catalog-0001", "sorna", "behavioral-verification", "application/json", "one.json")
	second := catalogRecord("lockwood-catalog-0002", "sorna", "mutation-campaign", "application/json", "two.json")
	third := catalogRecord("lockwood-catalog-0003", "paddock", "architecture", "text/plain", "three.txt")
	second.Parents = []custody.Lineage{{Relation: custody.References, Digest: first.Artifact.Digest}}
	for _, record := range []custody.Record{first, second, third} {
		if err := records.Put(record); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := New(records)
	if err != nil {
		t.Fatal(err)
	}

	inspected, err := catalog.Inspect(first.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Artifact.LogicalName != "one.json" {
		t.Fatalf("inspected record = %+v", inspected)
	}

	results, err := catalog.Find(Query{ProducerTool: "sorna", MediaType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].CustodyID != first.CustodyID || results[1].CustodyID != second.CustodyID {
		t.Fatalf("filtered results = %+v", results)
	}

	results, err = catalog.Find(Query{ProducerKind: "architecture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].CustodyID != third.CustodyID {
		t.Fatalf("kind results = %+v", results)
	}

	results, err = catalog.Find(Query{ParentDigest: first.Artifact.Digest, ParentRelation: custody.References})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].CustodyID != second.CustodyID {
		t.Fatalf("lineage results = %+v", results)
	}
}

func TestCatalogFailsClosedOnCorruptRecord(t *testing.T) {
	root := t.TempDir()
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	record := catalogRecord("lockwood-catalog-corrupt", "sorna", "behavioral-verification", "application/json", "corrupt.json")
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	path := root + "/records/" + record.CustodyID + ".json"
	if err := os.WriteFile(path, []byte(`{"schema":"unknown"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := New(records)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Find(Query{}); err == nil || !strings.Contains(err.Error(), "unexpected custody schema") {
		t.Fatalf("Find error = %v, want corrupt-record error", err)
	}
}

func TestCatalogFindsRemoteSourceMetadata(t *testing.T) {
	root := t.TempDir()
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	record := catalogRecord("lockwood-catalog-remote", "sorna", "remote", "application/json", "remote.json")
	record.Schema = custody.SchemaV2
	record.Source = custody.Source{
		URI:     "s3://evidence.example/runs/run-0001/remote.json",
		Version: "version-0001",
	}
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	catalog, err := New(records)
	if err != nil {
		t.Fatal(err)
	}
	results, err := catalog.Find(Query{
		SourceURI:     record.Source.URI,
		SourceVersion: record.Source.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].CustodyID != record.CustodyID {
		t.Fatalf("remote source results = %+v", results)
	}
}

func TestCatalogSupportsMemoryRecordStore(t *testing.T) {
	records := custody.NewMemory()
	record := catalogRecord("lockwood-catalog-memory", "example", "fixture", "text/plain", "memory.txt")
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	catalog, err := New(records)
	if err != nil {
		t.Fatal(err)
	}
	results, err := catalog.Find(Query{CustodyID: record.CustodyID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].CustodyID != record.CustodyID {
		t.Fatalf("memory catalog results = %+v", results)
	}
}

func catalogRecord(id, tool, kind, mediaType, logicalName string) custody.Record {
	record := custody.Record{
		Schema:     custody.Schema,
		CustodyID:  id,
		Status:     custody.Accepted,
		Artifact:   artifactReference(mediaType, logicalName),
		ReceivedAt: fixedTime,
		Producer:   custody.Producer{Tool: tool, Kind: kind},
		Custodian:  custody.Custodian{Tool: "lockwood"},
		Source:     custody.Source{RunID: id, Path: logicalName},
		Integrity:  custody.Integrity{Status: custody.IntegrityVerified, Method: "sha256", VerifiedAt: &fixedTime},
		Parents:    []custody.Lineage{},
		Handling:   custody.Handling{Redaction: "none", RetentionClass: "default"},
	}
	return record
}

func artifactReference(mediaType, logicalName string) artifact.Reference {
	data := []byte(logicalName)
	return artifact.Reference{
		Schema:      artifact.Schema,
		Digest:      artifact.DigestBytes(data),
		SizeBytes:   int64(len(data)),
		MediaType:   mediaType,
		LogicalName: logicalName,
	}
}
