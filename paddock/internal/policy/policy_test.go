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
		{
			name:    "unsupported severity",
			change:  "unsupported-severity",
			wantErr: `rule "bad-severity" has unsupported severity "notice"`,
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
			case "unsupported-severity":
				contents += "  - id: bad-severity\n    kind: no-cycles\n    severity: notice\n"
			case "language: ''":
				contents = strings.Replace(contents, "  language: go", "  language: ''", 1)
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

func TestLoadDefaultsSourceUnitByLanguage(t *testing.T) {
	tests := []struct {
		name     string
		language string
		wantUnit string
	}{
		{name: "go", language: "go", wantUnit: "package"},
		{name: "typescript", language: "typescript", wantUnit: "file"},
		{name: "python", language: "python", wantUnit: "file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.yaml")
			contents := strings.Replace(validPolicy, "  language: go", "  language: "+test.language, 1)
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := policy.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Source.Unit != test.wantUnit {
				t.Fatalf("source.unit = %q, want %q", loaded.Source.Unit, test.wantUnit)
			}
		})
	}
}

func TestLoadAcceptsExternalGraphLanguageWithExplicitUnit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	contents := strings.Replace(validPolicy, "  language: go", "  language: rust\n  unit: file", 1)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source.Language != "rust" || loaded.Source.Unit != "file" {
		t.Fatalf("external graph policy = %#v", loaded.Source)
	}
}

func TestLoadDefaultsRuleSeverity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	contents := validPolicy + `  - id: default-severity
    kind: no-cycles
  - id: warning-severity
    kind: coverage
    severity: warning
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Rules[0].Severity != "error" || loaded.Rules[1].Severity != "warning" {
		t.Fatalf("rule severities = %#v, want error and warning", loaded.Rules)
	}
}

func TestLoadRejectsTransitiveNonRequiredRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	contents := validPolicy + `  - id: invalid-transitive
    kind: no-cycles
    transitive: true
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := policy.Load(path)
	if err == nil || !strings.Contains(err.Error(), "may use transitive only with required-dependency") {
		t.Fatalf("policy.Load error = %v, want transitive validation error", err)
	}
}

func TestValidateRejectsUnsupportedRuleOptions(t *testing.T) {
	tests := []struct {
		name    string
		rule    policy.Rule
		wantErr string
	}{
		{
			name:    "deny dependencies needs deny targets",
			rule:    policy.Rule{ID: "missing-deny", Kind: "deny-dependencies"},
			wantErr: `rule "missing-deny" needs deny targets`,
		},
		{
			name:    "mediated dependency needs allow-to targets",
			rule:    policy.Rule{ID: "missing-allow-to", Kind: "mediated-dependency"},
			wantErr: `rule "missing-allow-to" needs allow-to targets`,
		},
		{
			name:    "layer direction needs direction",
			rule:    policy.Rule{ID: "missing-direction", Kind: "layer-direction"},
			wantErr: `rule "missing-direction" needs direction`,
		},
		{
			name: "allow dependencies rejects direction",
			rule: policy.Rule{
				ID:        "allow-with-direction",
				Kind:      "allow-dependencies",
				Allow:     policy.Targets{{Literal: "internal/domain"}},
				Direction: "toward-lower-layer",
			},
			wantErr: `rule "allow-with-direction" kind "allow-dependencies" cannot use direction`,
		},
		{
			name: "no cycles rejects target selector",
			rule: policy.Rule{
				ID:   "cycles-with-to",
				Kind: "no-cycles",
				To:   policy.Selectors{{"role": "domain"}},
			},
			wantErr: `rule "cycles-with-to" kind "no-cycles" cannot use to`,
		},
		{
			name: "coverage rejects source selector",
			rule: policy.Rule{
				ID:   "coverage-with-from",
				Kind: "coverage",
				From: policy.Selectors{{"role": "domain"}},
			},
			wantErr: `rule "coverage-with-from" kind "coverage" cannot use from`,
		},
		{
			name: "component ownership rejects edge selector",
			rule: policy.Rule{
				ID:    "ownership-with-to",
				Kind:  "component-owns",
				Allow: policy.Targets{{Literal: "domain"}},
				To:    policy.Selectors{{"role": "domain"}},
			},
			wantErr: `rule "ownership-with-to" kind "component-owns" cannot use to`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := policy.Policy{
				Schema: "paddock.architecture/v1",
				Source: policy.Source{Language: "go", Roots: []string{"internal"}},
				Components: map[string]policy.Component{
					"source": {Match: policy.Patterns{"internal/**"}},
				},
				Rules: []policy.Rule{test.rule},
			}
			candidate.Normalize()
			err := candidate.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Policy.Validate error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestCanonicalPolicyHashIgnoresFormattingDefaultsAndOrdering(t *testing.T) {
	first := policy.Policy{
		Schema:  "paddock.architecture/v1",
		Project: "demo",
		Source:  policy.Source{Language: "go", Roots: []string{"internal", "cmd"}},
		Components: map[string]policy.Component{
			"domain": {Match: policy.Patterns{"internal/domain/**"}},
		},
		Rules: []policy.Rule{
			{ID: "z-rule", Kind: "no-cycles", Severity: "error"},
			{ID: "a-rule", Kind: "coverage"},
		},
	}
	second := first
	second.Source.Roots = []string{"cmd", "internal"}
	second.Rules = []policy.Rule{
		{ID: "a-rule", Kind: "coverage", Severity: "error"},
		{ID: "z-rule", Kind: "no-cycles", Severity: "error"},
	}
	firstHash, err := policy.CanonicalSHA256(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := policy.CanonicalSHA256(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("canonical hashes differ for equivalent policies: %s != %s", firstHash, secondHash)
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
