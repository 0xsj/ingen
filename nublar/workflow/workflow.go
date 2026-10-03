// Package workflow exposes Nublar's declarative result-workflow boundary to
// other InGen verticals. Workflow semantics remain implemented by Nublar.
package workflow

import internal "ingen/nublar/internal/workflow"

const Schema = internal.Schema

type Document = internal.Document
type Check = internal.Check

var (
	LoadFile               = internal.LoadFile
	LoadFileWithReference  = internal.LoadFileWithReference
	Validate               = internal.Validate
	FileReference          = internal.FileReference
	FileReferenceFromBytes = internal.FileReferenceFromBytes
)
