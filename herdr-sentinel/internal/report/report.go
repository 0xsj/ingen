// Package report renders Sentinel's operator-facing lifecycle view. It keeps
// receipt state, audit integrity, and producer-owned verification meaning
// visibly separate.
package report

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	sentinelaudit "ingen/herdr-sentinel/internal/audit"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

type Document struct {
	Receipt sentinelrun.Receipt
	Audit   sentinelaudit.Report
}

func Build(receiptPath, root string) (Document, error) {
	receipt, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		return Document{}, fmt.Errorf("build Sentinel report: %w", err)
	}
	audit, err := sentinelaudit.BuildReceipt(receipt, root)
	if err != nil {
		return Document{}, fmt.Errorf("build Sentinel report: %w", err)
	}
	return Document{Receipt: receipt, Audit: audit}, nil
}

// Write renders facts owned by Sentinel and labels the boundaries around them.
// In particular, a completed lifecycle receipt is not called a passed run:
// only Sorna can interpret its verification result.
func Write(w io.Writer, document Document) error {
	output, err := render(document)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, output)
	return err
}

// SaveFile renders and publishes a complete operator report. The temporary
// file keeps readers from observing a partially written report.
func SaveFile(path string, document Document) error {
	output, err := render(document)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".sentinel-report-*")
	if err != nil {
		return fmt.Errorf("create temporary Sentinel report in %s: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(output); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Sentinel report: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Sentinel report permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Sentinel report: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Sentinel report: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Sentinel report %s: %w", path, err)
	}
	return nil
}

func render(document Document) (string, error) {
	if err := document.Receipt.Validate(); err != nil {
		return "", fmt.Errorf("write Sentinel report: %w", err)
	}
	if err := document.Audit.Validate(); err != nil {
		return "", fmt.Errorf("write Sentinel report: %w", err)
	}
	var output strings.Builder
	fmt.Fprintf(&output, "Run: %s\n", document.Receipt.RunID)
	fmt.Fprintf(&output, "Workspace: %s v%d\n", document.Receipt.Workspace.ID, document.Receipt.Workspace.Version)
	fmt.Fprintf(&output, "Lifecycle receipt: %s\n", document.Receipt.Status)
	fmt.Fprintf(&output, "Audit integrity: %s\n", document.Audit.Status)
	fmt.Fprintln(&output, "Sorna verdict: producer-owned; not interpreted by Sentinel")

	fmt.Fprintln(&output, "\nEvents:")
	for _, event := range document.Receipt.Events {
		fmt.Fprintf(&output, "  %03d %s %s", event.Sequence, event.At, event.Type)
		if event.Role != "" {
			fmt.Fprintf(&output, " role=%s", event.Role)
		}
		if event.Workspace != "" {
			fmt.Fprintf(&output, " workspace=%s", event.Workspace)
		}
		if event.SessionID != "" {
			fmt.Fprintf(&output, " session=%s", event.SessionID)
		}
		if event.Status != "" {
			fmt.Fprintf(&output, " receipt_status=%s", event.Status)
		}
		if event.SourceID != "" {
			fmt.Fprintf(&output, " source=%s", event.SourceID)
		}
		if event.Outcome != "" {
			fmt.Fprintf(&output, " outcome=%s", event.Outcome)
		}
		if event.Reason != "" {
			fmt.Fprintf(&output, " reason=%s", event.Reason)
		}
		output.WriteByte('\n')
	}

	fmt.Fprintln(&output, "Artifacts:")
	if len(document.Receipt.Artifacts) == 0 {
		fmt.Fprintln(&output, "  none")
	} else {
		for _, artifact := range document.Receipt.Artifacts {
			fmt.Fprintf(&output, "  %s kind=%s role=%s path=%s sha256=%s\n", artifact.ID, artifact.Kind, artifact.Role, artifact.Ref.Path, artifact.Ref.SHA256)
		}
	}

	fmt.Fprintln(&output, "\nAudit checks:")
	for _, check := range document.Audit.Checks {
		fmt.Fprintf(&output, "  %s: %s (%s)\n", check.ID, check.Status, check.Detail)
	}
	fmt.Fprintln(&output, "\nLimitations:")
	for _, limitation := range document.Audit.Limitations {
		fmt.Fprintf(&output, "  - %s\n", limitation)
	}
	return output.String(), nil
}
