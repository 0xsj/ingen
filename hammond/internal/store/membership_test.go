package store

import (
	"errors"
	"os"
	"strings"
	"testing"

	"ingen/hammond/internal/governance"
)

func TestFileMembershipVersionStoreAcceptsOnlyNewerReferences(t *testing.T) {
	versionStore, err := NewFileMembershipVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := testMembershipSnapshot(2, strings.Repeat("a", 64))
	if err := versionStore.Accept(first); err != nil {
		t.Fatal(err)
	}
	if err := versionStore.Accept(first); err != nil {
		t.Fatalf("idempotent membership acceptance error = %v", err)
	}
	current, err := versionStore.Current(first.Reference.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Equal(first.Reference) {
		t.Fatalf("current = %#v, want %#v", current, first.Reference)
	}

	stale := testMembershipSnapshot(1, strings.Repeat("b", 64))
	if err := versionStore.Accept(stale); err == nil || !errors.Is(err, ErrMembershipConflict) {
		t.Fatalf("stale acceptance error = %v, want ErrMembershipConflict", err)
	}
	sameVersion := testMembershipSnapshot(2, strings.Repeat("b", 64))
	if err := versionStore.Accept(sameVersion); err == nil || !errors.Is(err, ErrMembershipConflict) {
		t.Fatalf("same-version acceptance error = %v, want ErrMembershipConflict", err)
	}

	next := testMembershipSnapshot(3, strings.Repeat("c", 64))
	if err := versionStore.Accept(next); err != nil {
		t.Fatalf("newer membership acceptance error = %v", err)
	}
	current, err = versionStore.Current(next.Reference.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Equal(next.Reference) {
		t.Fatalf("current after update = %#v, want %#v", current, next.Reference)
	}
}

func TestFileMembershipVersionStoreCoordinatesInstances(t *testing.T) {
	root := t.TempDir()
	first, err := NewFileMembershipVersionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileMembershipVersionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Accept(testMembershipSnapshot(1, strings.Repeat("a", 64))); err != nil {
		t.Fatal(err)
	}
	if err := second.Accept(testMembershipSnapshot(2, strings.Repeat("b", 64))); err != nil {
		t.Fatal(err)
	}
	current, err := first.Current("directory-reviewers")
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != 2 {
		t.Fatalf("current version = %d, want 2", current.Version)
	}
}

func TestFileMembershipVersionStoreRejectsMissingCurrent(t *testing.T) {
	versionStore, err := NewFileMembershipVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := versionStore.Current("directory-reviewers"); err == nil || !errors.Is(err, ErrMembershipVersionNotFound) {
		t.Fatalf("missing current error = %v, want ErrMembershipVersionNotFound", err)
	}
}

func TestFileMembershipVersionStoreFailsClosedOnCorruptLedger(t *testing.T) {
	corruptions := []string{
		`{"schema":"ingen.hammond-membership-version/v1","id":"directory-reviewers","version":1,"membership_schema":"ingen.hammond-membership/v1","artifact":{"uri":"membership.json","sha256":"not-a-digest"}}`,
		`{"schema":"wrong","id":"directory-reviewers","version":1,"membership_schema":"ingen.hammond-membership/v1","artifact":{"uri":"membership.json","sha256":"` + strings.Repeat("a", 64) + `"}}`,
		`{"schema":"ingen.hammond-membership-version/v1","id":"directory-reviewers"} trailing`,
	}
	for _, corruption := range corruptions {
		versionStore, err := NewFileMembershipVersionStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		path := versionStore.pathFor("directory-reviewers")
		if err := os.WriteFile(path, []byte(corruption), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := versionStore.Current("directory-reviewers"); err == nil || errors.Is(err, ErrMembershipVersionNotFound) {
			t.Fatalf("corruption %q returned error = %v, want structural failure", corruption, err)
		}
	}
}

func TestFileMembershipVersionStoreBindsIdentityToLedgerPath(t *testing.T) {
	versionStore, err := NewFileMembershipVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := versionStore.pathFor("directory-reviewers")
	if err := versionStore.write(path, testMembershipSnapshot(1, strings.Repeat("a", 64)).Reference); err != nil {
		t.Fatal(err)
	}
	other := testMembershipSnapshot(1, strings.Repeat("b", 64)).Reference
	other.ID = "different-membership"
	if err := versionStore.write(path, other); err != nil {
		t.Fatal(err)
	}
	if _, err := versionStore.Current("directory-reviewers"); err == nil || !strings.Contains(err.Error(), "does not match its ledger path") {
		t.Fatalf("wrong ledger identity error = %v, want identity binding failure", err)
	}
}

func testMembershipSnapshot(version int, digest string) governance.MembershipSnapshot {
	return governance.MembershipSnapshot{
		Reference: governance.MembershipReference{
			ID:      "directory-reviewers",
			Version: version,
			Schema:  governance.MembershipSchema,
			Artifact: governance.Artifact{
				URI:    "membership.json",
				SHA256: digest,
			},
		},
		IssuedAt: "2026-09-15T00:00:00Z",
		Grants:   []governance.AuthorityGrant{},
	}
}
