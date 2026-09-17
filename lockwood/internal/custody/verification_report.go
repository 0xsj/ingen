package custody

import (
	"fmt"

	"ingen/lockwood/internal/store"
)

const VerificationReportSchema = "lockwood.verification-report/v1"

type VerificationResultStatus string

const (
	VerificationVerified VerificationResultStatus = "verified"
	VerificationFailed   VerificationResultStatus = "failed"
)

// VerificationResult is a read-only outcome for one custody record. A failed
// result is diagnostic evidence and does not change the custody record status.
type VerificationResult struct {
	CustodyID      string                   `json:"custody_id"`
	ArtifactDigest string                   `json:"artifact_digest"`
	CustodyStatus  Status                   `json:"custody_status"`
	Status         VerificationResultStatus `json:"status"`
	Error          string                   `json:"error,omitempty"`
}

// VerificationReport is a deterministic, read-only batch verification result.
// Results retain per-record failures so an operator can inspect every matched
// record in one run; callers may use Failed to choose a non-zero process exit.
type VerificationReport struct {
	Schema   string               `json:"schema"`
	Checked  int                  `json:"checked"`
	Verified int                  `json:"verified"`
	Failed   int                  `json:"failed"`
	Results  []VerificationResult `json:"results"`
}

// VerifyAll lists the current custody records and verifies each one. Record
// verification failures are included in the report; storage/list failures are
// returned because no complete report can be produced in that case.
func VerifyAll(records RecordStore, artifacts store.Store) (VerificationReport, error) {
	if records == nil {
		return VerificationReport{}, fmt.Errorf("custody record store is required")
	}
	candidates, err := records.List()
	if err != nil {
		return VerificationReport{}, err
	}
	return VerifyRecords(records, artifacts, candidates)
}

// VerifyRecords verifies the supplied deterministic candidate set. Individual
// record failures are reported and do not stop verification of later records.
func VerifyRecords(records RecordStore, artifacts store.Store, candidates []Record) (VerificationReport, error) {
	if records == nil {
		return VerificationReport{}, fmt.Errorf("custody record store is required")
	}
	if artifacts == nil {
		return VerificationReport{}, fmt.Errorf("artifact store is required")
	}
	report := VerificationReport{
		Schema:  VerificationReportSchema,
		Results: make([]VerificationResult, 0, len(candidates)),
	}
	for _, candidate := range candidates {
		result := VerificationResult{
			CustodyID:      candidate.CustodyID,
			ArtifactDigest: candidate.Artifact.Digest,
			CustodyStatus:  candidate.Status,
			Status:         VerificationVerified,
		}
		if _, err := VerifyRecord(records, artifacts, candidate.CustodyID); err != nil {
			result.Status = VerificationFailed
			result.Error = err.Error()
			report.Failed++
		} else {
			report.Verified++
		}
		report.Results = append(report.Results, result)
		report.Checked++
	}
	return report, nil
}
