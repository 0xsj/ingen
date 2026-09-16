// Package audit checks the integrity and completeness of a Sentinel receipt.
// It does not reinterpret Sorna results or promote Herdr self-report to
// independent evidence.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

const Schema = "ingen.sentinel-audit/v1"

type Report struct {
	Schema        string   `json:"schema"`
	RunID         string   `json:"run_id"`
	WorkspaceID   string   `json:"workspace_id"`
	ReceiptStatus string   `json:"receipt_status"`
	Status        string   `json:"status"`
	CreatedAt     string   `json:"created_at"`
	Checks        []Check  `json:"checks"`
	Limitations   []string `json:"limitations"`
}

type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Build loads a receipt and checks every file reference against bytes under
// root. A valid but non-terminal receipt is reported as an error because an
// audit cannot claim completeness before the lifecycle has ended.
func Build(receiptPath, root string) (Report, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return Report{}, fmt.Errorf("build Sentinel audit: receipt path must not be empty")
	}
	receipt, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		return Report{}, fmt.Errorf("build Sentinel audit: %w", err)
	}
	return BuildReceipt(receipt, root)
}

// BuildReceipt audits an already validated receipt snapshot. Callers that
// also need the receipt bytes can therefore audit and emit from one read.
func BuildReceipt(receipt sentinelrun.Receipt, root string) (Report, error) {
	if err := receipt.Validate(); err != nil {
		return Report{}, fmt.Errorf("build Sentinel audit from receipt: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	report := Report{
		Schema:        Schema,
		RunID:         receipt.RunID,
		WorkspaceID:   receipt.Workspace.ID,
		ReceiptStatus: receipt.Status,
		Status:        "passed",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Checks: []Check{{
			ID:     "receipt-structure",
			Status: "passed",
			Detail: "validated ingen.sentinel-run/v1 receipt",
		}},
		Limitations: []string{
			"file hashes verify bytes at audit time; they do not attest to event origin or absence of unobserved access",
			"Herdr callback delivery is not independently attested by Sentinel",
			"Sorna result meaning and enforcement evidence remain producer-owned",
		},
	}

	workspaceCheck := verifyFile(root, receipt.Workspace.File)
	workspaceCheck.ID = "workspace-integrity"
	report.Checks = append(report.Checks, workspaceCheck)
	if workspaceCheck.Status == "failed" {
		report.Status = "failed"
	}
	for _, artifact := range receipt.Artifacts {
		check := verifyFile(root, artifact.Ref)
		check.ID = "artifact-" + artifact.ID
		report.Checks = append(report.Checks, check)
		if check.Status == "failed" {
			report.Status = "failed"
		}
	}

	if terminal(receipt.Status) {
		report.Checks = append(report.Checks, Check{
			ID:     "lifecycle-terminal",
			Status: "passed",
			Detail: fmt.Sprintf("receipt status %q is terminal", receipt.Status),
		})
	} else {
		report.Checks = append(report.Checks, Check{
			ID:     "lifecycle-terminal",
			Status: "unavailable",
			Detail: fmt.Sprintf("receipt status %q is not terminal", receipt.Status),
		})
		if report.Status == "passed" {
			report.Status = "error"
		}
	}
	if err := report.Validate(); err != nil {
		return Report{}, fmt.Errorf("validate Sentinel audit: %w", err)
	}
	return report, nil
}

func verifyFile(root string, ref ciresult.FileRef) Check {
	path := filepath.Join(root, filepath.Clean(ref.Path))
	contents, err := os.ReadFile(path)
	if err != nil {
		return Check{Status: "failed", Detail: fmt.Sprintf("read %s: %v", ref.Path, err)}
	}
	digest := sha256.Sum256(contents)
	actual := hex.EncodeToString(digest[:])
	if !strings.EqualFold(actual, ref.SHA256) {
		return Check{Status: "failed", Detail: fmt.Sprintf("sha256 mismatch for %s: expected %s, got %s", ref.Path, ref.SHA256, actual)}
	}
	return Check{Status: "passed", Detail: fmt.Sprintf("sha256 verified for %s", ref.Path)}
}

func terminal(status string) bool {
	switch status {
	case "completed", "failed", "blocked", "cleaned":
		return true
	default:
		return false
	}
}

func (r Report) Validate() error {
	if r.Schema != Schema {
		return fmt.Errorf("Sentinel audit schema must be %s, got %q", Schema, r.Schema)
	}
	if strings.TrimSpace(r.RunID) == "" || strings.TrimSpace(r.WorkspaceID) == "" {
		return fmt.Errorf("Sentinel audit run_id and workspace_id are required")
	}
	if strings.TrimSpace(r.ReceiptStatus) == "" {
		return fmt.Errorf("Sentinel audit receipt_status is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
		return fmt.Errorf("Sentinel audit created_at must be RFC3339: %w", err)
	}
	if r.Status != "passed" && r.Status != "failed" && r.Status != "error" {
		return fmt.Errorf("Sentinel audit status %q is unsupported", r.Status)
	}
	if len(r.Checks) == 0 {
		return fmt.Errorf("Sentinel audit needs at least one check")
	}
	seen := make(map[string]bool, len(r.Checks))
	for _, check := range r.Checks {
		if strings.TrimSpace(check.ID) == "" || strings.TrimSpace(check.Detail) == "" {
			return fmt.Errorf("Sentinel audit checks need an ID and detail")
		}
		if seen[check.ID] {
			return fmt.Errorf("Sentinel audit check ID %q was duplicated", check.ID)
		}
		seen[check.ID] = true
		if check.Status != "passed" && check.Status != "failed" && check.Status != "unavailable" {
			return fmt.Errorf("Sentinel audit check %q has unsupported status %q", check.ID, check.Status)
		}
	}
	return nil
}

func (r Report) ExitCode() int {
	code, err := ciresult.ExitCodeForStatus(r.Status)
	if err != nil {
		return 2
	}
	return code
}

func WriteJSON(w io.Writer, report Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func SaveFile(path string, report Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Sentinel audit: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".sentinel-audit-*")
	if err != nil {
		return fmt.Errorf("create temporary Sentinel audit in %s: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Sentinel audit: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Sentinel audit permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Sentinel audit: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Sentinel audit: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Sentinel audit %s: %w", path, err)
	}
	return nil
}
