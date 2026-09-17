package attestation

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAuthorizationHandoffFixturesValidateAtExplicitTime(t *testing.T) {
	evaluationTime := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)
	valid := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	if err := valid.ValidateAt(evaluationTime); err != nil {
		t.Fatalf("valid authorization handoff rejected: %v", err)
	}
	encoded, err := MarshalCanonicalAuthorizationHandoff(valid)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../testdata/valid-authorization-handoff-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, bytes.TrimSpace(fixture)) {
		t.Fatalf("canonical authorization handoff differs from fixture:\n got %s\nwant %s", encoded, fixture)
	}
	digest, err := AuthorizationHandoffDigest(valid)
	if err != nil || digest == "" {
		t.Fatalf("authorization handoff digest = %q, err = %v", digest, err)
	}

	referenceOnly := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-reference-only-v1.json")
	if err := referenceOnly.ValidateAt(evaluationTime); err != nil {
		t.Fatalf("reference-only authorization handoff rejected: %v", err)
	}
	if referenceOnly.Principal.AssertionDigest != "" {
		t.Fatal("reference-only fixture unexpectedly authenticates the principal")
	}

	for _, test := range []struct {
		name string
		file string
		want string
	}{
		{name: "target action mismatch", file: "invalid-authorization-handoff-target-action-v1.json", want: "does not match target kind"},
		{name: "missing authorization assertion", file: "invalid-authorization-handoff-missing-assertion-v1.json", want: "invalid authorization assertion digest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/" + test.file)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.TrimSpace(data)
			if _, err := UnmarshalCanonicalAuthorizationHandoff(data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	expired := loadAuthorizationHandoffFixture(t, "invalid-authorization-handoff-expired-v1.json")
	if err := expired.ValidateAt(evaluationTime); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired handoff accepted: %v", err)
	}
}

func TestNewAuthorizationHandoffDerivesActionFromTarget(t *testing.T) {
	fixture := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	built, err := NewAuthorizationHandoff(fixture.Target, fixture.Principal, fixture.Authorization)
	if err != nil {
		t.Fatal(err)
	}
	if built.Schema != AuthorizationHandoffSchema || built.Action != fixture.Target.Kind || built != fixture {
		t.Fatalf("built authorization handoff = %+v, fixture = %+v", built, fixture)
	}

	invalidTarget := fixture.Target
	invalidTarget.Kind = "unsupported-target"
	if _, err := NewAuthorizationHandoff(invalidTarget, fixture.Principal, fixture.Authorization); err == nil || !strings.Contains(err.Error(), "unsupported authorization handoff target kind") {
		t.Fatalf("unsupported target accepted: %v", err)
	}
}

func TestAuthorizationHandoffRejectsNonCanonicalAndInvalidWindows(t *testing.T) {
	valid := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	encoded, err := MarshalCanonicalAuthorizationHandoff(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "leading whitespace", data: append([]byte(" \n"), encoded...), want: "not canonical JSON"},
		{name: "unknown field", data: append(encoded[:len(encoded)-1], []byte(",\"extra\":true}")...), want: "decode authorization handoff"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := UnmarshalCanonicalAuthorizationHandoff(test.data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	invalidWindow := valid
	invalidWindow.Authorization.NotBefore = "2026-09-17T13:00:00Z"
	invalidWindow.Authorization.ExpiresAt = "2026-09-17T12:00:00Z"
	if _, err := MarshalCanonicalAuthorizationHandoff(invalidWindow); err == nil || !strings.Contains(err.Error(), "expires_at must be after") {
		t.Fatalf("invalid validity window accepted: %v", err)
	}
	if err := valid.ValidateAt(time.Time{}); err == nil || !strings.Contains(err.Error(), "evaluation time is required") {
		t.Fatalf("missing evaluation time accepted: %v", err)
	}
	if err := valid.ValidateAt(time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("exclusive expiry boundary accepted: %v", err)
	}
}

func loadAuthorizationHandoffFixture(t *testing.T, name string) AuthorizationHandoff {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.TrimSpace(data)
	handoff, err := UnmarshalCanonicalAuthorizationHandoff(data)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return handoff
}
