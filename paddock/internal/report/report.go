package report

import (
	"encoding/json"
	"fmt"
	"io"

	"ingen/paddock/internal/model"
)

func Text(w io.Writer, result *model.Result) error {
	status := "PASS"
	if !result.OK() {
		status = "FAIL"
	}
	summary := ""
	if result.Baseline != nil {
		summary = fmt.Sprintf(", baseline %d/%d matched", result.Baseline.Matched, result.Baseline.Entries)
		if len(result.Baseline.Stale) > 0 {
			summary += fmt.Sprintf(", %d stale", len(result.Baseline.Stale))
		}
	}
	if _, err := fmt.Fprintf(w, "%s %s (%d packages, %d edges%s)\n", status, result.Root, result.PackageCount, result.EdgeCount, summary); err != nil {
		return err
	}
	for _, finding := range result.Findings {
		location := finding.From
		if finding.File != "" {
			location = fmt.Sprintf("%s:%d", finding.File, finding.Line)
		}
		if finding.To != "" {
			location += " -> " + finding.To
		}
		annotation := ""
		switch finding.WaiverStatus {
		case "applied":
			annotation = fmt.Sprintf(" (waived by %s until %s: %s)", finding.WaiverOwner, finding.WaiverExpires, finding.WaiverReason)
		case "expired":
			annotation = fmt.Sprintf(" (waiver expired on %s; owner %s: %s)", finding.WaiverExpires, finding.WaiverOwner, finding.WaiverReason)
		}
		if finding.Baselined {
			annotation += fmt.Sprintf(" (baseline %s)", finding.BaselineFingerprint)
		}
		if _, err := fmt.Fprintf(w, "  [%s] %s: %s — %s%s\n", finding.Severity, finding.RuleID, location, finding.Message, annotation); err != nil {
			return err
		}
	}
	return nil
}

func JSON(w io.Writer, result *model.Result) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
