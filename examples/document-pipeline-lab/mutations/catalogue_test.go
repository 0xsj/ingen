package mutations_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestRepositoryCatalogueIsBoundToItsContract(t *testing.T) {
	// go test runs this package with its directory as the working directory.
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	cataloguePath := filepath.Join("examples", "document-pipeline-lab", "mutations", "catalogue.yaml")
	contractPath := filepath.Join("examples", "document-pipeline-lab", "contract", "contract.yaml")

	catalogueBytes, err := os.ReadFile(filepath.Join(repoRoot, cataloguePath))
	if err != nil {
		t.Fatal(err)
	}
	var catalogue struct {
		MutationCatalogue struct {
			Mutations []struct {
				ID string `yaml:"id"`
			} `yaml:"mutations"`
		} `yaml:"mutation_catalogue"`
	}
	if err := yaml.Unmarshal(catalogueBytes, &catalogue); err != nil {
		t.Fatalf("decode repository catalogue: %v", err)
	}
	wantIDs := []string{
		"status-200-create",
		"remove-name-create",
		"unsupported-type-500",
		"process-stays-queued",
		"persistence-under-wrong-key",
		"accepts-png",
	}
	if len(catalogue.MutationCatalogue.Mutations) != len(wantIDs) {
		t.Fatalf("repository catalogue has %d mutations, want exactly %d", len(catalogue.MutationCatalogue.Mutations), len(wantIDs))
	}
	gotIDs := make(map[string]bool, len(catalogue.MutationCatalogue.Mutations))
	for _, mutation := range catalogue.MutationCatalogue.Mutations {
		gotIDs[mutation.ID] = true
	}
	for _, id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("repository catalogue is missing mutation %q", id)
		}
	}
	for id := range gotIDs {
		found := false
		for _, wantID := range wantIDs {
			if id == wantID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("repository catalogue has unexpected mutation %q", id)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "./sorna/cmd/sorna", "mutation", "validate", cataloguePath, "--contract", contractPath)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Sorna rejected repository catalogue binding: %v\n%s", err, output)
	}
	if ctx.Err() != nil {
		t.Fatalf("Sorna catalogue validation exceeded its timeout: %v\n%s", ctx.Err(), output)
	}
}
