package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/nublar/internal/run"
)

func TestStoreSavesAndLoadsImmutableRun(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	record := testRun("run-01")
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != record.RunID || loaded.Workflow.ID != record.Workflow.ID || loaded.Checks[0].Result.Artifact.Tool != "sorna" {
		t.Fatalf("loaded run = %+v, want stored run", loaded)
	}
	if err := store.Save(record); err == nil {
		t.Fatal("second save succeeded, want immutable duplicate-ID rejection")
	}
}

func TestStoreDoesNotUseRunIDAsFilesystemPath(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	record := testRun("../../outside")
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Dir(filepath.Join(root, entries[0].Name())) != root {
		t.Fatalf("store entries = %+v, want one file directly under root", entries)
	}
}

func TestStoreRejectsInvalidRunBeforeCreatingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run.Run{}); err == nil {
		t.Fatal("invalid run saved, want validation error")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("store root stat error = %v, want root to remain absent", err)
	}
}

func TestStoreListsNewestFirstAndIgnoresNonCanonicalFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runs")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	older := testRun("run-older")
	newer := testRun("run-newer")
	older.CreatedAt = "2026-09-15T12:00:00Z"
	older.CompletedAt = "2026-09-15T12:00:01Z"
	newer.CreatedAt = "2026-09-15T13:00:00Z"
	newer.CompletedAt = "2026-09-15T13:00:01Z"
	if err := store.Save(older); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(newer); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.json"), []byte("not a run"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".nublar-run-partial"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].RunID != "run-newer" || records[1].RunID != "run-older" {
		t.Fatalf("listed runs = %+v, want newest-first canonical records", records)
	}
}

func TestStoreListsMissingRootAsEmpty(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if records == nil || len(records) != 0 {
		t.Fatalf("listed runs = %#v, want non-nil empty list", records)
	}
}

func TestStoreListRejectsCorruptCanonicalFile(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path := store.pathFor("run-corrupt")
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("List() succeeded, want corrupt canonical file error")
	}
}

func TestStoreListRejectsFilenameIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path := store.pathFor("run-expected")
	contents, err := json.Marshal(testRun("run-other"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("List() succeeded, want filename identity mismatch error")
	}
}

func testRun(runID string) run.Run {
	return run.Run{
		Schema:      run.Schema,
		RunID:       runID,
		Workflow:    run.Workflow{ID: "test-workflow", File: fileRef("workflow.yaml")},
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		CompletedAt: "2026-09-15T12:00:01Z",
		Checks: []run.Check{{
			ID:       "behavior",
			Tool:     "sorna",
			Path:     "sorna.json",
			Required: true,
			Status:   "passed",
			Result: &run.Result{
				Ref: fileRef("sorna.json"),
				Artifact: ciresult.Artifact{
					Schema:      ciresult.Schema,
					Tool:        "sorna",
					Kind:        "test",
					Status:      "passed",
					ExitCode:    0,
					CreatedAt:   "2026-09-15T12:00:00Z",
					Source:      ciresult.Source{Root: "."},
					Report:      []byte(`{"ok":true}`),
					Explanation: []byte(`{"ok":true}`),
				},
			},
		}},
	}
}

func fileRef(path string) ciresult.FileRef {
	return ciresult.FileRef{Path: path, SHA256: strings.Repeat("a", 64)}
}
