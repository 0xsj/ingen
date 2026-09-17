package sattler

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// InvestigationSchema identifies the deterministic investigation projection.
const InvestigationSchema = "ingen.sattler-investigation/v0"

// InvestigationConfidence is deliberately evidence-oriented. It describes
// how directly the projection can point back to source artifacts, not the
// quality of the underlying run.
type InvestigationConfidence string

const (
	InvestigationConfidenceDirect  InvestigationConfidence = "direct"
	InvestigationConfidencePartial InvestigationConfidence = "partial"
	InvestigationConfidenceUnknown InvestigationConfidence = "unknown"
)

// InvestigationSourceRef points to one side of an adapter comparison.
type InvestigationSourceRef struct {
	Subsystem string `json:"subsystem"`
	Side      string `json:"side"`
	Path      string `json:"path,omitempty"`
	Field     string `json:"field"`
}

// InvestigationFinding is a focused rule or mutation observation. Related
// evidence IDs mean co-observed context in the same comparison; they do not
// establish a regression or causal relationship.
type InvestigationFinding struct {
	ID                 string                   `json:"id"`
	Kind               string                   `json:"kind"`
	Subject            string                   `json:"subject"`
	Before             any                      `json:"before,omitempty"`
	After              any                      `json:"after,omitempty"`
	Confidence         InvestigationConfidence  `json:"confidence"`
	Sources            []InvestigationSourceRef `json:"sources"`
	RelatedEvidenceIDs []string                 `json:"related_evidence_ids,omitempty"`
}

// InvestigationEvidenceChange is an adapter change retained as context for
// the focused findings. It preserves artifact identity when an adapter
// supplied one.
type InvestigationEvidenceChange struct {
	ID         string                   `json:"id"`
	Subsystem  string                   `json:"subsystem"`
	Field      string                   `json:"field"`
	Before     any                      `json:"before,omitempty"`
	After      any                      `json:"after,omitempty"`
	Identity   ArtifactIdentityRelation `json:"identity,omitempty"`
	Confidence InvestigationConfidence  `json:"confidence"`
	Sources    []InvestigationSourceRef `json:"sources"`
}

// InvestigationUnknown records a deliberately unresolved detail gap.
type InvestigationUnknown struct {
	ID         string                   `json:"id"`
	Subject    string                   `json:"subject"`
	Message    string                   `json:"message"`
	Confidence InvestigationConfidence  `json:"confidence"`
	Sources    []InvestigationSourceRef `json:"sources,omitempty"`
}

// InvestigationReport is a deterministic, neutral projection over a bundle.
// Findings and evidence remain observations of the supplied artifacts.
type InvestigationReport struct {
	Schema         string                        `json:"schema"`
	Manifest       string                        `json:"manifest,omitempty"`
	Compatible     bool                          `json:"compatible"`
	ChangeIDFilter []string                      `json:"change_id_filter,omitempty"`
	Notice         string                        `json:"notice"`
	Findings       []InvestigationFinding        `json:"findings,omitempty"`
	Evidence       []InvestigationEvidenceChange `json:"evidence,omitempty"`
	Unknowns       []InvestigationUnknown        `json:"unknowns,omitempty"`
}

const investigationNotice = "Findings and evidence are co-observed comparison context; they are not regression or causation claims."

// NewInvestigationReport derives the focused projection from a bundle without
// reinterpreting any producer verdict or inferring causation.
func NewInvestigationReport(bundle BundleComparison) InvestigationReport {
	summary := SummarizeBundle(bundle)
	report := InvestigationReport{
		Schema:         InvestigationSchema,
		Manifest:       bundle.Manifest,
		Compatible:     summary.Compatible,
		ChangeIDFilter: append([]string(nil), bundle.ChangeIDFilter...),
		Notice:         investigationNotice,
	}
	report.Evidence = investigationEvidence(bundle)
	report.Findings = investigationFindings(bundle, report.Evidence)
	report.Unknowns = investigationUnknowns(bundle)
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Kind != report.Findings[j].Kind {
			return report.Findings[i].Kind < report.Findings[j].Kind
		}
		return report.Findings[i].ID < report.Findings[j].ID
	})
	sort.Slice(report.Evidence, func(i, j int) bool {
		return report.Evidence[i].ID < report.Evidence[j].ID
	})
	sort.Slice(report.Unknowns, func(i, j int) bool {
		return report.Unknowns[i].ID < report.Unknowns[j].ID
	})
	return report
}

// Validate checks the compatibility-treated investigation projection without
// interpreting findings as regressions or evidence as causation.
func (report InvestigationReport) Validate() error {
	if report.Schema != InvestigationSchema {
		return fmt.Errorf("investigation schema must be %s, got %q", InvestigationSchema, report.Schema)
	}
	if report.Manifest != "" && strings.TrimSpace(report.Manifest) == "" {
		return fmt.Errorf("investigation manifest cannot be empty when present")
	}
	if report.Notice != investigationNotice {
		return fmt.Errorf("investigation notice must preserve the neutral interpretation notice")
	}
	if err := validateInvestigationStringList(report.ChangeIDFilter, "investigation change ID filter"); err != nil {
		return err
	}

	findingIDs := make(map[string]struct{}, len(report.Findings))
	for index, finding := range report.Findings {
		if err := finding.Validate(); err != nil {
			return fmt.Errorf("investigation finding %d: %w", index, err)
		}
		if _, exists := findingIDs[finding.ID]; exists {
			return fmt.Errorf("investigation finding ID %q is duplicated", finding.ID)
		}
		findingIDs[finding.ID] = struct{}{}
	}

	evidenceIDs := make(map[string]struct{}, len(report.Evidence))
	for index, evidence := range report.Evidence {
		if err := evidence.Validate(); err != nil {
			return fmt.Errorf("investigation evidence %d: %w", index, err)
		}
		if _, exists := evidenceIDs[evidence.ID]; exists {
			return fmt.Errorf("investigation evidence ID %q is duplicated", evidence.ID)
		}
		evidenceIDs[evidence.ID] = struct{}{}
	}

	unknownIDs := make(map[string]struct{}, len(report.Unknowns))
	for index, unknown := range report.Unknowns {
		if err := unknown.Validate(); err != nil {
			return fmt.Errorf("investigation unknown %d: %w", index, err)
		}
		if _, exists := unknownIDs[unknown.ID]; exists {
			return fmt.Errorf("investigation unknown ID %q is duplicated", unknown.ID)
		}
		unknownIDs[unknown.ID] = struct{}{}
	}

	for index, finding := range report.Findings {
		for _, evidenceID := range finding.RelatedEvidenceIDs {
			if _, exists := evidenceIDs[evidenceID]; !exists {
				return fmt.Errorf("investigation finding %q related evidence ID %q is missing", finding.ID, evidenceID)
			}
			if strings.TrimSpace(evidenceID) == "" {
				return fmt.Errorf("investigation finding %d related evidence ID cannot be empty", index)
			}
		}
	}
	return nil
}

// Validate checks a focused rule or mutation observation.
func (finding InvestigationFinding) Validate() error {
	if strings.TrimSpace(finding.ID) == "" || strings.TrimSpace(finding.Subject) == "" {
		return fmt.Errorf("finding needs an ID and subject")
	}
	switch finding.Kind {
	case "rule-change", "mutation-change":
	default:
		return fmt.Errorf("finding kind %q is unsupported", finding.Kind)
	}
	if err := validateInvestigationConfidence(finding.Confidence); err != nil {
		return err
	}
	if err := validateInvestigationSources(finding.Sources, true); err != nil {
		return err
	}
	return validateInvestigationStringList(finding.RelatedEvidenceIDs, "finding related evidence IDs")
}

// Validate checks one adapter change retained as investigation context.
func (evidence InvestigationEvidenceChange) Validate() error {
	if strings.TrimSpace(evidence.ID) == "" || strings.TrimSpace(evidence.Field) == "" {
		return fmt.Errorf("evidence needs an ID and field")
	}
	if !isInvestigationSubsystem(evidence.Subsystem) {
		return fmt.Errorf("evidence subsystem %q is unsupported", evidence.Subsystem)
	}
	if err := validateInvestigationConfidence(evidence.Confidence); err != nil {
		return err
	}
	switch evidence.Identity {
	case "", ArtifactIdentitySameBytes, ArtifactIdentityReplaced, ArtifactIdentityAdded, ArtifactIdentityRemoved, ArtifactIdentityUnknown:
	default:
		return fmt.Errorf("evidence identity %q is unsupported", evidence.Identity)
	}
	return validateInvestigationSources(evidence.Sources, true)
}

// Validate checks one deliberately unresolved investigation detail.
func (unknown InvestigationUnknown) Validate() error {
	if strings.TrimSpace(unknown.ID) == "" || strings.TrimSpace(unknown.Subject) == "" || strings.TrimSpace(unknown.Message) == "" {
		return fmt.Errorf("unknown needs an ID, subject, and message")
	}
	if err := validateInvestigationConfidence(unknown.Confidence); err != nil {
		return err
	}
	return validateInvestigationSources(unknown.Sources, false)
}

func validateInvestigationSource(source InvestigationSourceRef) error {
	if !isInvestigationSubsystem(source.Subsystem) {
		return fmt.Errorf("source subsystem %q is unsupported", source.Subsystem)
	}
	switch source.Side {
	case "before", "after":
	default:
		return fmt.Errorf("source side %q is unsupported", source.Side)
	}
	if strings.TrimSpace(source.Field) == "" {
		return fmt.Errorf("source field cannot be empty")
	}
	if source.Path != "" && strings.TrimSpace(source.Path) == "" {
		return fmt.Errorf("source path cannot be empty when present")
	}
	return nil
}

func validateInvestigationSources(sources []InvestigationSourceRef, required bool) error {
	if required && len(sources) == 0 {
		return fmt.Errorf("sources are required")
	}
	for index, source := range sources {
		if err := validateInvestigationSource(source); err != nil {
			return fmt.Errorf("source %d: %w", index, err)
		}
	}
	return nil
}

func validateInvestigationConfidence(confidence InvestigationConfidence) error {
	switch confidence {
	case InvestigationConfidenceDirect, InvestigationConfidencePartial, InvestigationConfidenceUnknown:
		return nil
	default:
		return fmt.Errorf("investigation confidence %q is unsupported", confidence)
	}
}

func validateInvestigationStringList(values []string, name string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot contain an empty value", name)
		}
	}
	return nil
}

func isInvestigationSubsystem(subsystem string) bool {
	switch subsystem {
	case "ci_result", "sorna_run", "nublar_run", "custody", "provenance":
		return true
	default:
		return false
	}
}

// WriteInvestigationJSON writes the machine-readable investigation projection.
func WriteInvestigationJSON(w io.Writer, report InvestigationReport) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteInvestigationText writes an operator-oriented investigation projection.
func WriteInvestigationText(w io.Writer, report InvestigationReport) error {
	if _, err := fmt.Fprintf(w, "Sattler investigation\n  manifest: %s\n  compatible: %t\n  notice: %s\n", report.Manifest, report.Compatible, report.Notice); err != nil {
		return err
	}
	if len(report.ChangeIDFilter) > 0 {
		if _, err := fmt.Fprintf(w, "  change ID filter: %s\n", strings.Join(report.ChangeIDFilter, ", ")); err != nil {
			return err
		}
	}
	if len(report.Findings) == 0 {
		if _, err := fmt.Fprintln(w, "  findings: none observed"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "  findings:"); err != nil {
			return err
		}
		for _, finding := range report.Findings {
			if _, err := fmt.Fprintf(w, "    - %s %q [%s]: %s -> %s\n", finding.Kind, finding.Subject, finding.Confidence, displayValue(finding.Before), displayValue(finding.After)); err != nil {
				return err
			}
			if len(finding.RelatedEvidenceIDs) > 0 {
				if _, err := fmt.Fprintf(w, "      related evidence: %s\n", strings.Join(finding.RelatedEvidenceIDs, ", ")); err != nil {
					return err
				}
			}
			if err := writeInvestigationSources(w, "      ", finding.Sources); err != nil {
				return err
			}
		}
	}
	if len(report.Evidence) == 0 {
		if _, err := fmt.Fprintln(w, "  evidence changes: none observed"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "  evidence changes:"); err != nil {
			return err
		}
		for _, evidence := range report.Evidence {
			identity := ""
			if evidence.Identity != "" {
				identity = " [" + string(evidence.Identity) + "]"
			}
			if _, err := fmt.Fprintf(w, "    - %s%s [%s]: %s -> %s\n", evidence.ID, identity, evidence.Confidence, displayValue(evidence.Before), displayValue(evidence.After)); err != nil {
				return err
			}
		}
	}
	if len(report.Unknowns) == 0 {
		_, err := fmt.Fprintln(w, "  unknowns: none")
		return err
	}
	if _, err := fmt.Fprintln(w, "  unknowns:"); err != nil {
		return err
	}
	for _, unknown := range report.Unknowns {
		if _, err := fmt.Fprintf(w, "    - %s %q [%s]: %s\n", unknown.ID, unknown.Subject, unknown.Confidence, unknown.Message); err != nil {
			return err
		}
		if err := writeInvestigationSources(w, "      ", unknown.Sources); err != nil {
			return err
		}
	}
	return nil
}

func investigationFindings(bundle BundleComparison, evidence []InvestigationEvidenceChange) []InvestigationFinding {
	findings := make([]InvestigationFinding, 0)
	if bundle.SornaRun != nil {
		related := investigationEvidenceIDs(evidence, "sorna_run")
		for _, change := range bundle.SornaRun.ChangedRules {
			field := "rules." + change.RuleID + ".status"
			sources := investigationSources("sorna_run", field, bundle.SornaRun.Before.Path, bundle.SornaRun.After.Path)
			findings = append(findings, InvestigationFinding{
				ID:                 "rule." + change.RuleID,
				Kind:               "rule-change",
				Subject:            change.RuleID,
				Before:             optionalString(change.Before),
				After:              optionalString(change.After),
				Confidence:         confidenceForSources(sources),
				Sources:            sources,
				RelatedEvidenceIDs: related,
			})
		}
		if bundle.SornaRun.Before.Mutation != nil || bundle.SornaRun.After.Mutation != nil {
			before := bundle.SornaRun.Before.Mutation
			after := bundle.SornaRun.After.Mutation
			if !valuesEqual(before, after) {
				beforeID := mutationSummaryID(before)
				afterID := mutationSummaryID(after)
				subject := afterID
				if subject == "" {
					subject = beforeID
				}
				field := "mutation"
				if subject != "" {
					field += "." + subject
				}
				field += ".outcome"
				sources := investigationSources("sorna_run", field, bundle.SornaRun.Before.Path, bundle.SornaRun.After.Path)
				findings = append(findings, InvestigationFinding{
					ID:                 "mutation.sorna_run." + subject,
					Kind:               "mutation-change",
					Subject:            subject,
					Before:             before,
					After:              after,
					Confidence:         confidenceForSources(sources),
					Sources:            sources,
					RelatedEvidenceIDs: related,
				})
			}
		}
	}
	if bundle.CIResult != nil && bundle.CIResult.MutationCampaign != nil {
		related := investigationEvidenceIDs(evidence, "ci_result")
		for _, change := range bundle.CIResult.MutationCampaign.ChangedMutations {
			sources := investigationSources("ci_result", "producer-report.report", bundle.CIResult.Before.Path, bundle.CIResult.After.Path)
			findings = append(findings, InvestigationFinding{
				ID:                 "mutation.ci_result." + change.MutationID,
				Kind:               "mutation-change",
				Subject:            change.MutationID,
				Before:             mutationStateValue(change.Before),
				After:              mutationStateValue(change.After),
				Confidence:         confidenceForSources(sources),
				Sources:            sources,
				RelatedEvidenceIDs: related,
			})
		}
	}
	return findings
}

func investigationEvidence(bundle BundleComparison) []InvestigationEvidenceChange {
	evidence := make([]InvestigationEvidenceChange, 0)
	appendChanges := func(subsystem, beforePath, afterPath string, changes []Change, skip func(Change) bool) {
		for _, change := range changes {
			if skip != nil && skip(change) {
				continue
			}
			field := change.StableID()
			sources := investigationSources(subsystem, field, beforePath, afterPath)
			evidence = append(evidence, InvestigationEvidenceChange{
				ID:         subsystem + "." + field,
				Subsystem:  subsystem,
				Field:      change.Field,
				Before:     change.Before,
				After:      change.After,
				Identity:   change.Identity,
				Confidence: confidenceForSources(sources),
				Sources:    sources,
			})
		}
	}
	if bundle.CIResult != nil {
		appendChanges("ci_result", bundle.CIResult.Before.Path, bundle.CIResult.After.Path, bundle.CIResult.Changes, nil)
	}
	if bundle.SornaRun != nil {
		appendChanges("sorna_run", bundle.SornaRun.Before.Path, bundle.SornaRun.After.Path, bundle.SornaRun.Changes, func(change Change) bool {
			return strings.HasPrefix(change.Category, "rules.") || change.Category == "mutation"
		})
	}
	if bundle.NublarRun != nil {
		appendChanges("nublar_run", bundle.NublarRun.Before.Path, bundle.NublarRun.After.Path, bundle.NublarRun.Changes, nil)
	}
	if bundle.Custody != nil {
		appendChanges("custody", bundle.Custody.Before.Path, bundle.Custody.After.Path, bundle.Custody.Changes, nil)
	}
	if bundle.Provenance != nil {
		appendChanges("provenance", bundle.Provenance.Before.Path, bundle.Provenance.After.Path, bundle.Provenance.Changes, nil)
	}
	return evidence
}

func investigationUnknowns(bundle BundleComparison) []InvestigationUnknown {
	unknowns := make([]InvestigationUnknown, 0, 2)
	if bundle.SornaRun == nil {
		unknowns = append(unknowns, InvestigationUnknown{
			ID:         "missing-sorna-run",
			Subject:    "rule changes",
			Message:    "Sorna rule detail is unavailable because the bundle has no sorna_run comparison",
			Confidence: InvestigationConfidenceUnknown,
		})
	}
	if bundle.CIResult == nil {
		unknowns = append(unknowns, InvestigationUnknown{
			ID:         "missing-ci-result",
			Subject:    "mutation campaign changes",
			Message:    "CI mutation campaign detail is unavailable because the bundle has no ci_result comparison",
			Confidence: InvestigationConfidenceUnknown,
		})
	} else if isSornaMutationCampaign(bundle.CIResult) && bundle.CIResult.MutationCampaign == nil {
		unknowns = append(unknowns, InvestigationUnknown{
			ID:         "missing-mutation-campaign-detail",
			Subject:    "mutation campaign changes",
			Message:    "Sorna mutation campaign detail is unavailable at the CI producer-report boundary",
			Confidence: InvestigationConfidenceUnknown,
			Sources:    investigationSources("ci_result", "producer-report.report", bundle.CIResult.Before.Path, bundle.CIResult.After.Path),
		})
	}
	return unknowns
}

func investigationEvidenceIDs(evidence []InvestigationEvidenceChange, subsystem string) []string {
	ids := make([]string, 0)
	for _, change := range evidence {
		if change.Subsystem == subsystem {
			ids = append(ids, change.ID)
		}
	}
	return ids
}

func investigationSources(subsystem, field, beforePath, afterPath string) []InvestigationSourceRef {
	return []InvestigationSourceRef{
		{Subsystem: subsystem, Side: "before", Path: beforePath, Field: field},
		{Subsystem: subsystem, Side: "after", Path: afterPath, Field: field},
	}
}

func confidenceForSources(sources []InvestigationSourceRef) InvestigationConfidence {
	if len(sources) < 2 {
		return InvestigationConfidenceUnknown
	}
	haveBefore := false
	haveAfter := false
	for _, source := range sources {
		switch source.Side {
		case "before":
			haveBefore = strings.TrimSpace(source.Path) != ""
		case "after":
			haveAfter = strings.TrimSpace(source.Path) != ""
		}
	}
	if haveBefore && haveAfter {
		return InvestigationConfidenceDirect
	}
	if haveBefore || haveAfter {
		return InvestigationConfidencePartial
	}
	return InvestigationConfidenceUnknown
}

func writeInvestigationSources(w io.Writer, indent string, sources []InvestigationSourceRef) error {
	for _, source := range sources {
		if _, err := fmt.Fprintf(w, "%s- %s %s %s", indent, source.Subsystem, source.Side, source.Field); err != nil {
			return err
		}
		if source.Path != "" {
			if _, err := fmt.Fprintf(w, " (%s)", source.Path); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mutationStateValue(value *MutationState) any {
	if value == nil {
		return nil
	}
	return *value
}

func mutationSummaryID(value *SornaRunMutationSummary) string {
	if value == nil {
		return ""
	}
	return value.ID
}

func isSornaMutationCampaign(report *Comparison) bool {
	if report == nil {
		return false
	}
	return (report.Before.Tool == "sorna" && report.Before.Kind == "mutation-campaign") ||
		(report.After.Tool == "sorna" && report.After.Kind == "mutation-campaign")
}
