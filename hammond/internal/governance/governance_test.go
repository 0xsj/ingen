package governance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRecordValidateApprovedContract(t *testing.T) {
	digest := strings.Repeat("a", 64)
	record := Record{
		Schema:   Schema,
		RecordID: "document-pipeline-v1",
		Contract: contractReference(digest, 1),
		Policy:   DefaultReviewPolicy().Reference,
		State:    StateApproved,
		Events: []Event{
			{ID: "event-001", Type: EventRegistered, Actor: "owner", At: "2026-09-15T00:00:00Z"},
			{ID: "event-002", Type: EventReviewOpened, Actor: "owner", At: "2026-09-15T00:01:00Z", ReviewCycleID: "review-001"},
			{ID: "event-003", Type: EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:02:00Z"},
		},
	}

	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordValidateRejectsDecisionFromEarlierReviewCycle(t *testing.T) {
	digest := strings.Repeat("a", 64)
	record := Record{
		Schema:   Schema,
		RecordID: "document-pipeline-v1",
		Contract: contractReference(digest, 1),
		Policy:   DefaultReviewPolicy().Reference,
		State:    StateApproved,
		Events: []Event{
			{ID: "event-001", Type: EventRegistered, Actor: "owner", At: "2026-09-15T00:00:00Z"},
			{ID: "event-002", Type: EventReviewOpened, Actor: "owner", ReviewCycleID: "review-001", At: "2026-09-15T00:01:00Z"},
			{ID: "event-003", Type: EventRejectionRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionReject, ArtifactSHA256: digest, Reason: "Needs clarification.", At: "2026-09-15T00:02:00Z"},
			{ID: "event-004", Type: EventReviewOpened, Actor: "owner", ReviewCycleID: "review-002", At: "2026-09-15T00:03:00Z"},
			{ID: "event-005", Type: EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:04:00Z"},
		},
	}

	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "must match the active review cycle") {
		t.Fatalf("error = %v, want stale review-cycle error", err)
	}
}

func TestRecordValidateRejectsMismatchedReviewPolicy(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	policy := DefaultReviewPolicy()
	policy.Reference.ID = "two-approval"
	policy.Reference.Artifact.SHA256 = strings.Repeat("b", 64)
	policy.MinimumApprovals = 2

	if err := record.ValidateWithPolicy(policy); err == nil || !strings.Contains(err.Error(), "policy must match the supplied review policy") {
		t.Fatalf("error = %v, want policy identity mismatch", err)
	}
}

func TestRecordValidateRejectsUnauthorizedPolicyActor(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	record.Events[2].Actor = "unlisted-reviewer"

	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "is not authorized for role") {
		t.Fatalf("error = %v, want actor-role authorization error", err)
	}
}

func TestAppendEventWithPolicyWaitsForDistinctApprovals(t *testing.T) {
	digest := strings.Repeat("a", 64)
	policy := ReviewPolicy{
		Reference: PolicyReference{
			ID:      "two-approval",
			Version: 1,
			Schema:  PolicySchema,
			Artifact: Artifact{
				URI:    "testdata/two-approval-policy.json",
				SHA256: strings.Repeat("b", 64),
			},
		},
		MinimumApprovals: 2,
	}
	record := registeredRecord(digest)
	record.Policy = policy.Reference
	var err error
	record, err = record.AppendEventWithPolicy(Event{
		ID: "event-002", Type: EventReviewOpened, Actor: "owner", ReviewCycleID: "review-001", At: "2026-09-15T00:01:00Z",
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.AppendEventWithPolicy(Event{
		ID: "event-003", Type: EventApprovalRecorded, Actor: "reviewer-one", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:02:00Z",
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != StateInReview {
		t.Fatalf("state after first approval = %q, want in_review", record.State)
	}
	record, err = record.AppendEventWithPolicy(Event{
		ID: "event-004", Type: EventApprovalRecorded, Actor: "reviewer-one", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:03:00Z",
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != StateInReview {
		t.Fatalf("state after repeated approval = %q, want in_review", record.State)
	}
	record, err = record.AppendEventWithPolicy(Event{
		ID: "event-005", Type: EventApprovalRecorded, Actor: "reviewer-two", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:04:00Z",
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != StateApproved {
		t.Fatalf("state after distinct approvals = %q, want approved", record.State)
	}
}

func TestAppendEventWithPolicyRequiresRoles(t *testing.T) {
	digest := strings.Repeat("a", 64)
	policy := ReviewPolicy{
		Reference: PolicyReference{
			ID:      "product-security-approval",
			Version: 1,
			Schema:  PolicySchema,
			Artifact: Artifact{
				URI:    "testdata/product-security-policy.json",
				SHA256: strings.Repeat("c", 64),
			},
		},
		MinimumApprovals: 2,
		RequiredRoles:    []string{"product-reviewer", "security-reviewer"},
	}
	record := registeredRecord(digest)
	record.Policy = policy.Reference
	var err error
	record, err = record.AppendEventWithPolicy(Event{
		ID: "event-002", Type: EventReviewOpened, Actor: "owner", ReviewCycleID: "review-001", At: "2026-09-15T00:01:00Z",
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []Event{
		{ID: "event-003", Type: EventApprovalRecorded, Actor: "product-reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:02:00Z"},
		{ID: "event-004", Type: EventApprovalRecorded, Actor: "second-product-reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:03:00Z"},
	} {
		record, err = record.AppendEventWithPolicy(event, policy)
		if err != nil {
			t.Fatal(err)
		}
	}
	if record.State != StateInReview {
		t.Fatalf("state without security approval = %q, want in_review", record.State)
	}
	record, err = record.AppendEventWithPolicy(Event{
		ID: "event-005", Type: EventApprovalRecorded, Actor: "security-reviewer", Role: "security-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:04:00Z",
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != StateApproved {
		t.Fatalf("state with required roles = %q, want approved", record.State)
	}
}

func TestRecordValidateRejectsApprovalForDifferentArtifact(t *testing.T) {
	digest := strings.Repeat("a", 64)
	record := approvedRecord(digest)
	record.Events[2].ArtifactSHA256 = strings.Repeat("b", 64)

	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "must match contract.artifact.sha256") {
		t.Fatalf("error = %v, want mismatched digest error", err)
	}
}

func TestRecordValidateRejectsInvalidLifecycleTransition(t *testing.T) {
	digest := strings.Repeat("a", 64)
	record := approvedRecord(digest)
	record.Events = []Event{
		record.Events[0],
		record.Events[2],
	}

	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "only valid from in_review") {
		t.Fatalf("error = %v, want lifecycle error", err)
	}
}

func TestRecordValidateRequiresMaterializedState(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	record.State = StateInReview

	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), `state is "in_review" but events derive "approved"`) {
		t.Fatalf("error = %v, want state mismatch error", err)
	}
}

func TestDecodeRecordRejectsUnknownFields(t *testing.T) {
	data, err := json.Marshal(approvedRecord(strings.Repeat("a", 64)))
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"unexpected":true}`)...)

	if _, err := DecodeRecord(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown-field error", err)
	}
}

func TestDecodeRecordLoadsReferencedPolicy(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRecord(data); err != nil {
		t.Fatal(err)
	}

	record.Policy.Artifact.SHA256 = strings.Repeat("b", 64)
	data, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRecord(data); err == nil || !strings.Contains(err.Error(), "do not match reference artifact.sha256") {
		t.Fatalf("error = %v, want policy digest error", err)
	}
}

func TestDecodeEventRejectsUnknownFields(t *testing.T) {
	data := []byte(`{"id":"event-001","type":"registered","actor":"owner","at":"2026-09-15T00:00:00Z","unexpected":true}`)
	if _, err := DecodeEvent(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown-field error", err)
	}
}

func TestLoadReviewPolicyVerifiesPolicyBytes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "policy.json")
	data := []byte("{\n  \"schema\": \"ingen.hammond-review-policy/v1\",\n  \"id\": \"two-approval\",\n  \"version\": 1,\n  \"minimum_approvals\": 2\n}\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	reference := PolicyReference{
		ID:      "two-approval",
		Version: 1,
		Schema:  PolicySchema,
		Artifact: Artifact{
			URI:    path,
			SHA256: "453139d7edd9405759305f01c2109b35dca672910b0a8eb492c7e36805969b35",
		},
	}
	policy, err := LoadReviewPolicy(reference)
	if err != nil {
		t.Fatal(err)
	}
	if policy.MinimumApprovals != 2 || !policy.Reference.Equal(reference) {
		t.Fatalf("policy = %#v, want loaded two-approval policy", policy)
	}

	data = append(data[:len(data)-2], []byte("  \n}\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReviewPolicy(reference); err == nil || !strings.Contains(err.Error(), "do not match reference artifact.sha256") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
}

func TestValidateLineageRejectsCycle(t *testing.T) {
	digestOne := strings.Repeat("a", 64)
	digestTwo := strings.Repeat("b", 64)
	identityOne := contractReference(digestOne, 1).Identity()
	identityTwo := contractReference(digestTwo, 2).Identity()

	first := approvedRecord(digestOne)
	first.Events = append(first.Events, Event{
		ID: "event-004", Type: EventSuperseded, Actor: "owner", At: "2026-09-15T00:03:00Z", Successor: &identityTwo,
	})
	first.State = StateSuperseded

	second := approvedRecord(digestTwo)
	second.RecordID = "document-pipeline-v2"
	second.Contract = contractReference(digestTwo, 2)
	second.Events = append(second.Events, Event{
		ID: "event-004", Type: EventSuperseded, Actor: "owner", At: "2026-09-15T00:03:00Z", Successor: &identityOne,
	})
	second.State = StateSuperseded

	if err := ValidateLineage([]Record{first, second}); err == nil || !strings.Contains(err.Error(), "lineage contains a cycle") {
		t.Fatalf("error = %v, want lineage cycle error", err)
	}
}

func TestValidateLineageRejectsAmendmentOnWrongRecord(t *testing.T) {
	predecessor := approvedRecord(strings.Repeat("a", 64))
	registeredSuccessor := registeredRecordForVersion(strings.Repeat("b", 64), 2)
	amendment, err := BuildAmendmentEvent(predecessor, registeredSuccessor, "event-004", "owner", "2026-09-15T00:03:00Z", AmendmentClarifying, "Clarify the public description.")
	if err != nil {
		t.Fatal(err)
	}
	misplacedSuccessor := registeredSuccessor
	misplacedSuccessor.State = StateApproved
	misplacedSuccessor.Events = append(misplacedSuccessor.Events,
		Event{ID: "event-002", Type: EventReviewOpened, Actor: "owner", ReviewCycleID: "review-002", At: "2026-09-15T00:01:00Z"},
		Event{ID: "event-003", Type: EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-002", Decision: DecisionApprove, ArtifactSHA256: strings.Repeat("b", 64), At: "2026-09-15T00:02:00Z"},
	)
	misplacedSuccessor, err = misplacedSuccessor.AppendEvent(amendment)
	if err != nil {
		t.Fatal(err)
	}

	if err := ValidateLineage([]Record{predecessor, misplacedSuccessor}); err == nil || !strings.Contains(err.Error(), "does not match its record") {
		t.Fatalf("error = %v, want amendment ownership error", err)
	}
}

func TestValidateLineageRejectsSupersessionBeforeSuccessorApproval(t *testing.T) {
	predecessor := approvedRecord(strings.Repeat("a", 64))
	successor := registeredRecordForVersion(strings.Repeat("b", 64), 2)
	amendment, err := BuildAmendmentEvent(predecessor, successor, "event-004", "owner", "2026-09-15T00:03:00Z", AmendmentClarifying, "Clarify the public description.")
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err = predecessor.AppendEvent(amendment)
	if err != nil {
		t.Fatal(err)
	}
	successorIdentity := successor.Contract.Identity()
	supersededEvent := Event{ID: "event-005", Type: EventSuperseded, Actor: "owner", At: "2026-09-15T00:04:00Z", Successor: &successorIdentity}
	predecessor, err = predecessor.AppendEvent(supersededEvent)
	if err != nil {
		t.Fatal(err)
	}

	if err := ValidateLineage([]Record{predecessor, successor}); err == nil || !strings.Contains(err.Error(), "successor must be approved before supersession") {
		t.Fatalf("error = %v, want successor approval ordering error", err)
	}
}

func TestValidateLineageRejectsSupersessionWithoutAmendment(t *testing.T) {
	predecessor := approvedRecord(strings.Repeat("a", 64))
	successor := approvedRecord(strings.Repeat("b", 64))
	successor.RecordID = "document-pipeline-v2"
	successor.Contract = contractReference(strings.Repeat("b", 64), 2)
	successorIdentity := successor.Contract.Identity()
	predecessor, err := predecessor.AppendEvent(Event{
		ID: "event-004", Type: EventSuperseded, Actor: "owner", At: "2026-09-15T00:03:00Z", Successor: &successorIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := ValidateLineage([]Record{predecessor, successor}); err == nil || !strings.Contains(err.Error(), "supersession has no amendment link") {
		t.Fatalf("error = %v, want missing amendment link error", err)
	}
}

func TestBuildAmendmentEventRequiresApprovedPredecessorAndRegisteredSuccessor(t *testing.T) {
	digestOne := strings.Repeat("a", 64)
	digestTwo := strings.Repeat("b", 64)
	predecessor := approvedRecord(digestOne)
	successor := registeredRecordForVersion(digestTwo, 2)

	event, err := BuildAmendmentEvent(predecessor, successor, "event-004", "owner", "2026-09-15T00:03:00Z", AmendmentClarifying, "Clarify the public description.")
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != EventAmendmentCreated || event.Predecessor == nil || event.Successor == nil {
		t.Fatalf("event = %#v, want linked amendment event", event)
	}

	predecessor.State = StateInReview
	predecessor.Events = predecessor.Events[:2]
	if _, err := BuildAmendmentEvent(predecessor, successor, "event-005", "owner", "2026-09-15T00:04:00Z", AmendmentClarifying, "not allowed"); err == nil || !strings.Contains(err.Error(), "predecessor must be approved") {
		t.Fatalf("error = %v, want approved predecessor error", err)
	}

	successor.State = StateInReview
	successor.Events = append(successor.Events, Event{
		ID:            "event-002",
		Type:          EventReviewOpened,
		Actor:         "owner",
		ReviewCycleID: "review-002",
		At:            "2026-09-15T00:01:00Z",
	})
	if _, err := BuildAmendmentEvent(approvedRecord(digestOne), successor, "event-006", "owner", "2026-09-15T00:05:00Z", AmendmentClarifying, "not independently registered"); err == nil || !strings.Contains(err.Error(), "independently registered") {
		t.Fatalf("error = %v, want independently registered successor error", err)
	}
}

func TestBuildSupersededEventLinksApprovedPredecessor(t *testing.T) {
	predecessor := approvedRecord(strings.Repeat("a", 64))
	registeredSuccessor := registeredRecordForVersion(strings.Repeat("b", 64), 2)
	amendment, err := BuildAmendmentEvent(predecessor, registeredSuccessor, "event-004", "owner", "2026-09-15T00:03:00Z", AmendmentClarifying, "Clarify the public description.")
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err = predecessor.AppendEvent(amendment)
	if err != nil {
		t.Fatal(err)
	}
	successor := approvedRecord(strings.Repeat("b", 64))
	successor.RecordID = "document-pipeline-v2"
	successor.Contract = contractReference(strings.Repeat("b", 64), 2)
	event, err := BuildSupersededEvent(predecessor, successor, "event-005", "owner", "2026-09-15T00:04:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != EventSuperseded || event.Successor == nil {
		t.Fatalf("event = %#v, want superseded successor event", event)
	}
}

func approvedRecord(digest string) Record {
	return Record{
		Schema:   Schema,
		RecordID: "document-pipeline-v1",
		Contract: contractReference(digest, 1),
		Policy:   DefaultReviewPolicy().Reference,
		State:    StateApproved,
		Events: []Event{
			{ID: "event-001", Type: EventRegistered, Actor: "owner", At: "2026-09-15T00:00:00Z"},
			{ID: "event-002", Type: EventReviewOpened, Actor: "owner", At: "2026-09-15T00:01:00Z", ReviewCycleID: "review-001"},
			{ID: "event-003", Type: EventApprovalRecorded, Actor: "reviewer", Role: "product-reviewer", ReviewCycleID: "review-001", Decision: DecisionApprove, ArtifactSHA256: digest, At: "2026-09-15T00:02:00Z"},
		},
	}
}

func registeredRecord(digest string) Record {
	return Record{
		Schema:   Schema,
		RecordID: "document-pipeline-v1",
		Contract: contractReference(digest, 1),
		Policy:   DefaultReviewPolicy().Reference,
		State:    StateRegistered,
		Events: []Event{
			{ID: "event-001", Type: EventRegistered, Actor: "owner", At: "2026-09-15T00:00:00Z"},
		},
	}
}

func contractReference(digest string, version int) ContractReference {
	return ContractReference{
		ProjectID: "document-pipeline",
		ID:        "document-pipeline",
		Version:   version,
		Schema:    "sorna.contract/v1",
		Artifact: Artifact{
			URI:    "examples/document-pipeline-lab/contract/contract.yaml",
			SHA256: digest,
		},
	}
}

func registeredRecordForVersion(digest string, version int) Record {
	record := registeredRecord(digest)
	record.RecordID = "document-pipeline-v" + strconv.Itoa(version)
	record.Contract = contractReference(digest, version)
	return record
}
