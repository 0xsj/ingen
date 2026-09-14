package version

import "testing"

func TestDevelopmentDefaults(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q, want dev", Version)
	}
	if Commit != "unknown" {
		t.Fatalf("Commit = %q, want unknown", Commit)
	}
	if BuildDate != "unknown" {
		t.Fatalf("BuildDate = %q, want unknown", BuildDate)
	}
}
