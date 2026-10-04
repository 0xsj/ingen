// Package governance exposes Hammond's read-only contract approval verifier.
// Approval and replay semantics remain implemented by Hammond's internal
// governance model.
package governance

import internal "ingen/hammond/internal/governance"

type Artifact = internal.Artifact
type ContractIdentity = internal.ContractIdentity
type ContractReference = internal.ContractReference
type PolicyReference = internal.PolicyReference
type Record = internal.Record
type VerifiedArtifact = internal.VerifiedArtifact
type ApprovedVerification = internal.ApprovedVerification

// VerifyApproved checks one approved record against the caller's expected
// contract identity and explicitly selected active review-policy file. Every
// referenced Hammond artifact is read under root and hash-checked.
var VerifyApproved = internal.VerifyApprovedRooted
