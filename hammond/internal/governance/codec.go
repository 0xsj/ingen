package governance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DecodeRecord parses one strict JSON governance record and applies domain
// validation before returning it to a caller.
func DecodeRecord(data []byte) (Record, error) {
	var record Record
	if err := decodeStrict(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode Hammond record: %w", err)
	}
	policy, err := LoadReviewPolicy(record.Policy)
	if err != nil {
		return Record{}, fmt.Errorf("load Hammond record policy: %w", err)
	}
	if err := record.ValidateWithPolicy(policy); err != nil {
		return Record{}, err
	}
	return record, nil
}

// DecodeEvent parses one strict JSON event. Contextual checks are applied when
// the event is appended to a specific record.
func DecodeEvent(data []byte) (Event, error) {
	var event Event
	if err := decodeStrict(data, &event); err != nil {
		return Event{}, fmt.Errorf("decode Hammond event: %w", err)
	}
	return event, nil
}

type reviewPolicyDocument struct {
	Schema           string              `json:"schema"`
	ID               string              `json:"id"`
	Version          int                 `json:"version"`
	MinimumApprovals int                 `json:"minimum_approvals"`
	RequiredRoles    []string            `json:"required_roles,omitempty"`
	ActorRoles       map[string][]string `json:"actor_roles,omitempty"`
}

// DecodeReviewPolicy strictly decodes a policy artifact and binds its exact
// bytes to the supplied policy reference.
func DecodeReviewPolicy(data []byte, reference PolicyReference) (ReviewPolicy, error) {
	var document reviewPolicyDocument
	if err := decodeStrict(data, &document); err != nil {
		return ReviewPolicy{}, fmt.Errorf("decode Hammond review policy: %w", err)
	}
	if document.Schema != PolicySchema {
		return ReviewPolicy{}, fmt.Errorf("review policy schema must be %s", PolicySchema)
	}
	if document.ID != reference.ID || document.Version != reference.Version {
		return ReviewPolicy{}, fmt.Errorf("review policy identity does not match its reference")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != reference.Artifact.SHA256 {
		return ReviewPolicy{}, fmt.Errorf("review policy bytes do not match reference artifact.sha256")
	}
	policy := ReviewPolicy{
		Reference:        reference,
		MinimumApprovals: document.MinimumApprovals,
		RequiredRoles:    document.RequiredRoles,
		ActorRoles:       document.ActorRoles,
	}
	if err := policy.Validate(); err != nil {
		return ReviewPolicy{}, err
	}
	return policy, nil
}

// LoadReviewPolicy loads the local file named by a policy reference and then
// verifies its identity and digest.
func LoadReviewPolicy(reference PolicyReference) (ReviewPolicy, error) {
	data, err := readLocalPolicyArtifact(reference.Artifact.URI)
	if err != nil {
		return ReviewPolicy{}, fmt.Errorf("read Hammond review policy %s: %w", reference.Artifact.URI, err)
	}
	return DecodeReviewPolicy(data, reference)
}

func readLocalPolicyArtifact(uri string) ([]byte, error) {
	data, err := os.ReadFile(uri)
	if err == nil || filepath.IsAbs(uri) {
		return data, err
	}

	workingDirectory, workingDirectoryErr := os.Getwd()
	if workingDirectoryErr != nil {
		return nil, err
	}
	for directory := workingDirectory; ; directory = filepath.Dir(directory) {
		candidate := filepath.Join(directory, uri)
		data, candidateErr := os.ReadFile(candidate)
		if candidateErr == nil {
			return data, nil
		}
		if !os.IsNotExist(candidateErr) {
			return nil, candidateErr
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
	}
	return nil, err
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return fmt.Errorf("trailing JSON is not allowed: %w", err)
	}
	return nil
}
