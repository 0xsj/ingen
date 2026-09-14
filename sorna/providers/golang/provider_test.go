package goprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/runner"
)

func TestBuildCopiesMutatesAndBuildsFreshGoVariant(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/provider-test\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "subject"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "subject", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "cmd"), filepath.Join(root, "node_modules", "linked-source")); err != nil {
		t.Skipf("symlink test unavailable: %v", err)
	}
	plan := testPlan()
	result, err := Build(Request{
		Plan:         plan,
		PlanSHA256:   strings.Repeat("e", 64),
		SourceRoot:   root,
		OutputDir:    ".generated",
		BinaryDir:    ".binaries",
		BuildPackage: "./cmd/subject",
		BinaryName:   "subject",
		ProviderID:   "test-go-provider",
		Capabilities: testCapabilities(),
		SubjectArgs:  []string{"-addr", "${SORA_ADDR}"},
		Mutate: func(variantRoot string, spec mutation.Spec) (campaign.ProviderProvenance, error) {
			if variantRoot == root || spec.ID != "m1" {
				return campaign.ProviderProvenance{}, os.ErrInvalid
			}
			return campaign.ProviderProvenance{Location: "cmd/subject/main.go:main", Before: "clean", After: "mutated"}, os.WriteFile(filepath.Join(variantRoot, "cmd", "subject", "mutation.marker"), []byte(spec.ID), 0o644)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider.PlanSHA256 != strings.Repeat("e", 64) || len(result.Provider.Capabilities) != 1 || len(result.Provider.Entries) != 1 || len(result.Variants) != 1 {
		t.Fatalf("result = %+v, want one provider capability, entry, and variant", result)
	}
	if !result.Provider.Supports(plan.Mutations[0].Spec) {
		t.Fatalf("provider capabilities = %+v, want test plan capability", result.Provider.Capabilities)
	}
	entry := result.Provider.Entries[0]
	if entry.Command != ".binaries/001-m1/subject" || len(entry.Args) != 2 || entry.Args[1] != "${SORA_ADDR}" {
		t.Fatalf("provider entry = %+v, want relative binary and runtime argv", entry)
	}
	variant := result.Variants[0]
	if variant.SourceDir == root || variant.BinaryPath == "" || len(variant.BinarySHA256) != 64 {
		t.Fatalf("variant = %+v, want isolated source and hashed binary", variant)
	}
	if _, err := os.Stat(variant.BinaryPath); err != nil {
		t.Fatalf("built binary = %s: %v", variant.BinaryPath, err)
	}
	if _, err := os.Stat(filepath.Join(variant.SourceDir, "cmd", "subject", "mutation.marker")); err != nil {
		t.Fatalf("mutation marker in copied source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "subject", "mutation.marker")); !os.IsNotExist(err) {
		t.Fatalf("source root mutation marker error = %v, want source untouched", err)
	}
	if entry.Provenance == nil || entry.Provenance.Location != "cmd/subject/main.go:main" || entry.Provenance.Before != "clean" || entry.Provenance.After != "mutated" {
		t.Fatalf("provider provenance = %+v, want edit provenance", entry.Provenance)
	}
	if entry.Provenance.SourceDir != ".generated/variants/001-m1/source" || entry.Provenance.SourceSHA256 == "" || entry.Provenance.BinarySHA256 != variant.BinarySHA256 {
		t.Fatalf("provider provenance identities = %+v, want generated paths and hashes", entry.Provenance)
	}
	if sourceHash, hashErr := campaign.HashTree(variant.SourceDir); hashErr != nil || sourceHash != entry.Provenance.SourceSHA256 {
		t.Fatalf("source provenance hash = %s (%v), want %s", sourceHash, hashErr, entry.Provenance.SourceSHA256)
	}

	providerPath := filepath.Join(root, ".generated", "provider.yaml")
	providerHash, err := WriteManifest(providerPath, result.Provider)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := campaign.LoadProviderFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "test-go-provider" || loaded.Entries[0].Command != entry.Command {
		t.Fatalf("loaded provider = %+v, want generated provider", loaded)
	}
	actualHash, err := campaign.HashFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if providerHash != actualHash {
		t.Fatalf("provider hash = %s, actual %s", providerHash, actualHash)
	}
}

func TestBuildRefusesToReuseProviderOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/provider-test\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "generated")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Build(Request{
		Plan:         testPlan(),
		PlanSHA256:   strings.Repeat("e", 64),
		SourceRoot:   root,
		OutputDir:    output,
		BinaryDir:    filepath.Join(root, "binaries"),
		BuildPackage: "./cmd/subject",
		Capabilities: testCapabilities(),
		Mutate: func(string, mutation.Spec) (campaign.ProviderProvenance, error) {
			return campaign.ProviderProvenance{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "output directory already exists") {
		t.Fatalf("Build() = %v, want no-reuse error", err)
	}
}

func TestBuildRejectsUndeclaredPlanCapability(t *testing.T) {
	_, err := Build(Request{
		Plan:         testPlan(),
		PlanSHA256:   strings.Repeat("e", 64),
		SourceRoot:   t.TempDir(),
		OutputDir:    ".generated",
		BinaryDir:    ".binaries",
		BuildPackage: "./cmd/subject",
		Capabilities: []campaign.ProviderCapability{{Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents"}},
		Mutate: func(string, mutation.Spec) (campaign.ProviderProvenance, error) {
			return campaign.ProviderProvenance{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not declare capability") {
		t.Fatalf("Build() = %v, want undeclared capability error", err)
	}
}

func TestBuildFailureDoesNotPublishPartialOutputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/provider-test\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "generated")
	binaries := filepath.Join(root, "binaries")
	_, err := Build(Request{
		Plan:         testPlan(),
		PlanSHA256:   strings.Repeat("e", 64),
		SourceRoot:   root,
		OutputDir:    output,
		BinaryDir:    binaries,
		BuildPackage: "./cmd/subject",
		Capabilities: testCapabilities(),
		Mutate: func(string, mutation.Spec) (campaign.ProviderProvenance, error) {
			return campaign.ProviderProvenance{}, os.ErrInvalid
		},
	})
	if err == nil {
		t.Fatal("Build() = nil, want mutator failure")
	}
	for name, path := range map[string]string{"output": output, "binaries": binaries} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("%s directory stat error = %v, want unpublished output", name, statErr)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".staging-") {
			t.Fatalf("staging directory %q remained after failure", entry.Name())
		}
	}
}

func testPlan() campaign.Plan {
	return campaign.Plan{
		Schema: campaign.Schema,
		Status: "ready",
		Catalogue: campaign.CatalogueReference{
			Path: "catalogue.yaml", ID: "catalogue", Version: 1, SHA256: strings.Repeat("a", 64),
		},
		Contract: runner.ContractReference{ID: "contract", Version: 1, SHA256: strings.Repeat("b", 64)},
		Oracle:   runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("c", 64)},
		Baseline: runner.BaselineReference{
			EvidencePath: "baseline",
			RunID:        "run-baseline",
			Contract:     runner.ContractReference{ID: "contract", Version: 1, SHA256: strings.Repeat("b", 64)},
			Oracle:       &runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("c", 64)},
		},
		OraclePolicySHA256: strings.Repeat("d", 64),
		Mutations: []campaign.MutationEntry{{
			Sequence: 1,
			Spec: mutation.Spec{
				ID: "m1", Plane: "implementation", Operator: "test.operator", Target: "GET /", Description: "test mutation",
				Change: map[string]any{"from": 1, "to": 2}, ExpectedRuleIDs: []string{"rule-1"}, Status: "candidate",
			},
		}},
	}
}

func testCapabilities() []campaign.ProviderCapability {
	return []campaign.ProviderCapability{{Plane: "implementation", Operator: "test.operator", Target: "GET /"}}
}
