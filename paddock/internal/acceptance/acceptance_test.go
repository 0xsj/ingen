package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/paddock/internal/artifact"
	"ingen/paddock/internal/baseline"
	"ingen/paddock/internal/explain"
	"ingen/paddock/internal/model"
	"ingen/paddock/internal/policy"
	"ingen/paddock/internal/policydiff"
	"ingen/paddock/internal/policylock"
	"ingen/paddock/internal/policyreview"
	"ingen/paddock/internal/policytest"
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
		{
			name:       "feature sliced TypeScript good",
			policy:     "feature-sliced-frontend.yaml",
			source:     "feature-sliced-ts/good",
			exitCode:   0,
			wantOutput: "PASS",
		},
		{
			name:       "feature sliced TypeScript violation",
			policy:     "feature-sliced-frontend.yaml",
			source:     "feature-sliced-ts/violating",
			exitCode:   1,
			wantOutput: "features-do-not-cross",
		},
		{
			name:       "Python hexagonal good",
			policy:     "python-hexagonal.yaml",
			source:     "python-hexagonal/good",
			exitCode:   0,
			wantOutput: "PASS",
		},
		{
			name:       "Python hexagonal violation",
			policy:     "python-hexagonal.yaml",
			source:     "python-hexagonal/violating",
			exitCode:   1,
			wantOutput: "domain-is-pure",
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

func TestOverwatchReviewPolicyLoads(t *testing.T) {
	repoRoot := repositoryRoot(t)
	path := filepath.Join(repoRoot, "paddock", "examples", "overwatch", "overwatch-backend-review.yaml")
	loaded, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source.Language != "go" || loaded.Source.Unit != "package" || len(loaded.Components) != 8 || len(loaded.Rules) != 8 {
		t.Fatalf("unexpected Overwatch review policy: %#v", loaded)
	}
}

func TestCLIPolicyTestManifest(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	manifestPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.policy-tests.yaml")

	output, exitCode := runCLI(t, cli, repoRoot, "policy", "test", "--policy", policyPath, "--cases", manifestPath)
	if exitCode != 0 || !strings.Contains(output, "POLICY-TEST PASS") || !strings.Contains(output, "good-service") || !strings.Contains(output, "violating-service") {
		t.Fatalf("policy test manifest failed: exit=%d output:\n%s", exitCode, output)
	}

	output, exitCode = runCLI(t, cli, repoRoot, "policy", "test", "--policy", policyPath, "--cases", manifestPath, "--format", "json")
	if exitCode != 0 {
		t.Fatalf("JSON policy test manifest exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	var document policytest.Document
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode policy test document: %v\n%s", err, output)
	}
	if document.Schema != policytest.DocumentSchema || document.Status != "PASS" || document.Passed != 2 || document.Failed != 0 {
		t.Fatalf("unexpected policy test document: %#v", document)
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

func TestCLIGraphCommand(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "feature-sliced-ts", "good")
	policy := filepath.Join(repoRoot, "paddock", "examples", "feature-sliced-frontend.yaml")
	output, exitCode := runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policy, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("graph exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	var document struct {
		Schema       string          `json:"schema"`
		Language     string          `json:"language"`
		PackageCount int             `json:"package_count"`
		EdgeCount    int             `json:"edge_count"`
		Packages     []model.Package `json:"packages"`
		Edges        []model.Edge    `json:"edges"`
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode graph JSON: %v\n%s", err, output)
	}
	if document.Schema != "paddock.graph/v1" || document.Language != "typescript" {
		t.Fatalf("unexpected graph identity: %#v", document)
	}
	if document.PackageCount == 0 || document.EdgeCount == 0 || len(document.Packages) != document.PackageCount || len(document.Edges) != document.EdgeCount {
		t.Fatalf("graph counts are incomplete: %#v", document)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"graph", source, "--language", "typescript",
	)
	if exitCode != 0 || !strings.Contains(output, "GRAPH typescript") || !strings.Contains(output, "EDGES") {
		t.Fatalf("text graph output is incomplete: exit=%d output:\n%s", exitCode, output)
	}

	pythonSource := filepath.Join(repoRoot, "paddock", "examples", "services", "python-hexagonal", "good")
	output, exitCode = runCLI(t, cli, repoRoot,
		"graph", pythonSource, "--language", "python", "--format", "json",
	)
	if exitCode != 0 || !strings.Contains(output, `"language": "python"`) || !strings.Contains(output, `"edge_count"`) {
		t.Fatalf("Python graph output is incomplete: exit=%d output:\n%s", exitCode, output)
	}
}

func TestCLIExternalGraphAdapter(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	directory := t.TempDir()
	graphPath := filepath.Join(directory, "graph.json")
	output, exitCode := runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policyPath, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("source graph exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if err := os.WriteFile(graphPath, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}

	adapterPath := filepath.Join(repoRoot, "paddock", "examples", "adapter", "fixture-adapter.sh")
	output, exitCode = runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policyPath,
		"--adapter", "sh", "--adapter-arg", adapterPath, "--adapter-arg", graphPath, "--format", "json",
	)
	if exitCode != 0 || !strings.Contains(output, `"schema": "paddock.graph/v1"`) || !strings.Contains(output, `"language": "go"`) {
		t.Fatalf("external graph adapter failed: exit=%d output:\n%s", exitCode, output)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policyPath,
		"--adapter", "sh", "--adapter-arg", adapterPath, "--adapter-arg", graphPath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("direct adapter check failed: exit=%d output:\n%s", exitCode, output)
	}

	manifestPath := filepath.Join(directory, "adapter-policy-tests.yaml")
	manifest := "schema: paddock.policy-tests/v1\ncases:\n" +
		"  - name: external-good-service\n    root: " + filepath.ToSlash(source) + "\n    expect: pass\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "test", "--policy", policyPath, "--cases", manifestPath,
		"--adapter", "sh", "--adapter-arg", adapterPath, "--adapter-arg", graphPath,
	)
	if exitCode != 0 || !strings.Contains(output, "POLICY-TEST PASS") {
		t.Fatalf("external adapter policy test failed: exit=%d output:\n%s", exitCode, output)
	}

	ciPath := filepath.Join(directory, "ci-result.json")
	generatedGraphPath := filepath.Join(directory, "generated-graph.json")
	output, exitCode = runCLI(t, cli, repoRoot,
		"ci", source, "--policy", policyPath,
		"--adapter", "sh", "--adapter-arg", adapterPath, "--adapter-arg", graphPath,
		"--graph-output", generatedGraphPath, "--output", ciPath,
	)
	if exitCode != 0 {
		t.Fatalf("direct adapter CI failed: exit=%d output:\n%s", exitCode, output)
	}
	ciArtifact, err := artifact.Load(ciPath)
	if err != nil {
		t.Fatal(err)
	}
	if ciArtifact.Graph == nil || ciArtifact.Graph.Path != generatedGraphPath || ciArtifact.Graph.SHA256 == "" {
		t.Fatalf("direct adapter CI omitted graph evidence: %#v", ciArtifact)
	}
}

func TestCLIGraphInputBoundary(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	directory := t.TempDir()
	graphPath := filepath.Join(directory, "paddock-graph.json")
	output, exitCode := runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policyPath, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("source graph exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if err := os.WriteFile(graphPath, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"graph", source, "--input", graphPath, "--format", "json",
	)
	if exitCode != 0 || !strings.Contains(output, `"schema": "paddock.graph/v1"`) {
		t.Fatalf("graph-input inspection failed: exit=%d output:\n%s", exitCode, output)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policyPath, "--graph", graphPath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("graph-input check failed: exit=%d output:\n%s", exitCode, output)
	}

	ciPath := filepath.Join(directory, "ci-result.json")
	output, exitCode = runCLI(t, cli, repoRoot,
		"ci", source, "--policy", policyPath, "--graph", graphPath, "--output", ciPath,
	)
	if exitCode != 0 {
		t.Fatalf("graph-input CI exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	ciArtifact, err := artifact.Load(ciPath)
	if err != nil {
		t.Fatal(err)
	}
	if ciArtifact.Graph == nil || ciArtifact.Graph.SHA256 == "" {
		t.Fatalf("CI artifact omitted graph evidence: %#v", ciArtifact)
	}

	foreignGraphPath := filepath.Join(directory, "foreign-graph.json")
	foreignPolicyPath := filepath.Join(directory, "foreign-policy.yaml")
	graphData, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	foreignGraph := strings.Replace(string(graphData), `"language": "go"`, `"language": "rust"`, 1)
	if err := os.WriteFile(foreignGraphPath, []byte(foreignGraph), 0o600); err != nil {
		t.Fatal(err)
	}
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	foreignPolicy := strings.Replace(string(policyData), "  language: go", "  language: rust", 1)
	if err := os.WriteFile(foreignPolicyPath, []byte(foreignPolicy), 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", foreignPolicyPath, "--graph", foreignGraphPath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("external-language graph check failed: exit=%d output:\n%s", exitCode, output)
	}
}

func TestCLIInitCreatesReviewableDraft(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "layered-go", "good")
	outputPath := filepath.Join(t.TempDir(), "paddock.yaml")
	output, exitCode := runCLI(t, cli, repoRoot,
		"init", source, "--template", "layered", "--output", outputPath,
	)
	if exitCode != 0 || !strings.Contains(output, "draft; review before CI") {
		t.Fatalf("init output is incomplete: exit=%d output:\n%s", exitCode, output)
	}
	loaded, err := policy.Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source.Language != "go" || len(loaded.Components) == 0 || len(loaded.Rules) == 0 {
		t.Fatalf("generated policy is incomplete: %#v", loaded)
	}
	for _, rule := range loaded.Rules {
		if rule.Severity != "warning" {
			t.Fatalf("generated rule severity = %q, want warning", rule.Severity)
		}
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"init", source, "--template", "layered", "--output", outputPath,
	)
	if exitCode != 2 || !strings.Contains(output, "already exists") {
		t.Fatalf("init overwrite protection failed: exit=%d output:\n%s", exitCode, output)
	}
}

func TestCLIInitWithExternalAdapter(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	directory := t.TempDir()
	graphPath := filepath.Join(directory, "rust-graph.json")
	outputPath := filepath.Join(directory, "paddock.yaml")
	graphOutput, exitCode := runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policyPath, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("graph exit code = %d, want 0; output:\n%s", exitCode, graphOutput)
	}
	graphOutput = strings.Replace(graphOutput, `"language": "go"`, `"language": "rust"`, 1)
	if err := os.WriteFile(graphPath, []byte(graphOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(repoRoot, "paddock", "examples", "adapter", "fixture-adapter.sh")
	output, exitCode := runCLI(t, cli, repoRoot,
		"init", source, "--language", "rust",
		"--adapter", "sh", "--adapter-arg", fixture, "--adapter-arg", graphPath,
		"--output", outputPath,
	)
	if exitCode != 0 || !strings.Contains(output, "draft; review before CI") {
		t.Fatalf("external init failed: exit=%d output:\n%s", exitCode, output)
	}
	loaded, err := policy.Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source.Language != "rust" || loaded.Source.Unit != "package" || len(loaded.Components) == 0 {
		t.Fatalf("external starter policy is incomplete: %#v", loaded)
	}
}

func TestCLIPolicyDiff(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	directory := t.TempDir()
	beforePath := filepath.Join(directory, "before.yaml")
	afterPath := filepath.Join(directory, "after.yaml")
	base := `schema: paddock.architecture/v1
project: policy-diff-test
source:
  language: go
  roots: [internal]
components:
  source:
    match: internal/**
rules:
  - id: no-cycles
    kind: no-cycles
`
	after := `schema: paddock.architecture/v1
project: policy-diff-test
source:
  language: go
  roots: [internal]
components:
  source:
    match: internal/**
rules:
  - id: no-cycles
    kind: no-cycles
    severity: warning
  - id: complete-classification
    kind: coverage
`
	if err := os.WriteFile(beforePath, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "policy-tests.yaml")
	goodRoot := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	manifest := "schema: paddock.policy-tests/v1\ncases:\n" +
		"  - name: good-service\n    root: " + filepath.ToSlash(goodRoot) + "\n    expect: pass\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	output, exitCode := runCLI(t, cli, repoRoot,
		"policy", "diff", "--before", beforePath, "--after", afterPath,
	)
	if exitCode != 0 || !strings.Contains(output, "POLICY-DIFF changed") || !strings.Contains(output, "rules.complete-classification") {
		t.Fatalf("text policy diff is incomplete: exit=%d output:\n%s", exitCode, output)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "diff", "--before", beforePath, "--after", afterPath, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("JSON policy diff exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	var document policydiff.Document
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode policy diff JSON: %v\n%s", err, output)
	}
	if document.Schema != "paddock.policy-diff/v1" || document.Summary.Total != 2 {
		t.Fatalf("unexpected policy diff document: %#v", document)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "diff", "--before", beforePath, "--after", afterPath, "--cases", manifestPath,
	)
	if exitCode != 0 || !strings.Contains(output, "tests: PASS") || !strings.Contains(output, "good-service") {
		t.Fatalf("policy diff with tests is incomplete: exit=%d output:\n%s", exitCode, output)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "diff", "--before", beforePath, "--after", afterPath, "--cases", manifestPath, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("JSON policy diff with tests exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode policy diff with tests JSON: %v\n%s", err, output)
	}
	if document.Tests == nil || document.Tests.Status != "PASS" || document.Tests.Passed != 1 {
		t.Fatalf("policy diff test results are incomplete: %#v", document.Tests)
	}

	badManifestPath := filepath.Join(directory, "failing-policy-tests.yaml")
	badManifest := strings.Replace(manifest, "expect: pass", "expect: fail", 1)
	if err := os.WriteFile(badManifestPath, []byte(badManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "diff", "--before", beforePath, "--after", afterPath, "--cases", badManifestPath,
	)
	if exitCode != 1 || !strings.Contains(output, "tests: FAIL") {
		t.Fatalf("mismatched policy diff tests returned exit=%d output:\n%s", exitCode, output)
	}

	reviewPath := filepath.Join(directory, "policy-review.json")
	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "review", "--before", beforePath, "--after", afterPath, "--cases", manifestPath, "--output", reviewPath,
	)
	if exitCode != 0 || !strings.Contains(output, "POLICY-REVIEW PASS") {
		t.Fatalf("policy review failed: exit=%d output:\n%s", exitCode, output)
	}
	reviewData, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	var review policyreview.Document
	if err := json.Unmarshal(reviewData, &review); err != nil {
		t.Fatalf("decode policy review: %v\n%s", err, reviewData)
	}
	if review.Schema != policyreview.Schema || review.Status != "PASS" || review.Diff.Tests == nil || review.Diff.Tests.Status != "PASS" {
		t.Fatalf("unexpected policy review: %#v", review)
	}
	output, exitCode = runCLI(t, cli, repoRoot, "policy", "review", "verify", "--input", reviewPath)
	if exitCode != 0 || !strings.Contains(output, "VERIFIED") {
		t.Fatalf("policy review verification failed: exit=%d output:\n%s", exitCode, output)
	}
	output, exitCode = runCLI(t, cli, repoRoot, "policy", "review", "verify", "--input", reviewPath, "--files")
	if exitCode != 0 || !strings.Contains(output, "VERIFIED") {
		t.Fatalf("policy review file verification failed: exit=%d output:\n%s", exitCode, output)
	}
	afterData, err := os.ReadFile(afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, append(afterData, []byte("\n# changed after review\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot, "policy", "review", "verify", "--input", reviewPath, "--files")
	if exitCode != 2 || !strings.Contains(output, "hash mismatch") {
		t.Fatalf("changed policy review inputs returned exit=%d output:\n%s", exitCode, output)
	}

	badReviewPath := filepath.Join(directory, "failing-policy-review.json")
	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "review", "--before", beforePath, "--after", afterPath, "--cases", badManifestPath, "--output", badReviewPath,
	)
	if exitCode != 1 || !strings.Contains(output, "POLICY-REVIEW FAIL") {
		t.Fatalf("mismatched policy review returned exit=%d output:\n%s", exitCode, output)
	}
}

func TestCLIPolicySealAndVerify(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	lockPath := filepath.Join(t.TempDir(), "paddock.lock.json")

	output, exitCode := runCLI(t, cli, repoRoot,
		"policy", "seal", "--input", policyPath, "--output", lockPath,
	)
	if exitCode != 0 || !strings.Contains(output, "SEALED") {
		t.Fatalf("policy seal failed: exit=%d output:\n%s", exitCode, output)
	}
	if _, err := policylock.Load(lockPath); err != nil {
		t.Fatalf("load policy lock: %v", err)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "verify", "--policy", policyPath, "--lock", lockPath,
	)
	if exitCode != 0 || !strings.Contains(output, "VERIFIED") {
		t.Fatalf("policy verify failed: exit=%d output:\n%s", exitCode, output)
	}
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	isolatedDirectory := t.TempDir()
	isolatedPolicyPath := filepath.Join(isolatedDirectory, "sealed.yaml")
	isolatedLockPath := filepath.Join(isolatedDirectory, "sealed.lock.json")
	if err := os.WriteFile(isolatedPolicyPath, policyData, 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"policy", "seal", "--input", isolatedPolicyPath, "--output", isolatedLockPath,
	)
	if exitCode != 0 {
		t.Fatalf("isolated policy seal failed: exit=%d output:\n%s", exitCode, output)
	}
	if err := os.Remove(isolatedPolicyPath); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy-lock", isolatedLockPath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("lock-only check without source policy failed: exit=%d output:\n%s", exitCode, output)
	}

	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policyPath, "--policy-lock", lockPath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("sealed policy check failed: exit=%d output:\n%s", exitCode, output)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy-lock", lockPath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("lock-only policy check failed: exit=%d output:\n%s", exitCode, output)
	}

	changedPolicyPath := filepath.Join(t.TempDir(), "changed.yaml")
	if err := os.WriteFile(changedPolicyPath, append(policyData, []byte("\n# formatting change after sealing\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", changedPolicyPath, "--policy-lock", lockPath,
	)
	if exitCode != 2 || !strings.Contains(output, "source hash") {
		t.Fatalf("changed sealed policy was accepted: exit=%d output:\n%s", exitCode, output)
	}

	ciPath := filepath.Join(t.TempDir(), "paddock-ci-result.json")
	output, exitCode = runCLI(t, cli, repoRoot,
		"ci", source, "--policy-lock", isolatedLockPath, "--output", ciPath,
	)
	if exitCode != 0 {
		t.Fatalf("sealed CI exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	ciArtifact, err := artifact.Load(ciPath)
	if err != nil {
		t.Fatal(err)
	}
	if ciArtifact.PolicyLock == nil || ciArtifact.PolicyLock.SHA256 == "" {
		t.Fatalf("CI artifact omitted policy lock evidence: %#v", ciArtifact)
	}
}

func TestPortableCIWorkflow(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	workflow := filepath.Join(repoRoot, "paddock", "examples", "ci", "paddock-gate.sh")
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	directory := t.TempDir()
	proposedPath := filepath.Join(directory, "proposed.yaml")
	lockPath := filepath.Join(directory, "paddock.lock.json")
	diffPath := filepath.Join(directory, "paddock-policy-diff.json")
	resultPath := filepath.Join(directory, "paddock-ci-result.json")
	graphPath := filepath.Join(directory, "paddock-graph.json")
	graphOutput, graphExitCode := runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policyPath, "--format", "json",
	)
	if graphExitCode != 0 {
		t.Fatalf("workflow graph generation exit code = %d, want 0; output:\n%s", graphExitCode, graphOutput)
	}
	if err := os.WriteFile(graphPath, []byte(graphOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proposedPath, append(policyData, []byte("\n# candidate policy formatting\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PADDOCK=" + cli,
		"PADDOCK_POLICY=" + policyPath,
		"PADDOCK_PROPOSED_POLICY=" + proposedPath,
		"PADDOCK_LOCK=" + lockPath,
		"PADDOCK_SOURCE_ROOT=" + source,
		"PADDOCK_DIFF=" + diffPath,
		"PADDOCK_RESULT=" + resultPath,
		"PADDOCK_GRAPH=" + graphPath,
	}

	output, exitCode := runWorkflow(t, workflow, repoRoot, env, "review")
	if exitCode != 0 {
		t.Fatalf("workflow review exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	diffData, err := os.ReadFile(diffPath)
	if err != nil || !strings.Contains(string(diffData), "paddock.policy-diff/v1") {
		t.Fatalf("workflow did not write policy diff: err=%v data=%s", err, diffData)
	}

	output, exitCode = runWorkflow(t, workflow, repoRoot, env, "seal")
	if exitCode != 0 || !strings.Contains(output, "SEALED") {
		t.Fatalf("workflow seal exit code = %d; output:\n%s", exitCode, output)
	}
	output, exitCode = runWorkflow(t, workflow, repoRoot, env, "verify")
	if exitCode != 0 || !strings.Contains(output, "VERIFIED") {
		t.Fatalf("workflow verify exit code = %d; output:\n%s", exitCode, output)
	}

	output, exitCode = runWorkflow(t, workflow, repoRoot, env, "gate")
	if exitCode != 0 {
		t.Fatalf("workflow gate exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	workflowArtifact, err := artifact.Load(resultPath)
	if err != nil {
		t.Fatalf("load workflow CI artifact: %v", err)
	}
	if workflowArtifact.Graph == nil || workflowArtifact.Graph.SHA256 == "" {
		t.Fatalf("workflow CI artifact omitted graph evidence: %#v", workflowArtifact)
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
	formattedPolicyPath := filepath.Join(t.TempDir(), "formatted.yaml")
	policyData, err := os.ReadFile(policy)
	if err != nil {
		t.Fatal(err)
	}
	policyData = append([]byte("\n# formatting-only policy change\n"), policyData...)
	if err := os.WriteFile(formattedPolicyPath, policyData, 0o600); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", formattedPolicyPath, "--baseline", baselinePath,
	)
	if exitCode != 0 {
		t.Fatalf("formatting-only policy change exit code = %d, want 0; output:\n%s", exitCode, output)
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
	if exitCode != 2 {
		t.Fatalf("changed-policy baseline check exit code = %d, want 2; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "baseline policy hash does not match") {
		t.Fatalf("policy drift error missing from output:\n%s", output)
	}

	stale := snapshot
	stale.Entries = append(stale.Entries, baseline.Entry{
		RuleID: "old-rule",
		Kind:   "deny-dependencies",
		From:   "internal/old",
	})
	stale.Entries[len(stale.Entries)-1].Fingerprint = baseline.Fingerprint(&model.Finding{
		RuleID: stale.Entries[len(stale.Entries)-1].RuleID,
		Kind:   stale.Entries[len(stale.Entries)-1].Kind,
		From:   stale.Entries[len(stale.Entries)-1].From,
	})
	if err := baseline.Save(baselinePath, stale); err != nil {
		t.Fatal(err)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policy, "--baseline", baselinePath,
	)
	if exitCode != 0 {
		t.Fatalf("stale baseline check exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "1 stale") {
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

func TestCLIBaselineWithExternalGraph(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	source := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "violating")
	policy := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	directory := t.TempDir()
	graphPath := filepath.Join(directory, "paddock-graph.json")
	baselinePath := filepath.Join(directory, "paddock-baseline.json")
	graphOutput, exitCode := runCLI(t, cli, repoRoot,
		"graph", source, "--policy", policy, "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("graph exit code = %d, want 0; output:\n%s", exitCode, graphOutput)
	}
	if err := os.WriteFile(graphPath, []byte(graphOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(repoRoot, "paddock", "examples", "adapter", "fixture-adapter.sh")
	output, exitCode := runCLI(t, cli, repoRoot,
		"baseline", source, "--policy", policy,
		"--adapter", "sh", "--adapter-arg", fixture, "--adapter-arg", graphPath,
		"--output", baselinePath,
	)
	if exitCode != 0 {
		t.Fatalf("external baseline exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	snapshot, err := baseline.Load(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) == 0 {
		t.Fatalf("external baseline contained no findings: %#v", snapshot)
	}
	output, exitCode = runCLI(t, cli, repoRoot,
		"check", source, "--policy", policy, "--graph", graphPath, "--baseline", baselinePath,
	)
	if exitCode != 0 || !strings.Contains(output, "PASS") {
		t.Fatalf("external baseline did not suppress its graph findings: exit=%d output:\n%s", exitCode, output)
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
	if document.Schema != "paddock.explanation/v1" || document.Status != "FAIL" || document.Triage.Outcome != "remediate" || len(document.Findings) != 2 || len(document.Summary) != 2 {
		t.Fatalf("unexpected explanation document: %#v", document)
	}
	output, exitCode = runCLI(t, cli, repoRoot, "explain", reportPath, "--rule", "cross-context-access-is-mediated", "--status", "blocking", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("filtered explanation exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode filtered explanation JSON: %v\n%s", err, output)
	}
	if document.Filter == nil || document.Filter.RuleID != "cross-context-access-is-mediated" || document.Filter.Status != "blocking" || document.Triage.Outcome != "remediate" || len(document.Findings) != 1 {
		t.Fatalf("filtered explanation is incomplete: %#v", document)
	}
}

func TestCLICIArtifact(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cli := buildCLI(t, repoRoot)
	policy := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	violating := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "violating")
	good := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")

	failedPath := filepath.Join(t.TempDir(), "failed.json")
	output, exitCode := runCLI(t, cli, repoRoot,
		"ci", violating, "--policy", policy, "--output", failedPath,
	)
	if exitCode != 1 {
		t.Fatalf("failed CI exit code = %d, want 1; output:\n%s", exitCode, output)
	}
	failedArtifact, err := artifact.Load(failedPath)
	if err != nil {
		t.Fatal(err)
	}
	if failedArtifact.Status != "failed" || failedArtifact.Report == nil || failedArtifact.Explanation == nil {
		t.Fatalf("failed CI artifact is incomplete: %#v", failedArtifact)
	}
	if failedArtifact.Policy.SHA256 == "" || !strings.Contains(output, "CI-RESULT") {
		t.Fatalf("CI artifact output is incomplete: %s", output)
	}

	output, exitCode = runCLI(t, cli, repoRoot, "explain", failedPath, "--format", "json")
	if exitCode != 0 {
		t.Fatalf("CI artifact explanation exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	var ciExplanation explain.Document
	if err := json.Unmarshal([]byte(output), &ciExplanation); err != nil {
		t.Fatalf("decode CI artifact explanation: %v\n%s", err, output)
	}
	if ciExplanation.Schema != "paddock.explanation/v1" || len(ciExplanation.Findings) == 0 || len(ciExplanation.Summary) == 0 {
		t.Fatalf("CI artifact explanation is incomplete: %#v", ciExplanation)
	}

	passedPath := filepath.Join(t.TempDir(), "passed.json")
	output, exitCode = runCLI(t, cli, repoRoot,
		"ci", good, "--policy", policy, "--output", passedPath,
	)
	if exitCode != 0 {
		t.Fatalf("passed CI exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	passedArtifact, err := artifact.Load(passedPath)
	if err != nil {
		t.Fatal(err)
	}
	if passedArtifact.Status != "passed" || passedArtifact.Report == nil || !passedArtifact.Report.OK() {
		t.Fatalf("passed CI artifact is incomplete: %#v", passedArtifact)
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

func runWorkflow(t *testing.T, workflow, repoRoot string, values []string, command string) (string, int) {
	t.Helper()
	env := toolchainEnv(t)
	env = append(env, values...)
	cmd := exec.Command("sh", workflow, command)
	cmd.Dir = repoRoot
	cmd.Env = env
	data, err := cmd.CombinedOutput()
	if err == nil {
		return string(data), 0
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		return string(data), exitError.ExitCode()
	}
	t.Fatalf("run workflow: %v\n%s", err, data)
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
