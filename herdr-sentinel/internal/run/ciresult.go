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

const CIExplanationSchema = "ingen.sentinel-ci-explanation/v1"

// AuditSummary carries only the integrity decision needed to explain why a
// Sentinel CI envelope was emitted. It is not a second copy of the audit
// report and does not interpret producer-owned Sorna results.
type AuditSummary struct {
	Status string
	Checks []AuditCheck
}

type AuditCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (a AuditSummary) validate() error {
	if a.Status == "" {
		if len(a.Checks) != 0 {
			return fmt.Errorf("Sentinel audit summary checks require a status")
		}
		return nil
	}
	if a.Status != "passed" && a.Status != "failed" && a.Status != "error" {
		return fmt.Errorf("Sentinel audit summary status %q is unsupported", a.Status)
	}
	seen := make(map[string]bool, len(a.Checks))
	for _, check := range a.Checks {
		if strings.TrimSpace(check.ID) == "" || strings.TrimSpace(check.Status) == "" {
			return fmt.Errorf("Sentinel audit summary checks need an ID and status")
		}
		if seen[check.ID] {
			return fmt.Errorf("Sentinel audit summary check ID %q was duplicated", check.ID)
		}
		seen[check.ID] = true
		if check.Status != "passed" && check.Status != "failed" && check.Status != "unavailable" {
			return fmt.Errorf("Sentinel audit summary check %q has unsupported status %q", check.ID, check.Status)
		}
	}
	return nil
}

// BuildCIResultFile adapts one validated Sentinel receipt to the shared CI
// envelope. The receipt remains the producer-owned report; the envelope only
// exposes lifecycle status and exact input references for coordinators.
func BuildCIResultFile(path, sourceRoot string) (ciresult.Artifact, error) {
	return buildCIResultFile(path, sourceRoot, AuditSummary{})
}

// BuildCIResultFileWithAudit adapts a receipt and the audit decision that was
// made for those exact receipt references. The compact summary makes the
// integrity gate visible in the shared envelope explanation.
func BuildCIResultFileWithAudit(path, sourceRoot string, audit AuditSummary) (ciresult.Artifact, error) {
	return buildCIResultFile(path, sourceRoot, audit)
}

func buildCIResultFile(path, sourceRoot string, audit AuditSummary) (ciresult.Artifact, error) {
	if strings.TrimSpace(path) == "" {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: receipt path must not be empty")
	}
	if err := audit.validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: %w", err)
	}
	path = filepath.Clean(path)
	contents, err := os.ReadFile(path)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: read receipt %s: %w", path, err)
	}
	return BuildCIResultBytes(path, contents, sourceRoot, audit)
}

// BuildCIResultBytes emits an envelope from an already-read receipt snapshot.
// The path is retained as the provenance label, while contents remain the
// exact bytes represented by the receipt report and input hash.
func BuildCIResultBytes(path string, contents []byte, sourceRoot string, audit AuditSummary) (ciresult.Artifact, error) {
	if strings.TrimSpace(path) == "" {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: receipt path must not be empty")
	}
	if err := audit.validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: %w", err)
	}
	receipt, err := loadBytes(path, contents)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel CI result: %w", err)
	}
	digest := sha256.Sum256(contents)
	return buildCIResult(receipt, ciresult.FileRef{
		Path:   path,
		SHA256: hex.EncodeToString(digest[:]),
	}, contents, sourceRoot, audit)
}

func buildCIResult(receipt Receipt, receiptRef ciresult.FileRef, report []byte, sourceRoot string, audit AuditSummary) (ciresult.Artifact, error) {
	status, outcome, lifecycleError := ciStatus(receipt.Status)
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	explanationPayload := struct {
		Schema        string       `json:"schema"`
		ReceiptStatus string       `json:"receipt_status"`
		Outcome       string       `json:"outcome"`
		ArtifactIDs   []string     `json:"artifact_ids"`
		AuditStatus   string       `json:"audit_status,omitempty"`
		AuditChecks   []AuditCheck `json:"audit_checks,omitempty"`
	}{
		Schema:        CIExplanationSchema,
		ReceiptStatus: receipt.Status,
		Outcome:       outcome,
		ArtifactIDs:   receiptArtifactIDs(receipt),
		AuditStatus:   audit.Status,
	}
	if audit.Status != "" {
		explanationPayload.AuditChecks = append([]AuditCheck(nil), audit.Checks...)
	}
	explanation, err := json.Marshal(explanationPayload)
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
