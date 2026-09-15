package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/campaign"
)

func TestBuildMutationPreparationCIResultBindsSummaryProviderAndPlan(t *testing.T) {
	summaryPath, providerPath := writePreparationFixtures(t)
	artifact, err := BuildMutationPreparationCIResult(summaryPath, providerPath, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Tool != "sorna" || artifact.Kind != "mutation-preparation" || artifact.Status != "passed" || artifact.ExitCode != 0 {
		t.Fatalf("artifact = %+v, want passing preparation envelope", artifact)
	}
	if artifact.Inputs["preparation"].Path != summaryPath || artifact.Inputs["provider"].Path != providerPath || artifact.Inputs["plan"].Path == "" {
		t.Fatalf("artifact inputs = %+v, want preparation, provider, and plan references", artifact.Inputs)
	}
	var report campaign.PreparationSummary
	if err := json.Unmarshal(artifact.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != campaign.PreparationSummarySchema || len(report.Variants) != 1 || report.Variants[0].ChangedFiles[0] != "main.go" {
		t.Fatalf("report = %+v, want preserved preparation summary", report)
	}
	var explanation mutationPreparationExplanationPayload
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.Schema != mutationPreparationExplanationSchema || explanation.VariantCount != 1 || explanation.ChangedFileCount != 1 {
		t.Fatalf("explanation = %+v, want preparation counts", explanation)
	}
}

func TestBuildMutationPreparationCIErrorResultIsCollectorFriendly(t *testing.T) {
	summaryPath, providerPath := writePreparationFixtures(t)
	artifact, err := BuildMutationPreparationCIErrorResult(summaryPath, providerPath, ".", os.ErrInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Kind != "mutation-preparation" || artifact.Status != "error" || artifact.ExitCode != 2 || artifact.Error == "" {
		t.Fatalf("artifact = %+v, want preparation error envelope", artifact)
	}
	if artifact.Inputs["preparation"].Path != summaryPath || artifact.Inputs["provider"].Path != providerPath {
		t.Fatalf("artifact inputs = %+v, want available preparation inputs", artifact.Inputs)
	}
}

func TestBuildMutationPreparationCIResultRejectsProviderPlanMismatch(t *testing.T) {
	summaryPath, providerPath := writePreparationFixtures(t)
	contents, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Provider campaign.ProviderManifest `json:"mutation_provider"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	document.Provider.PlanSHA256 = strings.Repeat("c", 64)
	contents, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildMutationPreparationCIResult(summaryPath, providerPath, "."); err == nil || !strings.Contains(err.Error(), "plan hash") {
		t.Fatalf("BuildMutationPreparationCIResult() = %v, want provider plan mismatch", err)
	}
}

func writePreparationFixtures(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	planPath := filepath.Join(directory, "plan.json")
	planContents := []byte("plan\n")
	if err := os.WriteFile(planPath, planContents, 0o644); err != nil {
		t.Fatal(err)
	}
	planHash := campaign.HashBytes(planContents)
	provenance := &campaign.ProviderProvenance{
		SourceDir:    "generated/source",
		SourceSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64),
		Location:     "main.go:main",
		Before:       "old",
		After:        "new",
		TargetResolution: &campaign.TargetResolution{
			Selector:       "main.go:main",
			CandidateCount: 1,
			AppliedCount:   1,
		},
	}
	provider := campaign.ProviderManifest{
		Schema:     campaign.ProviderSchema,
		ID:         "source-provider",
		Version:    1,
		PlanSchema: campaign.Schema,
		PlanSHA256: planHash,
		Capabilities: []campaign.ProviderCapability{{
			Plane: "implementation", Operator: "test.operator", Target: "GET /",
		}},
		Entries: []campaign.ProviderEntry{{
			MutationID: "m1", Command: "bin/subject", SubjectRoot: ".", Variant: "m1", Provenance: provenance,
		}},
	}
	summary := campaign.PreparationSummary{
		Schema:     campaign.PreparationSummarySchema,
		ProviderID: provider.ID,
		PlanPath:   planPath,
		PlanSHA256: planHash,
		Variants: []campaign.PreparationVariant{{
			Sequence: 1, MutationID: "m1", SourceDir: "generated/source", SourceSHA256: provenance.SourceSHA256,
			BinaryPath: "bin/subject", BinarySHA256: provenance.BinarySHA256, ChangedFiles: []string{"main.go"}, Provenance: provenance,
		}},
	}
	summaryPath := filepath.Join(directory, "preparation.json")
	if _, err := campaign.WritePreparationSummary(summaryPath, summary); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(directory, "provider.json")
	document := struct {
		Provider campaign.ProviderManifest `json:"mutation_provider"`
	}{Provider: provider}
	contents, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	return summaryPath, providerPath
}
