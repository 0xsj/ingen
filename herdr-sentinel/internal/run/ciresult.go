package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
)

// BuildCIResultFile adapts one validated Sentinel receipt to the shared CI
// envelope. The receipt remains the producer-owned report; the envelope only
// exposes lifecycle status and exact input references for coordinators.
func BuildCIResultFile(path, sourceRoot string) (ciresult.Artifact, error) {
	if strings.TrimSpace(path) == "" {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: receipt path must not be empty")
	}
	path = filepath.Clean(path)
	contents, err := os.ReadFile(path)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: read receipt %s: %w", path, err)
	}
	receipt, err := loadBytes(path, contents)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: %w", err)
	}
	digest := sha256.Sum256(contents)
	return buildCIResult(receipt, ciresult.FileRef{
		Path:   path,
		SHA256: hex.EncodeToString(digest[:]),
	}, contents, sourceRoot)
}

func buildCIResult(receipt Receipt, receiptRef ciresult.FileRef, report []byte, sourceRoot string) (ciresult.Artifact, error) {
	status, outcome, lifecycleError := ciStatus(receipt.Status)
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	explanation, err := json.Marshal(struct {
		Schema        string   `json:"schema"`
		ReceiptStatus string   `json:"receipt_status"`
		Outcome       string   `json:"outcome"`
		ArtifactIDs   []string `json:"artifact_ids"`
	}{
		Schema:        "ingen.sentinel-ci-explanation/v1",
		ReceiptStatus: receipt.Status,
		Outcome:       outcome,
		ArtifactIDs:   receiptArtifactIDs(receipt),
	})
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode Sentinel CI explanation: %w", err)
	}
	inputs := map[string]ciresult.FileRef{
		"receipt":   receiptRef,
		"workspace": receipt.Workspace.File,
	}
	for _, artifact := range receipt.Artifacts {
		inputs["artifact_"+artifact.ID] = artifact.Ref
	}
	result := ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        "sentinel",
		Kind:        "orchestration-receipt",
		Status:      status,
		ExitCode:    exitCodeForStatus(status),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Source:      ciresult.Source{Root: sourceRoot, ModulePath: receipt.Workspace.ID},
		Inputs:      inputs,
		Report:      append([]byte(nil), report...),
		Explanation: explanation,
	}
	if lifecycleError != "" {
		result.Error = lifecycleError
	}
	if err := result.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate Sentinel CI result: %w", err)
	}
	return result, nil
}

func ciStatus(receiptStatus string) (status, outcome, lifecycleError string) {
	switch receiptStatus {
	case "completed":
		return "passed", "Sentinel lifecycle completed", ""
	case "failed":
		return "failed", "Sentinel lifecycle failed", ""
	case "blocked", "created", "running", "cleaned":
		return "error", fmt.Sprintf("Sentinel lifecycle status %q is not a terminal verifier outcome", receiptStatus), fmt.Sprintf("Sentinel receipt is not complete: status %q", receiptStatus)
	default:
		return "error", "Sentinel lifecycle status is unsupported", fmt.Sprintf("unsupported Sentinel receipt status %q", receiptStatus)
	}
}

func exitCodeForStatus(status string) int {
	code, err := ciresult.ExitCodeForStatus(status)
	if err != nil {
		return 2
	}
	return code
}

func receiptArtifactIDs(receipt Receipt) []string {
	ids := make([]string, 0, len(receipt.Artifacts))
	for _, artifact := range receipt.Artifacts {
		ids = append(ids, artifact.ID)
	}
	return ids
}
