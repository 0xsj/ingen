package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ingen/hammond/internal/governance"
)

const contractDigest = "6b40dfb15fa67f96c9f3bc79bc46206d45f6d44124197b344757499299e43445"

func TestFileStoreRegistersAppendsAndLists(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := contractDigest
	record := registeredRecord(digest)

	if err := fileStore.Register(record); err != nil {
		t.Fatal(err)
	}
	updated, err := fileStore.AppendEvent(record.Contract.Identity(), governance.Event{
		ID:            "event-002",
		Type:          governance.EventReviewOpened,
		Actor:         "owner",
		ReviewCycleID: "review-001",
		At:            "2026-09-15T00:01:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != governance.StateInReview {
		t.Fatalf("state = %q, want in_review", updated.State)
	}

	if _, err := fileStore.AppendEvent(record.Contract.Identity(), governance.Event{
		ID:             "event-003",
		Type:           governance.EventApprovalRecorded,
		Actor:          "reviewer",
		Role:           "product-reviewer",
		ReviewCycleID:  "review-001",
		Decision:       governance.DecisionApprove,
		ArtifactSHA256: digest,
		At:             "2026-09-15T00:02:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := fileStore.Get(record.Contract.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != governance.StateApproved || len(loaded.Events) != 3 {
		t.Fatalf("loaded record = %#v, want approved record with three events", loaded)
	}

	records, err := fileStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Contract.Identity().Key() != record.Contract.Identity().Key() {
		t.Fatalf("records = %#v, want one registered identity", records)
	}
}

func TestFileStoreRejectsDuplicateRegistrationAndDuplicateEvent(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := registeredRecord(contractDigest)
	if err := fileStore.Register(record); err != nil {
		t.Fatal(err)
	}
	if err := fileStore.Register(record); err == nil {
		t.Fatal("duplicate registration succeeded")
	}

	event := governance.Event{ID: "event-002", Type: governance.EventReviewOpened, Actor: "owner", ReviewCycleID: "review-001", At: "2026-09-15T00:01:00Z"}
	if _, err := fileStore.AppendEvent(record.Contract.Identity(), event); err != nil {
		t.Fatal(err)
	}
	if _, err := fileStore.AppendEvent(record.Contract.Identity(), event); err == nil {
		t.Fatal("duplicate event append succeeded")
	}
	if _, err := fileStore.AppendEvent(record.Contract.Identity(), governance.Event{ID: "event-003", Type: governance.EventAmendmentCreated}); err == nil || !strings.Contains(err.Error(), "coordinated store operation") {
		t.Fatalf("amendment append error = %v, want coordinated operation error", err)
	}
	if _, err := fileStore.AppendEvent(record.Contract.Identity(), governance.Event{ID: "event-004", Type: governance.EventSuperseded}); err == nil || !strings.Contains(err.Error(), "coordinated store operation") {
		t.Fatalf("supersession append error = %v, want coordinated operation error", err)
	}
}

func TestFileStoreDoesNotPersistInvalidAppend(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := registeredRecord(contractDigest)
	if err := fileStore.Register(record); err != nil {
		t.Fatal(err)
	}
	_, err = fileStore.AppendEvent(record.Contract.Identity(), governance.Event{
		ID:             "event-002",
		Type:           governance.EventApprovalRecorded,
		Actor:          "reviewer",
		Role:           "product-reviewer",
		ReviewCycleID:  "review-001",
		Decision:       governance.DecisionApprove,
		ArtifactSHA256: record.Contract.Artifact.SHA256,
		At:             "2026-09-15T00:01:00Z",
	})
	if err == nil {
		t.Fatal("invalid approval append succeeded")
	}

	loaded, err := fileStore.Get(record.Contract.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 || loaded.State != governance.StateRegistered {
		t.Fatalf("loaded record = %#v, want unchanged registration", loaded)
	}
}

func TestFileStoreConditionalAppendRejectsStaleRevision(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := registeredRecord(contractDigest)
	if err := fileStore.Register(record); err != nil {
		t.Fatal(err)
	}
	readBeforeUpdate, err := fileStore.Get(record.Contract.Identity())
	if err != nil {
		t.Fatal(err)
	}
	staleRevision, err := RecordRevision(readBeforeUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileStore.AppendEvent(record.Contract.Identity(), governance.Event{
		ID:            "event-002",
		Type:          governance.EventReviewOpened,
		Actor:         "owner",
		ReviewCycleID: "review-001",
		At:            "2026-09-15T00:01:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	approval := governance.Event{
		ID:             "event-003",
		Type:           governance.EventApprovalRecorded,
		Actor:          "reviewer@example.test",
		Role:           "product-reviewer",
		ReviewCycleID:  "review-001",
		Decision:       governance.DecisionApprove,
		ArtifactSHA256: contractDigest,
		At:             "2026-09-15T00:02:00Z",
	}
	if _, err := fileStore.AppendEventIfRevision(record.Contract.Identity(), staleRevision, approval); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("stale append error = %v, want ErrConflict", err)
	}
	unchanged, err := fileStore.Get(record.Contract.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.Events) != 2 || unchanged.State != governance.StateInReview {
		t.Fatalf("record after stale append = %#v, want unchanged in-review record", unchanged)
	}

	freshRevision, err := RecordRevision(unchanged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileStore.AppendEventIfRevision(record.Contract.Identity(), freshRevision, approval); err != nil {
		t.Fatalf("fresh conditional append error = %v", err)
	}
}

func TestFileStoreCreatesAmendmentAndPreservesLineage(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digestOne := contractDigest
	digestTwo := contractDigest
	predecessor := registeredRecord(digestOne)
	if err := fileStore.Register(predecessor); err != nil {
		t.Fatal(err)
	}
	current, err := fileStore.AppendEvent(predecessor.Contract.Identity(), governance.Event{
		ID:            "event-002",
		Type:          governance.EventReviewOpened,
		Actor:         "owner",
		ReviewCycleID: "review-001",
		At:            "2026-09-15T00:01:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = fileStore.AppendEvent(predecessor.Contract.Identity(), governance.Event{
		ID:             "event-003",
		Type:           governance.EventApprovalRecorded,
		Actor:          "reviewer",
		Role:           "product-reviewer",
		ReviewCycleID:  "review-001",
		Decision:       governance.DecisionApprove,
		ArtifactSHA256: digestOne,
		At:             "2026-09-15T00:02:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	successor := registeredRecord(digestTwo)
	successor.RecordID = "document-pipeline-v2"
	successor.Contract.Version = 2
	event, err := governance.BuildAmendmentEvent(current, successor, "event-004", "owner", "2026-09-15T00:03:00Z", governance.AmendmentClarifying, "Clarify the public description.")
	if err != nil {
		t.Fatal(err)
	}
	currentRevision, err := RecordRevision(current)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := fileStore.CreateAmendmentIfRevision(current.Contract.Identity(), currentRevision, successor, event)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != governance.StateApproved || len(updated.Events) != 4 {
		t.Fatalf("updated predecessor = %#v, want approved record with four events", updated)
	}
	successorIdentity := successor.Contract.Identity()
	if _, err := fileStore.AppendEvent(successorIdentity, governance.Event{
		ID:            "event-002",
		Type:          governance.EventReviewOpened,
		Actor:         "owner",
		ReviewCycleID: "review-002",
		At:            "2026-09-15T00:04:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	approvedSuccessor, err := fileStore.AppendEvent(successorIdentity, governance.Event{
		ID:             "event-003",
		Type:           governance.EventApprovalRecorded,
		Actor:          "reviewer",
		Role:           "product-reviewer",
		ReviewCycleID:  "review-002",
		Decision:       governance.DecisionApprove,
		ArtifactSHA256: digestTwo,
		At:             "2026-09-15T00:05:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	supersededEvent, err := governance.BuildSupersededEvent(updated, approvedSuccessor, "event-005", "owner", "2026-09-15T00:06:00Z")
	if err != nil {
		t.Fatal(err)
	}
	updatedRevision, err := RecordRevision(updated)
	if err != nil {
		t.Fatal(err)
	}
	superseded, err := fileStore.SupersedeIfRevision(updated.Contract.Identity(), updatedRevision, successorIdentity, supersededEvent)
	if err != nil {
		t.Fatal(err)
	}
	if superseded.State != governance.StateSuperseded {
		t.Fatalf("superseded predecessor state = %q, want superseded", superseded.State)
	}

	records, err := fileStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want predecessor and successor", len(records))
	}
	if err := governance.ValidateLineage(records); err != nil {
		t.Fatal(err)
	}
}

func TestFileStoreRejectsAmendmentBeforeSuccessorRegistration(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digestOne := contractDigest
	digestTwo := contractDigest
	predecessor := registeredRecord(digestOne)
	if err := fileStore.Register(predecessor); err != nil {
		t.Fatal(err)
	}
	current, err := fileStore.AppendEvent(predecessor.Contract.Identity(), governance.Event{
		ID:            "event-002",
		Type:          governance.EventReviewOpened,
		Actor:         "owner",
		ReviewCycleID: "review-001",
		At:            "2026-09-15T00:01:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = fileStore.AppendEvent(predecessor.Contract.Identity(), governance.Event{
		ID:             "event-003",
		Type:           governance.EventApprovalRecorded,
		Actor:          "reviewer",
		Role:           "product-reviewer",
		ReviewCycleID:  "review-001",
		Decision:       governance.DecisionApprove,
		ArtifactSHA256: digestOne,
		At:             "2026-09-15T00:02:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	successor := registeredRecord(digestTwo)
	successor.RecordID = "document-pipeline-v2"
	successor.Contract.Version = 2
	successor.Events[0].At = "2026-09-15T00:04:00Z"
	event, err := governance.BuildAmendmentEvent(current, successor, "event-004", "owner", "2026-09-15T00:03:00Z", governance.AmendmentClarifying, "Clarify the public description.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileStore.CreateAmendment(current.Contract.Identity(), successor, event); err == nil || !strings.Contains(err.Error(), "successor must be registered before amendment") {
		t.Fatalf("error = %v, want registration ordering error", err)
	}
	records, err := fileStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].State != governance.StateApproved || len(records[0].Events) != 3 {
		t.Fatalf("records = %#v, want unchanged predecessor only", records)
	}
}

func TestFileStoreReturnsNotFound(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = fileStore.Get(governance.ContractIdentity{ProjectID: "missing", ID: "missing", Version: 1, Schema: "sorna.contract/v1", ArtifactSHA256: strings.Repeat("a", 64)})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestFileStoreCoordinatesLocksAcrossInstances(t *testing.T) {
	root := t.TempDir()
	first, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}

	unlockFirst, err := first.lockFile(false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlockFirst != nil {
			unlockFirst()
		}
	}()

	type lockResult struct {
		unlock func()
		err    error
	}
	result := make(chan lockResult, 1)
	started := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		close(started)
		unlock, err := second.lockFile(false)
		result <- lockResult{unlock: unlock, err: err}
	}()
	<-started

	select {
	case <-result:
		t.Fatal("second store acquired the lock while first store held it")
	case <-time.After(50 * time.Millisecond):
	}

	unlockFirst()
	unlockFirst = nil
	select {
	case acquired := <-result:
		if acquired.err != nil {
			t.Fatal(acquired.err)
		}
		acquired.unlock()
	case <-time.After(time.Second):
		t.Fatal("second store did not acquire the lock after release")
	}
	wait.Wait()
}

func registeredRecord(digest string) governance.Record {
	return governance.Record{
		Schema:   governance.Schema,
		RecordID: "document-pipeline-v1",
		Policy:   governance.DefaultReviewPolicy().Reference,
		Contract: governance.ContractReference{
			ProjectID: "document-pipeline",
			ID:        "document-pipeline",
			Version:   1,
			Schema:    "sorna.contract/v1",
			Artifact: governance.Artifact{
				URI:    "hammond/examples/document-pipeline/contract-v2.canonical.json",
				SHA256: digest,
			},
		},
		State: governance.StateRegistered,
		Events: []governance.Event{
			{ID: "event-001", Type: governance.EventRegistered, Actor: "owner", At: "2026-09-15T00:00:00Z"},
		},
	}
}
