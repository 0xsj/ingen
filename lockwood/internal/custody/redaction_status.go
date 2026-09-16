package custody

import (
	"fmt"
	"sort"
	"time"

	"ingen/lockwood/internal/store"
)

type RedactionResolutionStatus string

const (
	RedactionComplete   RedactionResolutionStatus = "complete"
	RedactionUnanchored RedactionResolutionStatus = "result-unanchored"
	RedactionIncomplete RedactionResolutionStatus = "incomplete"
)

type RedactionIssue struct {
	Kind      string `json:"kind"`
	Digest    string `json:"digest,omitempty"`
	CustodyID string `json:"custody_id,omitempty"`
	Error     string `json:"error"`
}

// RedactionStatus is a read-only projection connecting one redaction event to
// its source and resulting artifacts and to any explicit result custody
// records. It does not assert that the payload transformation was performed.
type RedactionStatus struct {
	SourceCustodyID           string                    `json:"source_custody_id"`
	SourceArtifactDigest      string                    `json:"source_artifact_digest"`
	EventID                   string                    `json:"event_id"`
	EventRecordedAt           time.Time                 `json:"event_recorded_at"`
	OriginalDigest            string                    `json:"original_digest"`
	ResultingDigest           string                    `json:"resulting_digest"`
	OriginalArtifactVerified  bool                      `json:"original_artifact_verified"`
	ResultingArtifactVerified bool                      `json:"resulting_artifact_verified"`
	OriginalCustodyIDs        []string                  `json:"original_custody_ids"`
	ResultCustodyIDs          []string                  `json:"result_custody_ids"`
	PromotedCustodyIDs        []string                  `json:"promoted_custody_ids"`
	Status                    RedactionResolutionStatus `json:"status"`
	Issues                    []RedactionIssue          `json:"issues"`
}

// AnalyzeRedaction returns a deterministic, non-mutating trace for one
// redaction event. A result can be verified but remain result-unanchored until
// PromoteRedactionResult creates an accepted custody record for it.
func AnalyzeRedaction(records RecordStore, artifacts store.Store, events HandlingEventStore, sourceCustodyID, eventID string) (RedactionStatus, error) {
	status := RedactionStatus{
		SourceCustodyID:    sourceCustodyID,
		EventID:            eventID,
		OriginalCustodyIDs: make([]string, 0),
		ResultCustodyIDs:   make([]string, 0),
		PromotedCustodyIDs: make([]string, 0),
		Issues:             make([]RedactionIssue, 0),
		Status:             RedactionIncomplete,
	}
	if records == nil {
		return status, fmt.Errorf("custody record store is required")
	}
	if artifacts == nil {
		return status, fmt.Errorf("artifact store is required")
	}
	if events == nil {
		return status, fmt.Errorf("handling event store is required")
	}
	sourceRecord, err := records.Get(sourceCustodyID)
	if err != nil {
		return status, fmt.Errorf("read source custody record: %w", err)
	}
	if err := sourceRecord.Validate(); err != nil {
		return status, fmt.Errorf("validate source custody record: %w", err)
	}
	status.SourceArtifactDigest = sourceRecord.Artifact.Digest
	event, err := events.GetEvent(sourceCustodyID, eventID)
	if err != nil {
		return status, fmt.Errorf("read redaction event: %w", err)
	}
	if event.Type != RedactionEvent {
		return status, fmt.Errorf("event %q is not a redaction event", eventID)
	}
	if err := event.Validate(); err != nil {
		return status, fmt.Errorf("validate redaction event: %w", err)
	}
	status.EventRecordedAt = event.RecordedAt
	status.OriginalDigest = event.OriginalDigest
	status.ResultingDigest = event.ResultingDigest

	if sourceRecord.Status != Accepted {
		status.Issues = append(status.Issues, RedactionIssue{
			Kind:      "source-not-accepted",
			CustodyID: sourceRecord.CustodyID,
			Error:     "source custody record is not accepted",
		})
	}
	if err := artifacts.Verify(event.OriginalDigest); err != nil {
		status.Issues = append(status.Issues, RedactionIssue{
			Kind:   "original-artifact-integrity",
			Digest: event.OriginalDigest,
			Error:  err.Error(),
		})
	} else {
		status.OriginalArtifactVerified = true
	}
	if err := artifacts.Verify(event.ResultingDigest); err != nil {
		status.Issues = append(status.Issues, RedactionIssue{
			Kind:   "resulting-artifact-integrity",
			Digest: event.ResultingDigest,
			Error:  err.Error(),
		})
	} else {
		status.ResultingArtifactVerified = true
	}

	allRecords, err := records.List()
	if err != nil {
		return status, fmt.Errorf("list custody records: %w", err)
	}
	for _, candidate := range allRecords {
		if candidate.Status != Accepted {
			continue
		}
		if candidate.Artifact.Digest == event.OriginalDigest {
			status.OriginalCustodyIDs = append(status.OriginalCustodyIDs, candidate.CustodyID)
		}
		if candidate.Artifact.Digest != event.ResultingDigest {
			continue
		}
		status.ResultCustodyIDs = append(status.ResultCustodyIDs, candidate.CustodyID)
		if err := artifacts.VerifyReference(candidate.Artifact); err != nil {
			status.Issues = append(status.Issues, RedactionIssue{
				Kind:      "result-custody-integrity",
				Digest:    event.ResultingDigest,
				CustodyID: candidate.CustodyID,
				Error:     err.Error(),
			})
		}
		if !hasDerivedFromParent(candidate, event.OriginalDigest) {
			continue
		}
		status.PromotedCustodyIDs = append(status.PromotedCustodyIDs, candidate.CustodyID)
		lineage, err := AnalyzeLineage(records, artifacts, candidate)
		if err != nil {
			return status, fmt.Errorf("analyze promoted redaction lineage: %w", err)
		}
		if lineage.Status != LineageComplete {
			status.Issues = append(status.Issues, RedactionIssue{
				Kind:      "result-lineage-incomplete",
				Digest:    event.ResultingDigest,
				CustodyID: candidate.CustodyID,
				Error:     "promoted result custody lineage is incomplete",
			})
		}
	}
	sort.Strings(status.OriginalCustodyIDs)
	sort.Strings(status.ResultCustodyIDs)
	sort.Strings(status.PromotedCustodyIDs)
	if len(status.Issues) > 0 {
		status.Status = RedactionIncomplete
	} else if len(status.PromotedCustodyIDs) == 0 {
		status.Status = RedactionUnanchored
	} else {
		status.Status = RedactionComplete
	}
	return status, nil
}

func hasDerivedFromParent(record Record, digest string) bool {
	for _, parent := range record.Parents {
		if parent.Relation == DerivedFrom && parent.Digest == digest {
			return true
		}
	}
	return false
}
