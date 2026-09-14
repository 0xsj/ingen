package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/sorna/internal/runner"
)

func TestAttachCampaignProvenancePreservesExactInputs(t *testing.T) {
	root := t.TempDir()
	evidenceDir := filepath.Join(root, "evidence")
	planPath := filepath.Join(root, "plan.json")
	providerPath := filepath.Join(root, "provider.yaml")
	planBytes := []byte("{\"schema\":\"ingen.mutation-plan/v1\"}\n")
	providerBytes := []byte("mutation_provider:\n  schema: ingen.mutation-provider/v1\n")
	if err := os.WriteFile(planPath, planBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerPath, providerBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-campaign-provenance-test",
		CreatedAt: time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC),
		Contract:  runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("a", 64)},
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1", Variant: "status-200-create"},
		Verdict:   runner.ContractVerdict{Status: "fail", Reason: "mutation was detected"},
	}
	if _, err := WriteBundle(evidenceDir, record, nil); err != nil {
		t.Fatal(err)
	}
	if err := AttachCampaignProvenance(evidenceDir, CampaignProvenanceInput{
		PlanPath:     planPath,
		ProviderPath: providerPath,
		Sequence:     1,
		MutationID:   "status-200-create",
	}); err != nil {
		t.Fatal(err)
	}

	for relative, want := range map[string][]byte{
		"campaign/plan.json":     planBytes,
		"campaign/provider.yaml": providerBytes,
	} {
		got, err := os.ReadFile(filepath.Join(evidenceDir, relative))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s = %q, want exact source bytes %q", relative, got, want)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(evidenceDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Campaign == nil || manifest.Campaign.MutationID != "status-200-create" || manifest.Campaign.Sequence != 1 {
		t.Fatalf("campaign provenance = %+v, want mutation identity", manifest.Campaign)
	}
	if manifest.Campaign.Plan.SourcePath != planPath || manifest.Campaign.Plan.SHA256 != hashBytes(planBytes) {
		t.Fatalf("campaign plan reference = %+v, want source and exact hash", manifest.Campaign.Plan)
	}
	if manifest.Campaign.Provider.SourcePath != providerPath || manifest.Campaign.Provider.SHA256 != hashBytes(providerBytes) {
		t.Fatalf("campaign provider reference = %+v, want source and exact hash", manifest.Campaign.Provider)
	}
	for _, relative := range []string{"campaign/plan.json", "campaign/provider.yaml", "campaign/provenance.json"} {
		if _, ok := manifest.ArtifactsSHA256[relative]; !ok {
			t.Fatalf("manifest artifacts = %+v, want %q", manifest.ArtifactsSHA256, relative)
		}
	}
	if err := Verify(evidenceDir); err != nil {
		t.Fatalf("Verify() = %v, want valid campaign evidence", err)
	}
}

func TestVerifyRejectsTamperedCampaignInput(t *testing.T) {
	root := t.TempDir()
	evidenceDir := filepath.Join(root, "evidence")
	planPath := filepath.Join(root, "plan.json")
	providerPath := filepath.Join(root, "provider.yaml")
	if err := os.WriteFile(planPath, []byte("plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerPath, []byte("provider\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-campaign-tamper-test",
		CreatedAt: time.Now().UTC(),
		Contract:  runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("b", 64)},
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
	}
	if _, err := WriteBundle(evidenceDir, record, nil); err != nil {
		t.Fatal(err)
	}
	if err := AttachCampaignProvenance(evidenceDir, CampaignProvenanceInput{PlanPath: planPath, ProviderPath: providerPath, Sequence: 2, MutationID: "mut-2"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "campaign/provider.yaml"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(evidenceDir); err == nil || !strings.Contains(err.Error(), "checksum mismatch for campaign/provider.yaml") {
		t.Fatalf("Verify() after campaign input tamper = %v, want checksum mismatch", err)
	}
}
