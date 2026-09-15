package artifact

import "testing"

func TestDigestBytesIsStableAndExplicit(t *testing.T) {
	got := DigestBytes([]byte("lockwood"))
	if err := ValidateDigest(got); err != nil {
		t.Fatal(err)
	}
	if got != "sha256:70f756d0618a1ff2e4cb0199a3692b27e20fae283b96089dcdc55fbde210bfcc" {
		t.Fatalf("digest = %q", got)
	}
}

func TestReferenceForRequiresMediaType(t *testing.T) {
	if _, err := ReferenceFor([]byte("data"), "", "data.json"); err == nil {
		t.Fatal("ReferenceFor accepted an empty media type")
	}
}

func TestValidateDigestRejectsUnsafeValues(t *testing.T) {
	for _, digest := range []string{"", "sha256:ABC", "sha256:../artifact", "md5:00000000000000000000000000000000"} {
		if err := ValidateDigest(digest); err == nil {
			t.Fatalf("ValidateDigest accepted %q", digest)
		}
	}
}
