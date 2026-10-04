// Package oracle exposes read-only access to Sorna's frozen oracle artifact
// model. Validation and canonicalization semantics remain in the internal
// oracle implementation.
package oracle

import (
	"ingen/sorna/contract"
	internal "ingen/sorna/internal/oracle"
)

const Schema = internal.Schema

type Artifact = internal.Artifact
type ContractReference = internal.ContractReference
type Case = internal.Case

// Generate materializes a frozen oracle from a sealed contract and policy
// digest using the existing Sorna oracle implementation.
func Generate(sealed contract.Sealed, policySHA256 string) (Artifact, error) {
	return internal.Generate(sealed, policySHA256)
}

// LoadFile loads and validates a frozen oracle artifact from disk.
func LoadFile(path string) (Artifact, error) { return internal.LoadFile(path) }

// LoadBytes parses and validates an already-read canonical frozen oracle.
func LoadBytes(path string, contents []byte) (Artifact, error) {
	return internal.LoadBytes(path, contents)
}

// Validate returns structural errors in the frozen oracle shape.
func Validate(artifact Artifact) []string { return internal.Validate(artifact) }

// CanonicalJSON returns deterministic JSON bytes for a valid oracle artifact.
func CanonicalJSON(artifact Artifact) ([]byte, error) { return internal.CanonicalJSON(artifact) }

// Hash returns the canonical artifact hash.
func Hash(artifact Artifact) (string, error) { return internal.Hash(artifact) }

// HashFile hashes the exact bytes stored at path.
func HashFile(path string) (string, error) { return internal.HashFile(path) }
