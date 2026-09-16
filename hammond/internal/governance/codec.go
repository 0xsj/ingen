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
	if _, err := LoadContractArtifact(record.Contract); err != nil {
		return Record{}, fmt.Errorf("load Hammond record contract: %w", err)
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
	Schema           string             `json:"schema"`
	ID               string             `json:"id"`
	Version          int                `json:"version"`
	MinimumApprovals int                `json:"minimum_approvals"`
	RequiredRoles    []string           `json:"required_roles,omitempty"`
	Authority        AuthorityReference `json:"authority,omitempty"`
}

type reviewAuthorityDocument struct {
	Schema    string              `json:"schema"`
	ID        string              `json:"id"`
	Version   int                 `json:"version"`
	Actors    map[string][]string `json:"actors"`
	Signature *AuthoritySignature `json:"signature,omitempty"`
}

type reviewAuthoritySigningDocument struct {
	Schema  string              `json:"schema"`
	ID      string              `json:"id"`
	Version int                 `json:"version"`
	Actors  map[string][]string `json:"actors"`
}

// DecodeReviewPolicy strictly decodes a policy artifact and binds its exact
// bytes to the supplied policy reference.
func DecodeReviewPolicy(data []byte, reference PolicyReference) (ReviewPolicy, error) {
	return decodeReviewPolicy(data, reference, nil)
}

// DecodeReviewPolicyWithAuthoritySignatureVerifier strictly decodes a policy
// and requires any referenced authority artifact signature to validate.
func DecodeReviewPolicyWithAuthoritySignatureVerifier(data []byte, reference PolicyReference, verifier AuthoritySignatureVerifier) (ReviewPolicy, error) {
	if verifier == nil {
		return ReviewPolicy{}, fmt.Errorf("authority signature verifier is required")
	}
	return decodeReviewPolicy(data, reference, verifier)
}

func decodeReviewPolicy(data []byte, reference PolicyReference, verifier AuthoritySignatureVerifier) (ReviewPolicy, error) {
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
		Authority:        document.Authority,
	}
	if !isEmptyAuthorityReference(document.Authority) {
		var authority ReviewAuthority
		var err error
		if verifier == nil {
			authority, err = LoadReviewAuthority(document.Authority)
		} else {
			authority, err = LoadReviewAuthorityWithSignatureVerifier(document.Authority, verifier)
		}
		if err != nil {
			return ReviewPolicy{}, fmt.Errorf("load review policy authority: %w", err)
		}
		policy.ActorRoles = authority.Actors
		policy.AuthorityVerifier = authority
	}
	if err := policy.Validate(); err != nil {
		return ReviewPolicy{}, err
	}
	return policy, nil
}

// LoadReviewPolicy loads the local file named by a policy reference and then
// verifies its identity and digest.
func LoadReviewPolicy(reference PolicyReference) (ReviewPolicy, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return ReviewPolicy{}, fmt.Errorf("read Hammond review policy %s: %w", reference.Artifact.URI, err)
	}
	return DecodeReviewPolicy(data, reference)
}

// LoadReviewPolicyWithAuthoritySignatureVerifier loads a policy and requires
// any referenced authority artifact signature to validate.
func LoadReviewPolicyWithAuthoritySignatureVerifier(reference PolicyReference, verifier AuthoritySignatureVerifier) (ReviewPolicy, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return ReviewPolicy{}, fmt.Errorf("read Hammond review policy %s: %w", reference.Artifact.URI, err)
	}
	return DecodeReviewPolicyWithAuthoritySignatureVerifier(data, reference, verifier)
}

// DecodeReviewAuthority strictly decodes an authority snapshot and binds its
// exact bytes to the supplied reference.
func DecodeReviewAuthority(data []byte, reference AuthorityReference) (ReviewAuthority, error) {
	return decodeReviewAuthority(data, reference, nil)
}

// DecodeReviewAuthorityWithSignatureVerifier strictly decodes and verifies a
// signed authority artifact using the caller's trusted key set.
func DecodeReviewAuthorityWithSignatureVerifier(data []byte, reference AuthorityReference, verifier AuthoritySignatureVerifier) (ReviewAuthority, error) {
	if verifier == nil {
		return ReviewAuthority{}, fmt.Errorf("authority signature verifier is required")
	}
	return decodeReviewAuthority(data, reference, verifier)
}

func decodeReviewAuthority(data []byte, reference AuthorityReference, verifier AuthoritySignatureVerifier) (ReviewAuthority, error) {
	var document reviewAuthorityDocument
	if err := decodeStrict(data, &document); err != nil {
		return ReviewAuthority{}, fmt.Errorf("decode Hammond authority: %w", err)
	}
	if document.Schema != AuthoritySchema {
		return ReviewAuthority{}, fmt.Errorf("authority schema must be %s", AuthoritySchema)
	}
	if document.ID != reference.ID || document.Version != reference.Version {
		return ReviewAuthority{}, fmt.Errorf("authority identity does not match its reference")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != reference.Artifact.SHA256 {
		return ReviewAuthority{}, fmt.Errorf("authority bytes do not match reference artifact.sha256")
	}
	if verifier != nil {
		if document.Signature == nil {
			return ReviewAuthority{}, fmt.Errorf("authority signature is required")
		}
		if err := validateAuthoritySignature(document.Signature); err != nil {
			return ReviewAuthority{}, err
		}
		signature, err := decodeAuthoritySignature(document.Signature.Signature)
		if err != nil {
			return ReviewAuthority{}, err
		}
		payload, err := json.Marshal(reviewAuthoritySigningDocument{
			Schema:  document.Schema,
			ID:      document.ID,
			Version: document.Version,
			Actors:  document.Actors,
		})
		if err != nil {
			return ReviewAuthority{}, fmt.Errorf("canonicalize authority for signature: %w", err)
		}
		if err := verifier.VerifySignature(document.Signature.KeyID, payload, signature); err != nil {
			return ReviewAuthority{}, fmt.Errorf("verify authority signature: %w", err)
		}
	}
	authority := ReviewAuthority{Reference: reference, Actors: document.Actors, Signature: document.Signature}
	if err := authority.Validate(); err != nil {
		return ReviewAuthority{}, err
	}
	return authority, nil
}

// LoadReviewAuthority loads and verifies a local authority snapshot.
func LoadReviewAuthority(reference AuthorityReference) (ReviewAuthority, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return ReviewAuthority{}, fmt.Errorf("read Hammond authority %s: %w", reference.Artifact.URI, err)
	}
	return DecodeReviewAuthority(data, reference)
}

// LoadReviewAuthorityWithSignatureVerifier loads an authority artifact and
// requires its signature to validate against the supplied trust set.
func LoadReviewAuthorityWithSignatureVerifier(reference AuthorityReference, verifier AuthoritySignatureVerifier) (ReviewAuthority, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return ReviewAuthority{}, fmt.Errorf("read Hammond authority %s: %w", reference.Artifact.URI, err)
	}
	return DecodeReviewAuthorityWithSignatureVerifier(data, reference, verifier)
}

func isEmptyAuthorityReference(reference AuthorityReference) bool {
	return reference.ID == "" && reference.Version == 0 && reference.Schema == "" && reference.Artifact.URI == "" && reference.Artifact.SHA256 == ""
}

// LoadContractArtifact loads the local contract bytes named by a reference and
// verifies their SHA-256 without interpreting the contract contents.
func LoadContractArtifact(reference ContractReference) ([]byte, error) {
	if problems := validateContractReference(reference, "contract"); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return nil, fmt.Errorf("read Hammond contract artifact %s: %w", reference.Artifact.URI, err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != reference.Artifact.SHA256 {
		return nil, fmt.Errorf("contract artifact bytes do not match reference artifact.sha256")
	}
	return data, nil
}

func readLocalArtifact(uri string) ([]byte, error) {
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
