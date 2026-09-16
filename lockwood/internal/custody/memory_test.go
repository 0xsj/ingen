package custody

import (
	"strings"
	"sync"
	"testing"
)

func TestMemoryRecordStorePutGetListAndIdempotency(t *testing.T) {
	var records Memory
	first := testRecord(t, "lockwood-memory-record-0002")
	second := testRecord(t, "lockwood-memory-record-0001")
	second.Parents = []Lineage{}
	for _, record := range []Record{first, second} {
		if err := records.Put(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := records.Put(first); err != nil {
		t.Fatalf("identical record was not idempotent: %v", err)
	}

	got, err := records.Get(first.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	got.Parents[0].Digest = "sha256:" + strings.Repeat("c", 64)
	again, err := records.Get(first.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Parents[0].Digest == got.Parents[0].Digest {
		t.Fatal("Get exposed mutable parent state")
	}

	listed, err := records.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].CustodyID != second.CustodyID || listed[1].CustodyID != first.CustodyID {
		t.Fatalf("listed records = %+v, want deterministic ID order", listed)
	}

	conflict := first
	conflict.Source.Path = "different.json"
	if err := records.Put(conflict); err == nil || !strings.Contains(err.Error(), "different contents") {
		t.Fatalf("conflicting Put error = %v", err)
	}
}

func TestMemoryRecordStoreRejectsInvalidAndUnknownRecords(t *testing.T) {
	records := NewMemory()
	invalid := testRecord(t, "lockwood-memory-record-invalid")
	invalid.Integrity.Status = IntegrityNotChecked
	invalid.Integrity.VerifiedAt = nil
	if err := records.Put(invalid); err == nil || !strings.Contains(err.Error(), "verified integrity") {
		t.Fatalf("invalid Put error = %v", err)
	}
	if _, err := records.Get("lockwood-memory-record-missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing Get error = %v", err)
	}
}

func TestRecordStoreConcurrentIdempotentPut(t *testing.T) {
	for _, backend := range []struct {
		name string
		new  func(t *testing.T) RecordStore
	}{
		{
			name: "filesystem",
			new: func(t *testing.T) RecordStore {
				records, err := NewFilesystem(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				return records
			},
		},
		{name: "memory", new: func(t *testing.T) RecordStore { return NewMemory() }},
	} {
		t.Run(backend.name, func(t *testing.T) {
			records := backend.new(t)
			record := testRecord(t, "lockwood-concurrent-record")
			const writers = 16
			errs := make(chan error, writers)
			var wait sync.WaitGroup
			wait.Add(writers)
			for range writers {
				go func() {
					defer wait.Done()
					errs <- records.Put(record)
				}()
			}
			wait.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			listed, err := records.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(listed) != 1 || listed[0].CustodyID != record.CustodyID {
				t.Fatalf("concurrent records = %+v", listed)
			}
		})
	}
}
