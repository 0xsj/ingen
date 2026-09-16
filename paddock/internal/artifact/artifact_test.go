package artifact_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/paddock/internal/artifact"
	"ingen/paddock/internal/model"
)

func TestSaveLoadPreservesFailedResultAndExplanation(t *testing.T) {
	result := &model.Result{
		Schema:     "paddock.report/v1",
		Root:       "/service",
		ModulePath: "example.com/service",
		Findings: []*model.Finding{{
			RuleID:   "domain-is-pure",
			Kind:     "allow-dependencies",
			Severity: "error",
			From:     "internal/domain",
			To:       "net/http",
			Message:  "domain must remain pure",
		}},
	}
	createdAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	ciArtifact := artifact.New(result, artifact.FileRef{Path: "paddock.yaml", SHA256: strings.Repeat("a", 64)}, nil, createdAt)
	ciArtifact.Inputs = map[string]artifact.FileRef{
		artifact.AdapterProfileInput: {Path: "adapter-profile.yaml", SHA256: strings.Repeat("b", 64)},
	}
	if ciArtifact.Status != "failed" || ciArtifact.ExitCode != 1 {
		t.Fatalf("unexpected artifact verdict: %#v", ciArtifact)
	}
	path := filepath.Join(t.TempDir(), "ci-result.json")
	if err := artifact.Save(path, ciArtifact); err != nil {
		t.Fatal(err)
	}
	loaded, err := artifact.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schema != "ingen.ci-result/v1" || loaded.Report == nil || loaded.Explanation == nil || loaded.Inputs[artifact.AdapterProfileInput].SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("artifact lost required fields: %#v", loaded)
	}
	if loaded.Explanation.Status != "FAIL" || loaded.Explanation.Triage.Outcome != "remediate" || len(loaded.Explanation.Findings) != 1 || len(loaded.Explanation.Summary) != 1 {
		t.Fatalf("artifact explanation is incomplete: %#v", loaded.Explanation)
	}
}

func TestErrorArtifactIsValid(t *testing.T) {
	ciArtifact := artifact.NewError(".", artifact.FileRef{Path: "missing.yaml"}, nil, errors.New("policy not found"), time.Now())
	if ciArtifact.Status != "error" || ciArtifact.ExitCode != 2 {
		t.Fatalf("unexpected error artifact: %#v", ciArtifact)
	}
	if err := ciArtifact.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateUsesSharedCIEnvelopeRules(t *testing.T) {
	result := &model.Result{
		Schema:     "paddock.report/v1",
		Root:       "/service",
		ModulePath: "example.com/service",
	}
	ciArtifact := artifact.New(result, artifact.FileRef{Path: "paddock.yaml"}, nil, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	ciArtifact.CreatedAt = "not-a-timestamp"
	if err := ciArtifact.Validate(); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("invalid shared CI timestamp was accepted: %v", err)
	}
}
