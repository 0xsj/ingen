package custody

import (
	"fmt"
	"strings"

	"ingen/lockwood/internal/store"
)

// VerifyLineage checks every reachable accepted parent and its blob. The
// starting record's own blob is checked by VerifyRecord; this function is
// responsible for extending that check through the record's parent graph.
func VerifyLineage(records RecordStore, artifacts store.Store, record Record) error {
	if records == nil {
		return fmt.Errorf("custody record store is required")
	}
	if artifacts == nil {
		return fmt.Errorf("artifact store is required")
	}
	allRecords, err := records.List()
	if err != nil {
		return err
	}
	acceptedByDigest := make(map[string][]Record)
	for _, candidate := range allRecords {
		if candidate.Status == Accepted {
			acceptedByDigest[candidate.Artifact.Digest] = append(acceptedByDigest[candidate.Artifact.Digest], candidate)
		}
	}

	// Keep the root on the active path so a descendant cannot point back to
	// the record being verified without being reported as a cycle. Multiple
	// accepted custody records for one digest are traversed as one graph node,
	// while every record's artifact reference is still checked below.
	active := map[string]int{record.Artifact.Digest: 0}
	visited := make(map[string]bool)

	var walk func(digest string, path []string) error
	walk = func(digest string, path []string) error {
		if index, ok := active[digest]; ok {
			cycle := append([]string{}, path[index:]...)
			cycle = append(cycle, digest)
			return fmt.Errorf("lineage cycle detected: %s", strings.Join(cycle, " -> "))
		}
		if visited[digest] {
			return nil
		}

		candidates := acceptedByDigest[digest]
		if len(candidates) == 0 {
			from := record.CustodyID
			if len(path) > 0 {
				from = path[len(path)-1]
			}
			return fmt.Errorf("unresolved lineage parent %s for %s", digest, from)
		}

		active[digest] = len(path)
		path = append(path, digest)
		defer delete(active, digest)

		for _, candidate := range candidates {
			if err := artifacts.VerifyReference(candidate.Artifact); err != nil {
				return fmt.Errorf("verify parent artifact %s via custody record %q: %w", digest, candidate.CustodyID, err)
			}
		}
		for _, candidate := range candidates {
			for _, parent := range candidate.Parents {
				if err := walk(parent.Digest, path); err != nil {
					return fmt.Errorf("verify lineage of %q: %w", candidate.CustodyID, err)
				}
			}
		}
		visited[digest] = true
		return nil
	}

	for _, parent := range record.Parents {
		if err := walk(parent.Digest, []string{record.Artifact.Digest}); err != nil {
			return err
		}
	}
	return nil
}
