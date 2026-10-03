// Package policy exposes Sorna's policy validation and sealing boundary to
// other InGen verticals. Policy semantics remain implemented by Sorna.
package policy

import internal "ingen/sorna/internal/policy"

const Schema = internal.Schema

type Document = internal.Document
type Sealed = internal.Sealed
type Reference = internal.Reference
type ValidationError = internal.ValidationError

var (
	LoadFile      = internal.LoadFile
	Validate      = internal.Validate
	CanonicalJSON = internal.CanonicalJSON
	Seal          = internal.Seal
	SealFile      = internal.SealFile
)

func SubjectID(document Document) (string, error) {
	return internal.SubjectID(document)
}
