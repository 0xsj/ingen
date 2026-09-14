package checker_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/paddock/internal/checker"
)

func TestServiceFixtures(t *testing.T) {
	repoRoot := repositoryRoot(t)
	fixtures := []struct {
		name      string
		policy    string
		source    string
		wantOK    bool
		wantRules []string
	}{
		{
			name:   "layered good",
			policy: "layered.yaml",
			source: "layered-go/good",
			wantOK: true,
		},
		{
			name:      "layered violation",
			policy:    "layered.yaml",
			source:    "layered-go/violating",
			wantRules: []string{"dependencies-point-inward"},
		},
		{
			name:   "hexagonal good",
			policy: "hexagonal.yaml",
			source: "hexagonal-go/good",
			wantOK: true,
		},
		{
			name:      "hexagonal violation",
			policy:    "hexagonal.yaml",
			source:    "hexagonal-go/violating",
			wantRules: []string{"domain-is-pure"},
		},
		{
			name:   "modular good",
			policy: "modular-monolith.yaml",
			source: "modular-monolith-go/good",
			wantOK: true,
		},
		{
			name:      "modular violation",
			policy:    "modular-monolith.yaml",
			source:    "modular-monolith-go/violating",
			wantRules: []string{"cross-context-access-is-mediated", "context-internals-are-private"},
		},
		{
			name:      "cycle violation",
			policy:    "cyclic.yaml",
			source:    "cyclic-go/violating",
			wantRules: []string{"no-cycles"},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			result, err := checker.Check(
				filepath.Join(repoRoot, "paddock", "examples", "services", fixture.source),
				filepath.Join(repoRoot, "paddock", "examples", fixture.policy),
			)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.OK(); got != fixture.wantOK {
				t.Fatalf("result OK = %v, want %v; findings: %#v", got, fixture.wantOK, result.Findings)
			}
			gotRules := make(map[string]bool)
			for _, finding := range result.Findings {
				gotRules[finding.RuleID] = true
			}
			for _, rule := range fixture.wantRules {
				if !gotRules[rule] {
					t.Errorf("missing expected rule %q; findings: %#v", rule, result.Findings)
				}
			}
		})
	}
}

func TestCoverageRejectsUnmatchedAndAmbiguousPackages(t *testing.T) {
	repoRoot := repositoryRoot(t)
	tests := []struct {
		name       string
		components string
		want       string
	}{
		{
			name: "unmatched package",
			components: `components:
  domain:
    match: internal/domain/**
`,
			want: "every source unit must match exactly one component",
		},
		{
			name: "ambiguous package",
			components: `components:
  first:
    match: internal/**
  second:
    match: internal/**
`,
			want: "source unit matches multiple components",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policyPath := filepath.Join(t.TempDir(), "coverage.yaml")
			contents := `schema: paddock.architecture/v1
project: coverage-test
source:
  language: go
  roots: [cmd, internal]
` + test.components + `rules:
  - id: complete-classification
    kind: coverage
`
			if err := os.WriteFile(policyPath, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			result, err := checker.Check(
				filepath.Join(repoRoot, "paddock", "examples", "services", "layered-go", "good"),
				policyPath,
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, finding := range result.Findings {
				if strings.Contains(finding.Message, test.want) {
					return
				}
			}
			t.Fatalf("no finding contained %q; findings: %#v", test.want, result.Findings)
		})
	}
}

func TestWaiversPreserveActiveFindingsAndRejectExpiredOnes(t *testing.T) {
	repoRoot := repositoryRoot(t)
	tests := []struct {
		name       string
		expires    string
		wantOK     bool
		wantStatus string
	}{
		{
			name:       "active waiver",
			expires:    "2099-01-01",
			wantOK:     true,
			wantStatus: "applied",
		},
		{
			name:       "expired waiver",
			expires:    "2000-01-01",
			wantOK:     false,
			wantStatus: "expired",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policyPath := filepath.Join(t.TempDir(), "waiver.yaml")
			contents := `schema: paddock.architecture/v1
project: waiver-test
source:
  language: go
  roots: [cmd, internal]
components:
  domain:
    match: internal/{context}/domain/**
    labels:
      role: domain
      context: "{context}"
rules:
  - id: domain-is-pure
    kind: allow-dependencies
    from: {role: domain}
    allow:
      - standard-library:errors
      - standard-library:fmt
    message: domain dependencies must be approved
waivers:
  - rule: domain-is-pure
    from: internal/orders/domain/order.go
    to: net/http
    reason: legacy HTTP coupling is tracked for removal
    owner: platform-team
    expires: ` + test.expires + `
`
			if err := os.WriteFile(policyPath, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			result, err := checker.Check(
				filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "violating"),
				policyPath,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.OK() != test.wantOK {
				t.Fatalf("result OK = %v, want %v; findings: %#v", result.OK(), test.wantOK, result.Findings)
			}
			if len(result.Findings) != 1 {
				t.Fatalf("finding count = %d, want 1; findings: %#v", len(result.Findings), result.Findings)
			}
			finding := result.Findings[0]
			if finding.WaiverStatus != test.wantStatus {
				t.Fatalf("waiver status = %q, want %q", finding.WaiverStatus, test.wantStatus)
			}
			if finding.WaiverOwner != "platform-team" || finding.WaiverReason == "" {
				t.Fatalf("waiver metadata was not preserved: %#v", finding)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}
