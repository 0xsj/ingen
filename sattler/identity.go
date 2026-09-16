package sattler

import "ingen/core/ciresult"

// ArtifactIdentityRelation describes what can be concluded from two file
// references. A digest identifies bytes; it does not establish correctness.
type ArtifactIdentityRelation string

const (
	ArtifactIdentitySameBytes ArtifactIdentityRelation = "same-bytes"
	ArtifactIdentityReplaced  ArtifactIdentityRelation = "replaced"
	ArtifactIdentityAdded     ArtifactIdentityRelation = "added"
	ArtifactIdentityRemoved   ArtifactIdentityRelation = "removed"
	ArtifactIdentityUnknown   ArtifactIdentityRelation = "unknown"
)

// CompareArtifactIdentity relates two shared file references without reading
// the referenced paths. It is intentionally conservative when a digest is
// missing.
func CompareArtifactIdentity(before, after *ciresult.FileRef) ArtifactIdentityRelation {
	if before == nil && after == nil {
		return ""
	}
	if before == nil {
		return ArtifactIdentityAdded
	}
	if after == nil {
		return ArtifactIdentityRemoved
	}
	if before.SHA256 == "" || after.SHA256 == "" {
		return ArtifactIdentityUnknown
	}
	return CompareArtifactDigests(before.SHA256, after.SHA256)
}

// CompareArtifactDigests relates two digest strings, including Lockwood's
// `sha256:` representation. Empty values are treated as missing identity.
func CompareArtifactDigests(before, after string) ArtifactIdentityRelation {
	if before == "" && after == "" {
		return ""
	}
	if before == "" {
		return ArtifactIdentityAdded
	}
	if after == "" {
		return ArtifactIdentityRemoved
	}
	if before == after {
		return ArtifactIdentitySameBytes
	}
	return ArtifactIdentityReplaced
}
