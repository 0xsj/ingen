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
		{
			name:   "feature sliced TypeScript good",
			policy: "feature-sliced-frontend.yaml",
			source: "feature-sliced-ts/good",
			wantOK: true,
		},
		{
			name:      "feature sliced TypeScript violation",
			policy:    "feature-sliced-frontend.yaml",
			source:    "feature-sliced-ts/violating",
			wantRules: []string{"features-do-not-cross", "shared-is-feature-free", "no-unresolved-imports"},
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

func TestCheckPolicyAppliesSourceDiscoveryScope(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "scoped.yaml")
	contents := `schema: paddock.architecture/v1
project: scoped-typescript
source:
  language: typescript
  unit: file
  roots: [src]
  include: [src/entities/**]
  exclude: [src/entities/**/*.spec.ts]
components:
  entity:
    match: src/entities/**
    labels:
      role: entity
rules:
  - id: complete-classification
    kind: coverage
`
	if err := os.WriteFile(policyPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := checker.Check(
		filepath.Join(repoRoot, "paddock", "examples", "services", "feature-sliced-ts", "good"),
		policyPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK() || result.PackageCount != 1 || result.EdgeCount != 0 {
		t.Fatalf("source discovery scope was not applied: %#v", result)
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

func TestComponentOwnershipRejectsUndeclaredComponent(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "ownership.yaml")
	contents := `schema: paddock.architecture/v1
project: ownership-test
source:
  language: go
  roots: [cmd, internal]
components:
  domain:
    match: internal/domain/**
  service:
    match: internal/service/**
  repository:
    match: internal/repository/**
  transport:
    match: internal/transport/**
  command:
    match: cmd/**
rules:
  - id: bounded-context-components
    kind: component-owns
    allow: [domain, service, repository, transport]
    message: bounded context uses an undeclared component
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
	if result.OK() {
		t.Fatalf("component ownership violation was not blocking: %#v", result.Findings)
	}
	for _, finding := range result.Findings {
		if finding.RuleID == "bounded-context-components" && finding.From == "cmd/api" && finding.FromComponent == "command" {
			return
		}
	}
	t.Fatalf("component ownership finding missing: %#v", result.Findings)
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
			if len(result.Waivers) != 1 || result.Waivers[0].Status != test.wantStatus {
				t.Fatalf("waiver summary = %#v, want status %q", result.Waivers, test.wantStatus)
			}
		})
	}
}

func TestUnusedWaiverIsReported(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "unused-waiver.yaml")
	contents := `schema: paddock.architecture/v1
project: unused-waiver-test
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
    from: internal/orders/domain/does-not-exist.go
    reason: stale waiver should be visible
    owner: platform-team
    expires: 2099-01-01
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
	if result.OK() {
		t.Fatal("result passed with an unused waiver")
	}
	if len(result.Waivers) != 1 || result.Waivers[0].Status != "unused" {
		t.Fatalf("waiver summary = %#v, want one unused waiver", result.Waivers)
	}
}

func TestRequiredDependencyFindsMissingBoundary(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "required.yaml")
	contents := `schema: paddock.architecture/v1
project: required-test
source:
  language: go
  roots: [internal]
components:
  domain:
    match: internal/domain/**
    labels:
      role: domain
rules:
  - id: domain-requires-port
    kind: required-dependency
    from: {role: domain}
    allow:
      - standard-library:errors
    message: domain packages must depend on the error boundary
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
		if finding.RuleID == "domain-requires-port" {
			return
		}
	}
	t.Fatalf("required dependency finding missing: %#v", result.Findings)
}

func TestWarningFindingsDoNotBlock(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "warning.yaml")
	contents := `schema: paddock.architecture/v1
project: warning-test
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
  - id: domain-is-pure-warning
    kind: allow-dependencies
    severity: warning
    from: {role: domain}
    allow:
      - standard-library:errors
      - standard-library:fmt
    message: domain dependencies should be approved
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
	if !result.OK() || len(result.Findings) != 1 || result.Findings[0].Severity != "warning" {
		t.Fatalf("warning finding changed the verdict: ok=%v findings=%#v", result.OK(), result.Findings)
	}
}

func TestRequiredDependencySupportsTransitiveReachability(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "transitive.yaml")
	contents := `schema: paddock.architecture/v1
project: transitive-test
source:
  language: go
  roots: [cmd, internal]
components:
  api:
    match: cmd/**
    labels:
      role: api
  service:
    match: internal/service/**
    labels:
      role: service
  domain:
    match: internal/domain/**
    labels:
      role: domain
rules:
  - id: api-reaches-domain
    kind: required-dependency
    from: {role: api}
    allow:
      - {role: domain}
    transitive: true
    message: API code must reach the domain boundary
`
	if err := os.WriteFile(policyPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "layered-go", "good")
	transitive, err := checker.Check(source, policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !transitive.OK() {
		t.Fatalf("transitive requirement failed: %#v", transitive.Findings)
	}

	directPolicy := strings.Replace(contents, "    transitive: true\n", "", 1)
	if err := os.WriteFile(policyPath, []byte(directPolicy), 0o600); err != nil {
		t.Fatal(err)
	}
	direct, err := checker.Check(source, policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if direct.OK() || len(direct.Findings) != 1 {
		t.Fatalf("direct requirement unexpectedly passed: ok=%v findings=%#v", direct.OK(), direct.Findings)
	}
}

func TestCycleRuleScopesToSelectedRoots(t *testing.T) {
	repoRoot := repositoryRoot(t)
	policyPath := filepath.Join(t.TempDir(), "scoped-cycle.yaml")
	contents := `schema: paddock.architecture/v1
project: scoped-cycle-test
source:
  language: go
  roots: [internal/alpha]
components:
  alpha:
    match: internal/alpha/**
    labels:
      role: alpha
rules:
  - id: alpha-must-be-acyclic
    kind: no-cycles
    from: {role: alpha}
    message: selected alpha sources must remain acyclic
`
	if err := os.WriteFile(policyPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := checker.Check(
		filepath.Join(repoRoot, "paddock", "examples", "services", "cyclic-go", "violating"),
		policyPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK() || len(result.Findings) != 0 {
		t.Fatalf("scoped cycle rule reported an out-of-scope cycle: ok=%v findings=%#v", result.OK(), result.Findings)
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
