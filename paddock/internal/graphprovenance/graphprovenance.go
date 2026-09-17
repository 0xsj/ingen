// Package graphprovenance creates a stable identity for the normalized graph
// consumed by Paddock authoring and policy analysis.
package graphprovenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	paddockgraph "ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
)

const schema = "paddock.init-graph/v1"

// SHA256 returns a digest of the stable graph plus the source configuration
// that selected its unit. It intentionally hashes normalized graph evidence,
// not raw source files, so callers can correlate a draft with the exact graph
// an adapter returned without exposing source contents.
func SHA256(language, unit string, input *model.Graph) (string, error) {
	if input == nil {
		return "", fmt.Errorf("graph provenance requires a graph")
	}
	payload := struct {
		Schema   string       `json:"schema"`
		Language string       `json:"language"`
		Unit     string       `json:"source_unit"`
		Graph    *model.Graph `json:"graph"`
	}{
		Schema:   schema,
		Language: language,
		Unit:     unit,
		Graph:    paddockgraph.StableCopy(input),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal graph provenance: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
