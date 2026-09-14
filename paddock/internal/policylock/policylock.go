package policylock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"ingen/paddock/internal/policy"
)

const Schema = "paddock.policy-lock/v1"

type Artifact struct {
	Schema          string          `json:"schema"`
	PolicyPath      string          `json:"policy_path"`
	SourceSHA256    string          `json:"source_sha256"`
	CanonicalSHA256 string          `json:"canonical_sha256"`
	Policy          json.RawMessage `json:"policy"`
}

func Build(policyPath string) (Artifact, error) {
	source, err := os.ReadFile(policyPath)
	if err != nil {
		return Artifact{}, fmt.Errorf("read policy for sealing: %w", err)
	}
	config, err := policy.Load(policyPath)
	if err != nil {
		return Artifact{}, err
	}
	canonical, err := policy.CanonicalJSON(config)
	if err != nil {
		return Artifact{}, err
	}
	sourceDigest := sha256.Sum256(source)
	canonicalDigest := sha256.Sum256(canonical)
	return Artifact{
		Schema:          Schema,
		PolicyPath:      policyPath,
		SourceSHA256:    hex.EncodeToString(sourceDigest[:]),
		CanonicalSHA256: hex.EncodeToString(canonicalDigest[:]),
		Policy:          canonical,
	}, nil
}

func Verify(policyPath, lockPath string) error {
	locked, err := Load(lockPath)
	if err != nil {
		return err
	}
	current, err := Build(policyPath)
	if err != nil {
		return err
	}
	if locked.SourceSHA256 != current.SourceSHA256 {
		return fmt.Errorf("policy lock source hash does not match %s", policyPath)
	}
	if locked.CanonicalSHA256 != current.CanonicalSHA256 {
		return fmt.Errorf("policy lock canonical hash does not match %s", policyPath)
	}
	return nil
}

func Load(path string) (Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("read policy lock: %w", err)
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("parse policy lock: %w", err)
	}
	if err := artifact.Validate(); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func Save(path string, artifact Artifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode policy lock: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write policy lock: %w", err)
	}
	return nil
}

func (a Artifact) Validate() error {
	if a.Schema != Schema {
		return fmt.Errorf("policy lock schema must be %s, got %q", Schema, a.Schema)
	}
	if a.PolicyPath == "" {
		return fmt.Errorf("policy lock policy_path is required")
	}
	if !isSHA256(a.SourceSHA256) {
		return fmt.Errorf("policy lock source_sha256 must be a hexadecimal SHA-256 digest")
	}
	if !isSHA256(a.CanonicalSHA256) {
		return fmt.Errorf("policy lock canonical_sha256 must be a hexadecimal SHA-256 digest")
	}
	if len(a.Policy) == 0 || !json.Valid(a.Policy) {
		return fmt.Errorf("policy lock policy must be valid canonical JSON")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, a.Policy); err != nil {
		return fmt.Errorf("compact policy lock policy: %w", err)
	}
	digest := sha256.Sum256(compact.Bytes())
	if hex.EncodeToString(digest[:]) != a.CanonicalSHA256 {
		return fmt.Errorf("policy lock canonical_sha256 does not match policy")
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
