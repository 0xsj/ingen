// Package contract exposes Sorna's contract validation and sealing boundary
// to other InGen verticals. Contract semantics remain implemented by Sorna.
package contract

import internal "ingen/sorna/internal/contract"

const Schema = internal.Schema

type Document = internal.Document
type Sealed = internal.Sealed
type ValidationError = internal.ValidationError

var (
	LoadFile      = internal.LoadFile
	LoadBytes     = internal.LoadBytes
	Validate      = internal.Validate
	CanonicalJSON = internal.CanonicalJSON
	Seal          = internal.Seal
	SealFile      = internal.SealFile
)

// SealAt seals a contract while resolving relative fixture paths from baseDir.
// The implementation and fixture hashing behavior remain in Sorna's internal
// contract package; callers are responsible for applying their own path-root
// constraints before selecting a source document and base directory.
func SealAt(document Document, baseDir string) (Sealed, error) {
	return internal.SealAt(document, baseDir)
}

// SealWithFixtureReader seals using fixture bytes supplied by readFixture. The
// callback receives the path declared in each fixture and can enforce a
// caller-owned root boundary before reading it.
func SealWithFixtureReader(document Document, readFixture func(path string) ([]byte, error)) (Sealed, error) {
	return internal.SealWithFixtureReader(document, readFixture)
}
