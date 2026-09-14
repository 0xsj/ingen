package policy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/paddock/internal/policy"
)

func TestLoadRejectsInvalidPolicies(t *testing.T) {
	tests := []struct {
		name    string
		change  string
		wantErr string
	}{
		{
			name:    "wrong schema",
			change:  "schema: other/v1",
			wantErr: "policy schema must be paddock.architecture/v1",
		},
		{
			name:    "missing source language",
			change:  "language: ''",
			wantErr: "source.language is required",
		},
		{
			name:    "unsupported language",
			change:  "language: rust",
			wantErr: `source.language "rust" is not supported yet`,
		},
		{
			name:    "missing roots",
			change:  "roots: []",
			wantErr: "source.roots must not be empty",
		},
		{
			name:    "missing components",
			change:  "components: {}",
			wantErr: "components must not be empty",
		},
		{
			name:    "duplicate rule IDs",
			change:  "duplicate-rules",
			wantErr: `duplicate rule id "same"`,
		},
		{
			name:    "unsupported rule kind",
			change:  "unsupported-rule",
			wantErr: `rule "bad-rule" has unsupported kind "unknown"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.yaml")
			contents := validPolicy
			switch test.change {
			case "duplicate-rules":
				contents += "  - id: same\n    kind: no-cycles\n  - id: same\n    kind: coverage\n"
			case "unsupported-rule":
				contents += "  - id: bad-rule\n    kind: unknown\n"
			case "language: ''":
				contents = strings.Replace(contents, "  language: go", "  language: ''", 1)
			case "language: rust":
				contents = strings.Replace(contents, "  language: go", "  language: rust", 1)
			case "roots: []":
				contents = strings.Replace(contents, "  roots: [internal]", "  roots: []", 1)
			case "components: {}":
				contents = strings.Replace(contents, "components:\n  source:\n    match: internal/**", "components: {}", 1)
			default:
				contents = strings.Replace(contents, "schema: paddock.architecture/v1", test.change, 1)
			}
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := policy.Load(path)
			if err == nil {
				t.Fatal("policy.Load succeeded, want error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadRejectsInvalidWaivers(t *testing.T) {
	tests := []struct {
		name    string
		change  string
		wantErr string
	}{
		{
			name:    "missing rule",
			change:  "rule: ''",
			wantErr: "every waiver needs a rule",
		},
		{
			name:    "unknown rule",
			change:  "rule: unknown",
			wantErr: `waiver references unknown rule "unknown"`,
		},
		{
			name:    "missing from",
			change:  "from: ''",
			wantErr: `waiver for rule "known" needs from`,
		},
		{
			name:    "missing reason",
			change:  "reason: ''",
			wantErr: `waiver for rule "known" needs a reason`,
		},
		{
			name:    "missing owner",
			change:  "owner: ''",
			wantErr: `waiver for rule "known" needs an owner`,
		},
		{
			name:    "invalid expiry",
			change:  "expires: tomorrow",
			wantErr: `waiver for rule "known" has invalid expires date "tomorrow"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.yaml")
			contents := validWaiverPolicy
			switch test.name {
			case "missing rule", "unknown rule":
				contents = strings.Replace(contents, "rule: known", test.change, 1)
			case "missing from":
				contents = strings.Replace(contents, "from: internal/legacy.go", test.change, 1)
			case "missing reason":
				contents = strings.Replace(contents, "reason: migration", test.change, 1)
			case "missing owner":
				contents = strings.Replace(contents, "owner: platform", test.change, 1)
			case "invalid expiry":
				contents = strings.Replace(contents, "expires: 2099-01-01", test.change, 1)
			}
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := policy.Load(path)
			if err == nil {
				t.Fatal("policy.Load succeeded, want error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, test.wantErr)
			}
		})
	}
}

const validPolicy = `schema: paddock.architecture/v1
project: test
source:
  language: go
  roots: [internal]
components:
  source:
    match: internal/**
rules:
`

const validWaiverPolicy = `schema: paddock.architecture/v1
project: test
source:
  language: go
  roots: [internal]
components:
  source:
    match: internal/**
rules:
  - id: known
    kind: no-cycles
waivers:
  - rule: known
    from: internal/legacy.go
    reason: migration
    owner: platform
    expires: 2099-01-01
`
