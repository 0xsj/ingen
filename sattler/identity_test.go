package sattler

import (
	"strings"
	"testing"

	"ingen/core/ciresult"
)

func TestCompareArtifactIdentityIsConservative(t *testing.T) {
	refA := ciresult.FileRef{Path: "a.json", SHA256: strings.Repeat("a", 64)}
	refASameBytes := ciresult.FileRef{Path: "moved/a.json", SHA256: strings.Repeat("a", 64)}
	refB := ciresult.FileRef{Path: "b.json", SHA256: strings.Repeat("b", 64)}
	refWithoutDigest := ciresult.FileRef{Path: "unknown.json"}
	tests := []struct {
		name   string
		before *ciresult.FileRef
		after  *ciresult.FileRef
		want   ArtifactIdentityRelation
	}{
		{name: "same bytes", before: &refA, after: &refASameBytes, want: ArtifactIdentitySameBytes},
		{name: "replaced", before: &refA, after: &refB, want: ArtifactIdentityReplaced},
		{name: "added", after: &refA, want: ArtifactIdentityAdded},
		{name: "removed", before: &refA, want: ArtifactIdentityRemoved},
		{name: "unknown", before: &refWithoutDigest, after: &refASameBytes, want: ArtifactIdentityUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CompareArtifactIdentity(test.before, test.after); got != test.want {
				t.Fatalf("identity = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCompareCarriesArtifactIdentityOnInputChanges(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Inputs["contract"] = ciresult.FileRef{Path: "moved/contract.yaml", SHA256: strings.Repeat("a", 64)}
	after.Inputs["provider"] = ciresult.FileRef{Path: "provider.yaml", SHA256: strings.Repeat("b", 64)}

	report := Compare(before, after)
	identities := make(map[string]ArtifactIdentityRelation)
	for _, change := range report.Changes {
		identities[change.Field] = change.Identity
	}
	if got, want := identities["inputs.contract"], ArtifactIdentitySameBytes; got != want {
		t.Fatalf("contract identity = %q, want %q", got, want)
	}
	if got, want := identities["inputs.provider"], ArtifactIdentityAdded; got != want {
		t.Fatalf("provider identity = %q, want %q", got, want)
	}
}
