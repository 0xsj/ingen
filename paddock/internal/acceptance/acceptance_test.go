package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/paddock/internal/baseline"
	"ingen/paddock/internal/explain"
	"ingen/paddock/internal/model"
)

func TestCLIEndToEnd(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)

	fixtures := []struct {
		name       string
		policy     string
		source     string
		exitCode   int
		wantOutput string
	}{
		{
			name:       "layered good",
			policy:     "layered.yaml",
			source:     "layered-go/good",
			exitCode:   0,
			wantOutput: "PASS",
		},
		{
			name:       "layered violation",
			policy:     "layered.yaml",
			source:     "layered-go/violating",
			exitCode:   1,
			wantOutput: "dependencies-point-inward",
		},
		{
			name:       "hexagonal good",
			policy:     "hexagonal.yaml",
			source:     "hexagonal-go/good",
			exitCode:   0,
			wantOutput: "PASS",
		},
		{
			name:       "hexagonal violation",
			policy:     "hexagonal.yaml",
			source:     "hexagonal-go/violating",
			exitCode:   1,
			wantOutput: "domain-is-pure",
		},
		{
			name:       "modular good",
			policy:     "modular-monolith.yaml",
			source:     "modular-monolith-go/good",
			exitCode:   0,
			wantOutput: "PASS",
		},
		{
			name:       "modular violation",
			policy:     "modular-monolith.yaml",
			source:     "modular-monolith-go/violating",
			exitCode:   1,
			wantOutput: "cross-context-access-is-mediated",
		},
		{
			name:       "cycle violation",
			policy:     "cyclic.yaml",
			source:     "cyclic-go/violating",
			exitCode:   1,
			wantOutput: "no-cycles",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			output, exitCode := runCLI(t, cli, repoRoot,
				"check",
				filepath.Join(repoRoot, "paddock", "examples", "services", fixture.source),
				"--policy", filepath.Join(repoRoot, "paddock", "examples", fixture.policy),
			)
			if exitCode != fixture.exitCode {
				t.Fatalf("exit code = %d, want %d; output:\n%s", exitCode, fixture.exitCode, output)
			}
			if !strings.Contains(output, fixture.wantOutput) {
				t.Fatalf("output does not contain %q:\n%s", fixture.wantOutput, output)
			}
		})
	}
}

func TestCLIJSONReport(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	output, exitCode := runCLI(t, cli, repoRoot,
		"check",
		filepath.Join(repoRoot, "paddock", "examples", "services", "modular-monolith-go", "violating"),
		"--policy", filepath.Join(repoRoot, "paddock", "examples", "modular-monolith.yaml"),
		"--format", "json",
	)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", exitCode, output)
	}
	var result model.Result
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode JSON report: %v\n%s", err, output)
	}
	if result.Schema != "paddock.report/v1" {
		t.Fatalf("report schema = %q, want paddock.report/v1", result.Schema)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("finding count = %d, want 2; findings: %#v", len(result.Findings), result.Findings)
	}
}

func TestCLIInvalidPolicyExitCode(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	policyPath := filepath.Join(t.TempDir(), "invalid.yaml")
	policy := `schema: paddock.architecture/v1
project: invalid
source:
  language: go
  roots: [internal]
components:
  source:
    match: internal/**
rules:
  - id: duplicate
    kind: no-cycles
  - id: duplicate
    kind: no-cycles
`
	if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
		t.Fatal(err)
	}

	output, exitCode := runCLI(t, cli, repoRoot,
		"check", repoRoot, "--policy", policyPath,
	)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, `duplicate rule id "duplicate"`) {
		t.Fatalf("output does not identify invalid policy:\n%s", output)
	}
}

func TestCLIWaiverExitCodes(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "violating")

	tests := []struct {
		name       string
		expires    string
		exitCode   int
		wantOutput string
	}{
		{
			name:       "active waiver passes",
			expires:    "2099-01-01",
			exitCode:   0,
			wantOutput: "waived by platform-team",
		},
		{
			name:       "expired waiver fails",
			expires:    "2000-01-01",
			exitCode:   1,
			wantOutput: "waiver expired on 2000-01-01",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policyPath := filepath.Join(t.TempDir(), "waiver.yaml")
			policy := `schema: paddock.architecture/v1
project: waiver-cli-test
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
			if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
				t.Fatal(err)
			}
			output, exitCode := runCLI(t, cli, repoRoot,
				"check", source, "--policy", policyPath,
			)
			if exitCode != test.exitCode {
				t.Fatalf("exit code = %d, want %d; output:\n%s", exitCode, test.exitCode, output)
			}
			if !strings.Contains(output, test.wantOutput) {
				t.Fatalf("output does not contain %q:\n%s", test.wantOutput, output)
			}
		})
	}
}

func TestCLIBaselineLifecycle(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "modular-monolith-go", "violating")
	policy := filepath.Join(repoRoot, "paddock", "examples", "modular-monolith.yaml")
	baselinePath := filepath.Join(t.TempDir(), "paddock-baseline.json")
	cleanPolicyPath := filepath.Join(t.TempDir(), "clean.yaml")
	cleanPolicy := `schema: paddock.architecture/v1
project: clean-policy
source:
  language: go
  roots: [cmd, internal, pkg]
components:
  source:
    match:
      - cmd/**
      - internal/**
      - pkg/**
rules: []
`
	if err := os.WriteFile(cleanPolicyPath, []byte(cleanPolicy), 0o600); err != nil {
		t.Fatal(err)
	}

	output, exitCode := runCLI(t, cli, repoRoot,
		"baseline", source, "--policy", policy, "--output", baselinePath,
	)
	if exitCode != 0 {
		t.Fatalf("baseline exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	snapshot, err := baseline.Load(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 2 {
		t.Fatalf("baseline entries = %d, want 2", len(snapshot.Entries))
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policy, "--baseline", baselinePath,
	)
	if exitCode != 0 {
		t.Fatalf("baselined check exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "baseline 2/2 matched") {
		t.Fatalf("baseline summary missing from output:\n%s", output)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policy, "--baseline", baselinePath, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("JSON baselined check exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	var report model.Result
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode baselined JSON report: %v\n%s", err, output)
	}
	if report.Baseline == nil || report.Baseline.Matched != 2 {
		t.Fatalf("baseline summary missing from JSON report: %#v", report.Baseline)
	}
	for _, finding := range report.Findings {
		if !finding.Baselined {
			t.Fatalf("finding was not marked baselined: %#v", finding)
		}
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", cleanPolicyPath, "--baseline", baselinePath,
	)
	if exitCode != 0 {
		t.Fatalf("stale baseline check exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "2 stale") {
		t.Fatalf("stale baseline summary missing from output:\n%s", output)
	}

	filtered := snapshot
	filtered.Entries = nil
	for _, entry := range snapshot.Entries {
		if entry.RuleID == "context-internals-are-private" {
			filtered.Entries = append(filtered.Entries, entry)
		}
	}
	if err := baseline.Save(baselinePath, filtered); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policy, "--baseline", baselinePath,
	)
	if exitCode != 1 {
		t.Fatalf("check with new finding exit code = %d, want 1; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "cross-context-access-is-mediated") {
		t.Fatalf("new finding missing from output:\n%s", output)
	}
}

func TestCLIExplainReport(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	reportPath := filepath.Join(t.TempDir(), "paddock-report.json")
	output, exitCode := runCLI(t, cli, repoRoot,
		"check",
		filepath.Join(repoRoot, "paddock", "examples", "services", "modular-monolith-go", "violating"),
		"--policy", filepath.Join(repoRoot, "paddock", "examples", "modular-monolith.yaml"),
		"--format", "json",
	)
	if exitCode != 1 {
		t.Fatalf("check exit code = %d, want 1; output:\n%s", exitCode, output)
	}
	if err := os.WriteFile(reportPath, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}

	output, exitCode = runCLI(t, cli, repoRoot, "explain", reportPath)
	if exitCode != 0 {
		t.Fatalf("explain exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "EXPLAIN FAIL") ||
		!strings.Contains(output, "cross-context-access-is-mediated") ||
		!strings.Contains(output, "Route the dependency through") {
		t.Fatalf("text explanation missing expected evidence:\n%s", output)
	}

	output, exitCode = runCLI(t, cli, repoRoot, "explain", reportPath, "--format", "json")
	if exitCode != 0 {
		t.Fatalf("JSON explain exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	var document explain.Document
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode explanation JSON: %v\n%s", err, output)
	}
	if document.Schema != "paddock.explanation/v1" || document.Status != "FAIL" || len(document.Findings) != 2 {
		t.Fatalf("unexpected explanation document: %#v", document)
	}
}

func buildCLI(t *testing.T, repoRoot string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "paddock")
	cmd := exec.Command("go", "build", "-o", output, "./paddock/cmd/paddock")
	cmd.Dir = repoRoot
	cmd.Env = toolchainEnv(t)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build paddock: %v\n%s", err, data)
	}
	return output
}

func runCLI(t *testing.T, cli, repoRoot string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(cli, args...)
	cmd.Dir = repoRoot
	data, err := cmd.CombinedOutput()
	if err == nil {
		return string(data), 0
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		return string(data), exitError.ExitCode()
	}
	t.Fatalf("run paddock: %v\n%s", err, data)
	return "", -1
}

func toolchainEnv(t *testing.T) []string {
	t.Helper()
	env := append([]string{}, os.Environ()...)
	return append(env, "GOCACHE="+filepath.Join(t.TempDir(), "go-build"))
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}
