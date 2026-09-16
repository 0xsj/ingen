package custody

import (
	"fmt"
	"sort"
	"strings"

	"ingen/lockwood/internal/store"
)

type LineageResolutionStatus string

const (
	LineageComplete   LineageResolutionStatus = "complete"
	LineageIncomplete LineageResolutionStatus = "incomplete"
)

type LineageIssueKind string

const (
	UnresolvedParentIssue  LineageIssueKind = "unresolved-parent"
	CycleIssue             LineageIssueKind = "cycle"
	ArtifactIntegrityIssue LineageIssueKind = "artifact-integrity"
)

// LineageIssue describes one problem found while projecting the reachable
// custody lineage. It is diagnostic evidence only; it does not change the
// custody record's status.
type LineageIssue struct {
	Kind      LineageIssueKind `json:"kind"`
	Digest    string           `json:"digest"`
	CustodyID string           `json:"custody_id,omitempty"`
	Path      []string         `json:"path,omitempty"`
	Error     string           `json:"error"`
}

// LineageReport is a read-only projection of one record's reachable lineage.
// Complete means every reachable accepted parent and artifact verified. An
// incomplete report preserves diagnostic issues for operators; it is not a
// new custody status.
type LineageReport struct {
	CustodyID        string                  `json:"custody_id"`
	ArtifactDigest   string                  `json:"artifact_digest"`
	Status           LineageResolutionStatus `json:"status"`
	ReachableDigests []string                `json:"reachable_digests"`
	Issues           []LineageIssue          `json:"issues"`
}

// AnalyzeLineage returns a deterministic, non-mutating lineage projection.
// Unlike VerifyLineage, it reports unresolved parents, cycles, and damaged
// reachable artifacts instead of returning at the first issue.
func AnalyzeLineage(records RecordStore, artifacts store.Store, record Record) (LineageReport, error) {
	report := LineageReport{
		CustodyID:        record.CustodyID,
		ArtifactDigest:   record.Artifact.Digest,
		Status:           LineageComplete,
		ReachableDigests: make([]string, 0),
		Issues:           make([]LineageIssue, 0),
	}
	if records == nil {
		return report, fmt.Errorf("custody record store is required")
	}
	if artifacts == nil {
		return report, fmt.Errorf("artifact store is required")
	}
	if err := record.Validate(); err != nil {
		return report, fmt.Errorf("validate lineage root: %w", err)
	}
	allRecords, err := records.List()
	if err != nil {
		return report, err
	}
	acceptedByDigest := make(map[string][]Record)
	for _, candidate := range allRecords {
		if candidate.Status == Accepted {
			acceptedByDigest[candidate.Artifact.Digest] = append(acceptedByDigest[candidate.Artifact.Digest], candidate)
		}
	}

	if err := artifacts.VerifyReference(record.Artifact); err != nil {
		report.Issues = append(report.Issues, LineageIssue{
			Kind:      ArtifactIntegrityIssue,
			Digest:    record.Artifact.Digest,
			CustodyID: record.CustodyID,
			Error:     err.Error(),
		})
	}

	active := map[string]int{record.Artifact.Digest: 0}
	visited := make(map[string]bool)
	var reachable = map[string]bool{record.Artifact.Digest: true}
	var walk func(string, []string, string)
	walk = func(digest string, path []string, fromCustodyID string) {
		if index, ok := active[digest]; ok {
			cyclePath := append([]string{}, path[index:]...)
			cyclePath = append(cyclePath, digest)
			report.Issues = append(report.Issues, LineageIssue{
				Kind:      CycleIssue,
				Digest:    digest,
				CustodyID: fromCustodyID,
				Path:      cyclePath,
				Error:     fmt.Sprintf("lineage cycle detected: %s", strings.Join(cyclePath, " -> ")),
			})
			return
		}
		if visited[digest] {
			return
		}
		reachable[digest] = true
		candidates := acceptedByDigest[digest]
		if len(candidates) == 0 {
			report.Issues = append(report.Issues, LineageIssue{
				Kind:      UnresolvedParentIssue,
				Digest:    digest,
				CustodyID: fromCustodyID,
				Error:     fmt.Sprintf("unresolved lineage parent %s for %s", digest, fromCustodyID),
			})
			visited[digest] = true
			return
		}

		active[digest] = len(path)
		nextPath := append(append([]string{}, path...), digest)
		defer delete(active, digest)
		for _, candidate := range candidates {
			if err := artifacts.VerifyReference(candidate.Artifact); err != nil {
				report.Issues = append(report.Issues, LineageIssue{
					Kind:      ArtifactIntegrityIssue,
					Digest:    candidate.Artifact.Digest,
					CustodyID: candidate.CustodyID,
					Error:     err.Error(),
				})
			}
		}
		for _, candidate := range candidates {
			for _, parent := range candidate.Parents {
				walk(parent.Digest, nextPath, candidate.CustodyID)
			}
		}
		visited[digest] = true
	}

	for _, parent := range record.Parents {
		walk(parent.Digest, []string{record.Artifact.Digest}, record.CustodyID)
	}
	for digest := range reachable {
		report.ReachableDigests = append(report.ReachableDigests, digest)
	}
	sort.Strings(report.ReachableDigests)
	sort.Slice(report.Issues, func(i, j int) bool {
		left, right := report.Issues[i], report.Issues[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Digest != right.Digest {
			return left.Digest < right.Digest
		}
		if left.CustodyID != right.CustodyID {
			return left.CustodyID < right.CustodyID
		}
		return left.Error < right.Error
	})
	if len(report.Issues) > 0 {
		report.Status = LineageIncomplete
	}
	return report, nil
}
