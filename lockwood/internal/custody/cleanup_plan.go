package custody

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"ingen/lockwood/internal/artifact"
)

const CleanupPlanSchema = "lockwood.cleanup-plan/v1"

type CleanupPlanState string

const (
	ReportedOrphanState   CleanupPlanState = "reported-orphan"
	CleanupCandidateState CleanupPlanState = "cleanup-candidate"
)

const CleanupNotAuthorized = "not-authorized"

type CleanupPlanEntry struct {
	Digest       string           `json:"digest"`
	SizeBytes    int64            `json:"size_bytes"`
	ModifiedAt   time.Time        `json:"modified_at"`
	State        CleanupPlanState `json:"state"`
	ActionStatus string           `json:"action_status"`
	Blockers     []string         `json:"blockers"`
}

// CleanupPlan is a read-only projection of reconciliation output. It never
// reports an artifact as eligible for deletion: every entry remains blocked by
// the fresh checks and external authority required by the cleanup contract.
type CleanupPlan struct {
	Schema             string              `json:"schema"`
	AsOf               time.Time           `json:"as_of"`
	OrphanGrace        string              `json:"orphan_grace"`
	Entries            []CleanupPlanEntry  `json:"entries"`
	DanglingReferences []DanglingReference `json:"dangling_references"`
	CorruptBlobs       []CorruptBlob       `json:"corrupt_blobs"`
}

type CleanupPlanOptions struct {
	OrphanGrace time.Duration
	AsOf        time.Time
}

// Validate checks the read-only cleanup-plan contract. It does not infer
// eligibility or authorize any action.
func (plan CleanupPlan) Validate() error {
	if plan.Schema != CleanupPlanSchema {
		return fmt.Errorf("unexpected cleanup plan schema %q", plan.Schema)
	}
	if plan.AsOf.IsZero() {
		return fmt.Errorf("cleanup plan as_of is required")
	}
	if plan.OrphanGrace == "" {
		return fmt.Errorf("cleanup plan orphan_grace is required")
	}
	seenEntries := make(map[string]struct{}, len(plan.Entries))
	for _, entry := range plan.Entries {
		if err := artifact.ValidateDigest(entry.Digest); err != nil {
			return fmt.Errorf("invalid cleanup plan entry digest: %w", err)
		}
		if _, ok := seenEntries[entry.Digest]; ok {
			return fmt.Errorf("cleanup plan repeats entry digest %q", entry.Digest)
		}
		seenEntries[entry.Digest] = struct{}{}
		if entry.SizeBytes < 0 {
			return fmt.Errorf("cleanup plan entry %q has negative size", entry.Digest)
		}
		if entry.ModifiedAt.IsZero() {
			return fmt.Errorf("cleanup plan entry %q modified_at is required", entry.Digest)
		}
		switch entry.State {
		case ReportedOrphanState, CleanupCandidateState:
		default:
			return fmt.Errorf("cleanup plan entry %q has invalid state %q", entry.Digest, entry.State)
		}
		if entry.ActionStatus != CleanupNotAuthorized {
			return fmt.Errorf("cleanup plan entry %q has invalid action status %q", entry.Digest, entry.ActionStatus)
		}
		if len(entry.Blockers) == 0 {
			return fmt.Errorf("cleanup plan entry %q must have blockers", entry.Digest)
		}
		for _, blocker := range entry.Blockers {
			if blocker == "" {
				return fmt.Errorf("cleanup plan entry %q has an empty blocker", entry.Digest)
			}
		}
	}
	for _, reference := range plan.DanglingReferences {
		if reference.CustodyID == "" || reference.Error == "" {
			return fmt.Errorf("cleanup plan dangling reference requires custody_id and error")
		}
		if err := artifact.ValidateDigest(reference.Digest); err != nil {
			return fmt.Errorf("invalid cleanup plan dangling reference digest: %w", err)
		}
	}
	for _, corrupt := range plan.CorruptBlobs {
		if corrupt.Error == "" {
			return fmt.Errorf("cleanup plan corrupt blob requires error")
		}
		if err := artifact.ValidateDigest(corrupt.Digest); err != nil {
			return fmt.Errorf("invalid cleanup plan corrupt blob digest: %w", err)
		}
	}
	return nil
}

// MarshalCanonicalCleanupPlan returns stable compact JSON for the complete
// read-only plan. Collection ordering is normalized before hashing.
func MarshalCanonicalCleanupPlan(plan CleanupPlan) ([]byte, error) {
	normalized := normalizeCleanupPlan(plan)
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

// CleanupPlanDigest identifies the canonical plan bytes referenced by a
// cleanup authorization target. It grants no authority.
func CleanupPlanDigest(plan CleanupPlan) (string, error) {
	encoded, err := MarshalCanonicalCleanupPlan(plan)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func normalizeCleanupPlan(plan CleanupPlan) CleanupPlan {
	normalized := plan
	normalized.AsOf = plan.AsOf.UTC()
	normalized.Entries = append([]CleanupPlanEntry(nil), plan.Entries...)
	normalized.DanglingReferences = append([]DanglingReference(nil), plan.DanglingReferences...)
	normalized.CorruptBlobs = append([]CorruptBlob(nil), plan.CorruptBlobs...)
	sort.Slice(normalized.Entries, func(i, j int) bool {
		return normalized.Entries[i].Digest < normalized.Entries[j].Digest
	})
	sort.Slice(normalized.DanglingReferences, func(i, j int) bool {
		if normalized.DanglingReferences[i].CustodyID != normalized.DanglingReferences[j].CustodyID {
			return normalized.DanglingReferences[i].CustodyID < normalized.DanglingReferences[j].CustodyID
		}
		return normalized.DanglingReferences[i].Digest < normalized.DanglingReferences[j].Digest
	})
	sort.Slice(normalized.CorruptBlobs, func(i, j int) bool {
		return normalized.CorruptBlobs[i].Digest < normalized.CorruptBlobs[j].Digest
	})
	if normalized.Entries == nil {
		normalized.Entries = make([]CleanupPlanEntry, 0)
	}
	if normalized.DanglingReferences == nil {
		normalized.DanglingReferences = make([]DanglingReference, 0)
	}
	if normalized.CorruptBlobs == nil {
		normalized.CorruptBlobs = make([]CorruptBlob, 0)
	}
	return normalized
}

// BuildCleanupPlan projects an already completed reconciliation into explicit
// cleanup states. It does not re-read, mutate, delete, or authorize anything.
func BuildCleanupPlan(report Reconciliation, options CleanupPlanOptions) (CleanupPlan, error) {
	if options.OrphanGrace < 0 {
		return CleanupPlan{}, fmt.Errorf("orphan grace period cannot be negative")
	}
	if options.AsOf.IsZero() {
		options.AsOf = time.Now().UTC()
	}
	plan := CleanupPlan{
		Schema:             CleanupPlanSchema,
		AsOf:               options.AsOf.UTC(),
		OrphanGrace:        options.OrphanGrace.String(),
		Entries:            make([]CleanupPlanEntry, 0, len(report.Orphans)),
		DanglingReferences: make([]DanglingReference, 0, len(report.DanglingReferences)),
		CorruptBlobs:       make([]CorruptBlob, 0, len(report.CorruptBlobs)),
	}
	plan.DanglingReferences = append(plan.DanglingReferences, report.DanglingReferences...)
	plan.CorruptBlobs = append(plan.CorruptBlobs, report.CorruptBlobs...)
	candidates := make(map[string]bool, len(report.CleanupCandidates))
	for _, candidate := range report.CleanupCandidates {
		candidates[candidate.Digest] = true
	}
	for _, orphan := range report.Orphans {
		state := ReportedOrphanState
		blockers := []string{
			"cleanup grace period has not been satisfied",
			"fresh reference, recovery, protection, and legal-hold checks are required",
			"retention policy evaluation and explicit delete authorization are required",
		}
		if candidates[orphan.Digest] {
			state = CleanupCandidateState
			blockers = []string{
				"fresh exclusive reference, recovery, protection, and legal-hold checks are required",
				"retention policy evaluation and policy snapshot are required",
				"explicit delete authorization is required",
			}
		} else if options.OrphanGrace <= 0 {
			blockers[0] = "no cleanup grace period was configured"
		}
		plan.Entries = append(plan.Entries, CleanupPlanEntry{
			Digest:       orphan.Digest,
			SizeBytes:    orphan.SizeBytes,
			ModifiedAt:   orphan.ModifiedAt,
			State:        state,
			ActionStatus: CleanupNotAuthorized,
			Blockers:     blockers,
		})
	}
	sort.Slice(plan.Entries, func(i, j int) bool {
		return plan.Entries[i].Digest < plan.Entries[j].Digest
	})
	sort.Slice(plan.DanglingReferences, func(i, j int) bool {
		if plan.DanglingReferences[i].CustodyID != plan.DanglingReferences[j].CustodyID {
			return plan.DanglingReferences[i].CustodyID < plan.DanglingReferences[j].CustodyID
		}
		return plan.DanglingReferences[i].Digest < plan.DanglingReferences[j].Digest
	})
	sort.Slice(plan.CorruptBlobs, func(i, j int) bool {
		return plan.CorruptBlobs[i].Digest < plan.CorruptBlobs[j].Digest
	})
	return plan, nil
}
