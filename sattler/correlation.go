package sattler

import (
	"fmt"
	"io"
)

// Bundle correlation kinds identify supported cross-artifact identity
// observations.
const (
	BundleCorrelationKindNublarCustodySource = "nublar-run-to-custody-source"
	BundleCorrelationKindNublarAmber         = "nublar-correlation-to-amber"
	// BundleCorrelationKind is retained as the original custody correlation
	// kind for callers that used the first correlation slice.
	BundleCorrelationKind = BundleCorrelationKindNublarCustodySource
)

// BundleCorrelationRelation describes how two explicit identifiers relate.
type BundleCorrelationRelation string

const (
	// BundleCorrelationExactMatch means both identifiers are present and equal.
	BundleCorrelationExactMatch BundleCorrelationRelation = "exact-match"
	// BundleCorrelationMismatch means both identifiers are present but differ.
	BundleCorrelationMismatch BundleCorrelationRelation = "mismatch"
	// BundleCorrelationUnknown means one or both identifiers are unavailable.
	BundleCorrelationUnknown BundleCorrelationRelation = "unknown"
)

// BundleCorrelation is an identity observation between artifacts on one side
// of a bundle. It does not assert that either artifact caused the other.
type BundleCorrelation struct {
	Kind                string                    `json:"kind"`
	Side                string                    `json:"side"`
	Relation            BundleCorrelationRelation `json:"relation"`
	NublarRunID         string                    `json:"nublar_run_id,omitempty"`
	CustodySourceRunID  string                    `json:"custody_source_run_id,omitempty"`
	NublarCorrelationID string                    `json:"nublar_correlation_id,omitempty"`
	AmberCorrelationID  string                    `json:"amber_correlation_id,omitempty"`
}

// CorrelateBundle observes supported exact identifier relationships in a
// bundle. Results are ordered by subsystem relationship and then side.
func CorrelateBundle(report BundleComparison) []BundleCorrelation {
	if report.NublarRun == nil || (report.Custody == nil && report.Provenance == nil) {
		return nil
	}
	correlations := make([]BundleCorrelation, 0, 4)
	if report.Custody != nil {
		correlations = append(correlations,
			correlateBundleSide("before", report.NublarRun.Before.RunID, report.Custody.Before.SourceRunID),
			correlateBundleSide("after", report.NublarRun.After.RunID, report.Custody.After.SourceRunID),
		)
	}
	if report.Provenance != nil {
		correlations = append(correlations,
			correlateAmberSide("before", report.NublarRun.Before.Correlation, report.Provenance.Before.CorrelationID),
			correlateAmberSide("after", report.NublarRun.After.Correlation, report.Provenance.After.CorrelationID),
		)
	}
	return correlations
}

func correlateBundleSide(side, nublarRunID, custodySourceRunID string) BundleCorrelation {
	relation := BundleCorrelationUnknown
	if nublarRunID != "" && custodySourceRunID != "" {
		relation = BundleCorrelationMismatch
		if nublarRunID == custodySourceRunID {
			relation = BundleCorrelationExactMatch
		}
	}
	return BundleCorrelation{
		Kind:               BundleCorrelationKindNublarCustodySource,
		Side:               side,
		Relation:           relation,
		NublarRunID:        nublarRunID,
		CustodySourceRunID: custodySourceRunID,
	}
}

func correlateAmberSide(side string, nublar *NublarCorrelationSummary, amberCorrelationID string) BundleCorrelation {
	nublarCorrelationID := ""
	if nublar != nil {
		nublarCorrelationID = nublar.ID
	}
	relation := BundleCorrelationUnknown
	if nublarCorrelationID != "" && amberCorrelationID != "" {
		relation = BundleCorrelationMismatch
		if nublarCorrelationID == amberCorrelationID {
			relation = BundleCorrelationExactMatch
		}
	}
	return BundleCorrelation{
		Kind:                BundleCorrelationKindNublarAmber,
		Side:                side,
		Relation:            relation,
		NublarCorrelationID: nublarCorrelationID,
		AmberCorrelationID:  amberCorrelationID,
	}
}

func writeBundleCorrelationText(w io.Writer, correlation BundleCorrelation) error {
	if correlation.Kind == BundleCorrelationKindNublarAmber {
		_, err := fmt.Fprintf(w, "    - %s %s: %s (Nublar correlation %q, Amber correlation %q)\n", correlation.Kind, correlation.Side, correlation.Relation, correlation.NublarCorrelationID, correlation.AmberCorrelationID)
		return err
	}
	_, err := fmt.Fprintf(w, "    - %s %s: %s (Nublar run %q, custody source %q)\n", correlation.Kind, correlation.Side, correlation.Relation, correlation.NublarRunID, correlation.CustodySourceRunID)
	return err
}
