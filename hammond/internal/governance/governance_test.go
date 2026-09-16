package governance

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestRecordValidateRejectsActorWhenAuthoritySnapshotHasNoGrant(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	policy := DefaultReviewPolicy()
	policy.ActorRoles = map[string][]string{}
	record.Policy = policy.Reference

	if err := record.ValidateWithPolicy(policy); err == nil || !strings.Contains(err.Error(), "is not authorized for role") {
		t.Fatalf("error = %v, want empty-authority denial", err)
	}
}

func TestRecordValidateUsesInjectedAuthorityVerifier(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	policy := DefaultReviewPolicy()
	policy.AuthorityVerifier = testAuthorityVerifier{allowed: false}

	if err := record.ValidateWithPolicy(policy); err == nil || !strings.Contains(err.Error(), "is not authorized for role") {
		t.Fatalf("error = %v, want injected authority denial", err)
	}

	policy.AuthorityVerifier = testAuthorityVerifier{allowed: true}
	if err := record.ValidateWithPolicy(policy); err != nil {
		t.Fatalf("error = %v, want injected authority approval", err)
	}

	policy.AuthorityVerifier = testAuthorityVerifier{err: errors.New("authority unavailable")}
	if err := record.ValidateWithPolicy(policy); err == nil || !strings.Contains(err.Error(), "authority verification failed") {
		t.Fatalf("error = %v, want authority verification failure", err)
	}
}

func TestRecordValidatePassesDecisionTimestampToAuthorityVerifier(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	policy := DefaultReviewPolicy()
	verifier := &recordingAuthorityVerifier{allowed: true}
	policy.AuthorityVerifier = verifier

	if err := record.ValidateWithPolicy(policy); err != nil {
		t.Fatalf("error = %v, want valid record", err)
	}
	if verifier.at != record.Events[2].At {
		t.Fatalf("authority timestamp = %q, want decision timestamp %q", verifier.at, record.Events[2].At)
	}
}

func TestRecordValidateDoesNotVerifyAuthorityWithInvalidTimestamp(t *testing.T) {
	record := approvedRecord(strings.Repeat("a", 64))
	record.Events[2].At = "not-a-timestamp"
	policy := DefaultReviewPolicy()
	verifier := &recordingAuthorityVerifier{allowed: true}
	policy.AuthorityVerifier = verifier

	if err := record.ValidateWithPolicy(policy); err == nil || !strings.Contains(err.Error(), "decision timestamp must be validated") {
		t.Fatalf("error = %v, want invalid timestamp error", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("authority verifier calls = %d, want zero for invalid timestamp", verifier.calls)
	}
}

func TestTimeScopedAuthorityHonorsGrantWindow(t *testing.T) {
	authority := TimeScopedAuthority{Grants: []AuthorityGrant{{
		Actor:      "reviewer@example.test",
		Role:       "product-reviewer",
		ValidFrom:  "2026-09-15T00:02:00Z",
		ValidUntil: "2026-09-15T01:00:00Z",
	}}}
	for _, test := range []struct {
		at         string
		authorized bool
	}{
		{at: "2026-09-15T00:01:59Z", authorized: false},
		{at: "2026-09-15T00:02:00Z", authorized: true},
		{at: "2026-09-15T00:59:59Z", authorized: true},
		{at: "2026-09-15T01:00:00Z", authorized: false},
	} {
		authorized, err := authority.Verify("reviewer@example.test", "product-reviewer", test.at)
		if err != nil {
			t.Fatalf("at %s: error = %v", test.at, err)
		}
		if authorized != test.authorized {
			t.Fatalf("at %s: authorized = %v, want %v", test.at, authorized, test.authorized)
		}
	}
}

func TestTimeScopedAuthorityRejectsInvalidGrantWindow(t *testing.T) {
	authority := TimeScopedAuthority{Grants: []AuthorityGrant{{
		Actor:      "reviewer@example.test",
		Role:       "product-reviewer",
		ValidFrom:  "2026-09-15T01:00:00Z",
		ValidUntil: "2026-09-15T00:00:00Z",
	}}}

	if err := authority.Validate(); err == nil || !strings.Contains(err.Error(), "valid_until must be after valid_from") {
		t.Fatalf("error = %v, want invalid grant window", err)
	}
}

func TestLoadMembershipSnapshotWithSignatureVerifier(t *testing.T) {
	providerPublicKey, providerPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := membershipDocument{
		Schema:    MembershipSchema,
		ID:        "directory-reviewers",
		Version:   1,
		IssuedAt:  "2026-09-15T00:00:00Z",
		ExpiresAt: "2026-09-15T02:00:00Z",
		Grants: []AuthorityGrant{
			{Actor: "reviewer@example.test", Role: "product-reviewer", ValidFrom: "2026-09-15T00:02:00Z", ValidUntil: "2026-09-15T01:00:00Z"},
			{Actor: "security@example.test", Role: "security-reviewer", ValidFrom: "2026-09-15T00:02:00Z"},
		},
	}
	payload, err := canonicalMembershipPayload(document)
	if err != nil {
		t.Fatal(err)
	}
	document.Signature = &AuthoritySignature{
		Algorithm: AuthoritySignatureAlgorithmEd25519,
		KeyID:     "directory-2026",
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(providerPrivateKey, payload)),
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "membership.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	reference := MembershipReference{
		ID:      document.ID,
		Version: document.Version,
		Schema:  MembershipSchema,
		Artifact: Artifact{
			URI:    path,
			SHA256: hex.EncodeToString(digest[:]),
		},
	}
	verifier := Ed25519AuthoritySignatureVerifier{Keys: map[string]ed25519.PublicKey{"directory-2026": providerPublicKey}}
	snapshot, err := LoadMembershipSnapshotWithSignatureVerifier(reference, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if authorized, err := snapshot.Verifier().Verify("reviewer@example.test", "product-reviewer", "2026-09-15T00:30:00Z"); err != nil || !authorized {
		t.Fatalf("membership grant = %v, %v; want authorized", authorized, err)
	}
	if authorized, err := snapshot.Verifier().Verify("reviewer@example.test", "product-reviewer", "2026-09-15T01:00:00Z"); err != nil || authorized {
		t.Fatalf("expired membership grant = %v, %v; want denied", authorized, err)
	}
	if _, err := snapshot.VerifierAt("2026-09-15T00:30:00Z", time.Hour, 5*time.Minute); err != nil {
		t.Fatalf("fresh membership snapshot error = %v", err)
	}
	if _, err := snapshot.VerifierAt("2026-09-15T03:00:00Z", 24*time.Hour, 5*time.Minute); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error = %v, want expired membership snapshot", err)
	}
	if _, err := snapshot.VerifierAt("2026-09-14T23:00:00Z", 24*time.Hour, 5*time.Minute); err == nil || !strings.Contains(err.Error(), "future-dated") {
		t.Fatalf("error = %v, want future-dated membership snapshot", err)
	}

	document.Grants[0].Role = "security-reviewer"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	tamperedDigest := sha256.Sum256(tampered)
	reference.Artifact.SHA256 = hex.EncodeToString(tamperedDigest[:])
	if _, err := LoadMembershipSnapshotWithSignatureVerifier(reference, verifier); err == nil || !strings.Contains(err.Error(), "authority signature is invalid") {
		t.Fatalf("error = %v, want invalid membership signature", err)
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
	record := approvedRecord("6b40dfb15fa67f96c9f3bc79bc46206d45f6d44124197b344757499299e43445")
	record.Contract.Artifact.URI = "hammond/examples/document-pipeline/contract-v2.canonical.json"
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

func TestLoadReviewPolicyLoadsAuthoritySnapshot(t *testing.T) {
	expected := DefaultReviewPolicy()
	loaded, err := LoadReviewPolicy(expected.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Authority.Equal(expected.Authority) {
		t.Fatalf("authority = %#v, want %#v", loaded.Authority, expected.Authority)
	}
	if authorized, err := loaded.authorizes("reviewer", "product-reviewer", "2026-09-15T00:02:00Z"); err != nil || !authorized {
		t.Fatal("loaded authority did not grant product-reviewer to reviewer")
	}
}

func TestLoadReviewAuthorityWithEd25519Signature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "authority.json")
	document := reviewAuthorityDocument{
		Schema:  AuthoritySchema,
		ID:      "signed-reviewers",
		Version: 1,
		Actors:  map[string][]string{"reviewer@example.test": {"product-reviewer"}},
	}
	payload, err := json.Marshal(reviewAuthoritySigningDocument{
		Schema:  document.Schema,
		ID:      document.ID,
		Version: document.Version,
		Actors:  document.Actors,
	})
	if err != nil {
		t.Fatal(err)
	}
	document.Signature = &AuthoritySignature{
		Algorithm: AuthoritySignatureAlgorithmEd25519,
		KeyID:     "test-authority",
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	reference := AuthorityReference{
		ID:      document.ID,
		Version: document.Version,
		Schema:  AuthoritySchema,
		Artifact: Artifact{
			URI:    path,
			SHA256: hex.EncodeToString(digest[:]),
		},
	}
	verifier := Ed25519AuthoritySignatureVerifier{Keys: map[string]ed25519.PublicKey{"test-authority": publicKey}}
	loaded, err := LoadReviewAuthorityWithSignatureVerifier(reference, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if authorized, err := loaded.Verify("reviewer@example.test", "product-reviewer", "2026-09-15T00:02:00Z"); err != nil || !authorized {
		t.Fatalf("loaded authority verification = %v, %v; want authorized", authorized, err)
	}

	policyPath := filepath.Join(directory, "policy.json")
	policyDocument := reviewPolicyDocument{
		Schema:           PolicySchema,
		ID:               "signed-authority-policy",
		Version:          1,
		MinimumApprovals: 1,
		Authority:        reference,
	}
	policyData, err := json.Marshal(policyDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, policyData, 0o644); err != nil {
		t.Fatal(err)
	}
	policyDigest := sha256.Sum256(policyData)
	policyReference := PolicyReference{
		ID:      policyDocument.ID,
		Version: policyDocument.Version,
		Schema:  PolicySchema,
		Artifact: Artifact{
			URI:    policyPath,
			SHA256: hex.EncodeToString(policyDigest[:]),
		},
	}
	if _, err := LoadReviewPolicyWithAuthoritySignatureVerifier(policyReference, verifier); err != nil {
		t.Fatalf("load signed authority through policy: %v", err)
	}

	document.Signature.Signature = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", ed25519.SignatureSize)))
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	tamperedDigest := sha256.Sum256(tampered)
	reference.Artifact.SHA256 = hex.EncodeToString(tamperedDigest[:])
	if _, err := LoadReviewAuthorityWithSignatureVerifier(reference, verifier); err == nil || !strings.Contains(err.Error(), "authority signature is invalid") {
		t.Fatalf("error = %v, want invalid signature", err)
	}
}

func TestLoadAuthorityTrustStoreSupportsKeyRotation(t *testing.T) {
	oldPublicKey, oldPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	newPublicKey, newPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := authorityTrustDocument{
		Schema:  AuthorityTrustSchema,
		ID:      "authority-keys",
		Version: 2,
		Keys: []authorityTrustKeyDocument{
			{ID: "authority-old", Algorithm: AuthoritySignatureAlgorithmEd25519, PublicKey: base64.StdEncoding.EncodeToString(oldPublicKey), Status: AuthorityTrustKeyRevoked},
			{ID: "authority-new", Algorithm: AuthoritySignatureAlgorithmEd25519, PublicKey: base64.StdEncoding.EncodeToString(newPublicKey), Status: AuthorityTrustKeyActive},
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "authority-trust.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	reference := AuthorityTrustReference{
		ID:      document.ID,
		Version: document.Version,
		Schema:  AuthorityTrustSchema,
		Artifact: Artifact{
			URI:    path,
			SHA256: hex.EncodeToString(digest[:]),
		},
	}
	store, err := LoadAuthorityTrustStore(reference)
	if err != nil {
		t.Fatal(err)
	}
	verifier := store.SignatureVerifier()
	payload := []byte("authority-payload")
	if err := verifier.VerifySignature("authority-new", payload, ed25519.Sign(newPrivateKey, payload)); err != nil {
		t.Fatalf("new key verification error = %v", err)
	}
	if err := verifier.VerifySignature("authority-old", payload, ed25519.Sign(oldPrivateKey, payload)); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("old key verification error = %v, want revoked key rejection", err)
	}
}

func TestLoadAuthorityTrustStoreWithRootSignature(t *testing.T) {
	rootPublicKey, rootPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	activePublicKey, activePrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := authorityTrustDocument{
		Schema:  AuthorityTrustSchema,
		ID:      "rooted-authority-keys",
		Version: 1,
		Keys: []authorityTrustKeyDocument{
			{ID: "authority-active", Algorithm: AuthoritySignatureAlgorithmEd25519, PublicKey: base64.StdEncoding.EncodeToString(activePublicKey), Status: AuthorityTrustKeyActive},
		},
	}
	payload, err := json.Marshal(authorityTrustSigningDocument{
		Schema:  document.Schema,
		ID:      document.ID,
		Version: document.Version,
		Keys:    document.Keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	document.Signature = &AuthoritySignature{
		Algorithm: AuthoritySignatureAlgorithmEd25519,
		KeyID:     "root-2026",
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(rootPrivateKey, payload)),
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "rooted-authority-trust.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	reference := AuthorityTrustReference{
		ID:      document.ID,
		Version: document.Version,
		Schema:  AuthorityTrustSchema,
		Artifact: Artifact{
			URI:    path,
			SHA256: hex.EncodeToString(digest[:]),
		},
	}
	rootVerifier := Ed25519AuthoritySignatureVerifier{Keys: map[string]ed25519.PublicKey{"root-2026": rootPublicKey}}
	store, err := LoadAuthorityTrustStoreWithSignatureVerifier(reference, rootVerifier)
	if err != nil {
		t.Fatal(err)
	}
	authorityVerifier := store.SignatureVerifier()
	authorityPayload := []byte("authority-payload")
	if err := authorityVerifier.VerifySignature("authority-active", authorityPayload, ed25519.Sign(activePrivateKey, authorityPayload)); err != nil {
		t.Fatalf("active key from rooted trust store failed: %v", err)
	}

	document.Keys[0].Status = AuthorityTrustKeyRevoked
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	tamperedDigest := sha256.Sum256(tampered)
	reference.Artifact.SHA256 = hex.EncodeToString(tamperedDigest[:])
	if _, err := LoadAuthorityTrustStoreWithSignatureVerifier(reference, rootVerifier); err == nil || !strings.Contains(err.Error(), "authority signature is invalid") {
		t.Fatalf("error = %v, want invalid trust-root signature", err)
	}
}

func TestLoadAuthorityRootStoreSupportsBootstrapRotationAndTrustChain(t *testing.T) {
	oldRootPublicKey, oldRootPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	newRootPublicKey, newRootPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	authorityPublicKey, authorityPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	rootDocument := authorityRootDocument{
		Schema:  AuthorityRootSchema,
		ID:      "governance-roots",
		Version: 2,
		Keys: []authorityRootKeyDocument{
			{ID: "root-new", Algorithm: AuthoritySignatureAlgorithmEd25519, PublicKey: base64.StdEncoding.EncodeToString(newRootPublicKey), Status: AuthorityTrustKeyActive},
			{ID: "root-old", Algorithm: AuthoritySignatureAlgorithmEd25519, PublicKey: base64.StdEncoding.EncodeToString(oldRootPublicKey), Status: AuthorityTrustKeyRevoked},
		},
	}
	rootPayload, err := json.Marshal(authorityRootSigningDocument{
		Schema:  rootDocument.Schema,
		ID:      rootDocument.ID,
		Version: rootDocument.Version,
		Keys:    rootDocument.Keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	rootDocument.Signature = &AuthoritySignature{
		Algorithm: AuthoritySignatureAlgorithmEd25519,
		KeyID:     "root-old",
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(oldRootPrivateKey, rootPayload)),
	}
	rootData, err := json.Marshal(rootDocument)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "authority-root.json")
	if err := os.WriteFile(rootPath, rootData, 0o644); err != nil {
		t.Fatal(err)
	}
	rootDigest := sha256.Sum256(rootData)
	rootReference := AuthorityRootReference{
		ID:      rootDocument.ID,
		Version: rootDocument.Version,
		Schema:  AuthorityRootSchema,
		Artifact: Artifact{
			URI:    rootPath,
			SHA256: hex.EncodeToString(rootDigest[:]),
		},
	}
	bootstrap := Ed25519AuthoritySignatureVerifier{Keys: map[string]ed25519.PublicKey{"root-old": oldRootPublicKey}}
	roots, err := LoadAuthorityRootStoreWithSignatureVerifier(rootReference, bootstrap)
	if err != nil {
		t.Fatal(err)
	}

	rotationVerifier := roots.SignatureVerifier()
	rotationPayload := []byte("next-root-rotation")
	if err := rotationVerifier.VerifySignature("root-new", rotationPayload, ed25519.Sign(newRootPrivateKey, rotationPayload)); err != nil {
		t.Fatalf("new root verification error = %v", err)
	}
	if err := rotationVerifier.VerifySignature("root-old", rotationPayload, ed25519.Sign(oldRootPrivateKey, rotationPayload)); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("old root verification error = %v, want revoked root rejection", err)
	}

	trustDocument := authorityTrustDocument{
		Schema:  AuthorityTrustSchema,
		ID:      "authority-keys",
		Version: 3,
		Keys: []authorityTrustKeyDocument{
			{ID: "authority-active", Algorithm: AuthoritySignatureAlgorithmEd25519, PublicKey: base64.StdEncoding.EncodeToString(authorityPublicKey), Status: AuthorityTrustKeyActive},
		},
	}
	trustPayload, err := json.Marshal(authorityTrustSigningDocument{
		Schema:  trustDocument.Schema,
		ID:      trustDocument.ID,
		Version: trustDocument.Version,
		Keys:    trustDocument.Keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	trustDocument.Signature = &AuthoritySignature{
		Algorithm: AuthoritySignatureAlgorithmEd25519,
		KeyID:     "root-new",
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(newRootPrivateKey, trustPayload)),
	}
	trustData, err := json.Marshal(trustDocument)
	if err != nil {
		t.Fatal(err)
	}
	trustPath := filepath.Join(directory, "authority-trust.json")
	if err := os.WriteFile(trustPath, trustData, 0o644); err != nil {
		t.Fatal(err)
	}
	trustDigest := sha256.Sum256(trustData)
	trustReference := AuthorityTrustReference{
		ID:      trustDocument.ID,
		Version: trustDocument.Version,
		Schema:  AuthorityTrustSchema,
		Artifact: Artifact{
			URI:    trustPath,
			SHA256: hex.EncodeToString(trustDigest[:]),
		},
	}
	trustStore, err := LoadAuthorityTrustStoreWithRootStore(trustReference, roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := trustStore.SignatureVerifier().VerifySignature("authority-active", rotationPayload, ed25519.Sign(authorityPrivateKey, rotationPayload)); err != nil {
		t.Fatalf("authority verification through rotated root chain = %v", err)
	}
}

func TestLoadContractArtifactVerifiesContractBytes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "contract.yaml")
	data := []byte("contract: bytes\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256(data)
	reference := contractReference(hex.EncodeToString(digestBytes[:]), 1)
	reference.Artifact.URI = path
	loaded, err := LoadContractArtifact(reference)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(data) {
		t.Fatalf("loaded contract = %q, want original bytes", loaded)
	}

	if err := os.WriteFile(path, []byte("contract: changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadContractArtifact(reference); err == nil || !strings.Contains(err.Error(), "do not match reference artifact.sha256") {
		t.Fatalf("error = %v, want contract digest error", err)
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

type testAuthorityVerifier struct {
	allowed bool
	err     error
}

func (verifier testAuthorityVerifier) Verify(string, string, string) (bool, error) {
	return verifier.allowed, verifier.err
}

type recordingAuthorityVerifier struct {
	allowed bool
	at      string
	calls   int
}

func (verifier *recordingAuthorityVerifier) Verify(_, _, at string) (bool, error) {
	verifier.calls++
	verifier.at = at
	return verifier.allowed, nil
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
