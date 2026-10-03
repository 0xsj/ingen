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
	Validate      = internal.Validate
	CanonicalJSON = internal.CanonicalJSON
	Seal          = internal.Seal
	SealFile      = internal.SealFile
)
