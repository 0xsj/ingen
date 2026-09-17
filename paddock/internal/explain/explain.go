package explain

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"ingen/core/ciresult"
	"ingen/paddock/internal/model"
)

const schema = "paddock.explanation/v1"

type Document struct {
	Schema       string                 `json:"schema"`
	SourceSchema string                 `json:"source_schema"`
	Status       string                 `json:"status"`
	Root         string                 `json:"root"`
	ModulePath   string                 `json:"module_path"`
	PackageCount int                    `json:"package_count"`
	EdgeCount    int                    `json:"edge_count"`
	Provenance   *Provenance            `json:"provenance,omitempty"`
	Baseline     *model.BaselineSummary `json:"baseline,omitempty"`
	Waivers      []model.WaiverSummary  `json:"waivers,omitempty"`
	Filter       *FindingFilter         `json:"filter,omitempty"`
	Triage       TriageSummary          `json:"triage"`
	Summary      []FindingSummary       `json:"summary"`
	Findings     []FindingExplanation   `json:"findings"`
}

// Provenance identifies the CI artifact and policy inputs from which an
// explanation was derived. It is populated when explain reads an
// ingen.ci-result/v1 artifact; direct paddock.report/v1 explanations do not
// have an enclosing artifact to reference.
type Provenance struct {
	ArtifactSchema   string            `json:"artifact_schema"`
	ToolVersion      string            `json:"tool_version,omitempty"`
	ArtifactPath     string            `json:"artifact_path"`
	ArtifactSHA256   string            `json:"artifact_sha256"`
	ArtifactStatus   string            `json:"artifact_status"`
	ArtifactExitCode int               `json:"artifact_exit_code"`
	CreatedAt        string            `json:"created_at"`
	Policy           FileReference     `json:"policy"`
	PolicyLock       *FileReference    `json:"policy_lock,omitempty"`
	Graph            *FileReference    `json:"graph,omitempty"`
	Baseline         *FileReference    `json:"baseline,omitempty"`
	Adapter          *AdapterReference `json:"adapter,omitempty"`
	SourceVCS        *ciresult.VCS     `json:"source_vcs,omitempty"`
}

type FileReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type AdapterReference struct {
	Kind               string `json:"kind"`
	Name               string `json:"name,omitempty"`
	Version            string `json:"version,omitempty"`
	Executable         string `json:"executable,omitempty"`
	ResolvedExecutable string `json:"resolved_executable,omitempty"`
	ExecutableSHA256   string `json:"executable_sha256,omitempty"`
	ArgsSHA256         string `json:"args_sha256,omitempty"`
	ProfilePath        string `json:"profile_path,omitempty"`
	ProfileSHA256      string `json:"profile_sha256,omitempty"`
}

type FindingFilter struct {
	RuleID string `json:"rule_id,omitempty"`
	Status string `json:"status,omitempty"`
}

// TriageSummary gives an agent an explicit disposition for the selected
// findings. It is not the CI verdict; Document.Status remains the verdict for
// the complete, unfiltered report.
type TriageSummary struct {
	Outcome  string `json:"outcome"`
	Findings int    `json:"findings"`
	Active   int    `json:"active"`
	Blocking int    `json:"blocking"`
	Accepted int    `json:"accepted"`
}

// FindingSummary groups the explanation's findings by rule so callers can
// triage a report before inspecting each individual dependency edge.
type FindingSummary struct {
	RuleID    string `json:"rule_id"`
	Kind      string `json:"kind"`
	Severity  string `json:"severity"`
	Findings  int    `json:"findings"`
	Active    int    `json:"active"`
	Blocking  int    `json:"blocking"`
	Waived    int    `json:"waived"`
	Baselined int    `json:"baselined"`
}

type FindingExplanation struct {
	Finding          *model.Finding     `json:"finding"`
	Rule             *model.RuleSummary `json:"rule,omitempty"`
	RelatedRules     []string           `json:"related_rules,omitempty"`
	Status           string             `json:"status"`
	Observation      string             `json:"observation"`
	Why              string             `json:"why"`
	SuggestedActions []string           `json:"suggested_actions"`
}

func Explain(result *model.Result) Document {
	rules := make(map[string]*model.RuleSummary, len(result.Rules))
	for index := range result.Rules {
		rule := result.Rules[index]
		rules[rule.ID] = &rule
	}
	document := Document{
		Schema:       schema,
		SourceSchema: result.Schema,
		Status:       resultStatus(result),
		Root:         result.Root,
		ModulePath:   result.ModulePath,
		PackageCount: result.PackageCount,
		EdgeCount:    result.EdgeCount,
		Baseline:     result.Baseline,
		Waivers:      result.Waivers,
		Summary:      summarizeFindings(result.Findings, rules),
		Findings:     make([]FindingExplanation, 0, len(result.Findings)),
	}
	relatedRules := relatedRulesByFinding(result.Findings)
	for index, finding := range result.Findings {
		rule := rules[finding.RuleID]
		document.Findings = append(document.Findings, FindingExplanation{
			Finding:          finding,
			Rule:             rule,
			RelatedRules:     relatedRules[index],
			Status:           findingStatus(finding),
			Observation:      observation(finding),
			Why:              finding.Message,
			SuggestedActions: suggestedActions(finding, rule),
		})
	}
	document.Triage = triage(document.Summary)
	return document
}

func Text(w io.Writer, document Document) error {
	if _, err := fmt.Fprintf(w, "EXPLAIN %s %s (%d findings)\n", document.Status, document.Root, len(document.Findings)); err != nil {
		return err
	}
	if document.Provenance != nil {
		if _, err := fmt.Fprintf(w, "PROVENANCE artifact=%s sha256=%s status=%s exit=%d\n", document.Provenance.ArtifactPath, document.Provenance.ArtifactSHA256, document.Provenance.ArtifactStatus, document.Provenance.ArtifactExitCode); err != nil {
			return err
		}
		if document.Provenance.ToolVersion != "" {
			if _, err := fmt.Fprintf(w, "TOOL_VERSION %s\n", document.Provenance.ToolVersion); err != nil {
				return err
			}
		}
		if document.Provenance.SourceVCS != nil {
			changes := ""
			if document.Provenance.SourceVCS.ChangesSHA256 != "" {
				changes = fmt.Sprintf(" changes_sha256=%s", document.Provenance.SourceVCS.ChangesSHA256)
			}
			if _, err := fmt.Fprintf(w, "SOURCE_VCS system=%s revision=%s dirty=%t%s\n", document.Provenance.SourceVCS.System, document.Provenance.SourceVCS.Revision, document.Provenance.SourceVCS.Dirty, changes); err != nil {
				return err
			}
		}
		if document.Provenance.Adapter != nil {
			profile := ""
			if document.Provenance.Adapter.ProfilePath != "" {
				profile = fmt.Sprintf(" profile=%s profile_sha256=%s", document.Provenance.Adapter.ProfilePath, document.Provenance.Adapter.ProfileSHA256)
			}
			if _, err := fmt.Fprintf(w, "ADAPTER kind=%s name=%s executable=%s executable_sha256=%s args_sha256=%s%s\n", document.Provenance.Adapter.Kind, document.Provenance.Adapter.Name, document.Provenance.Adapter.Executable, document.Provenance.Adapter.ExecutableSHA256, document.Provenance.Adapter.ArgsSHA256, profile); err != nil {
				return err
			}
		}
	}
	if document.Filter != nil {
		if _, err := fmt.Fprintf(w, "FILTER rule=%s status=%s\n", document.Filter.RuleID, document.Filter.Status); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "TRIAGE %s (%d findings, %d active, %d blocking, %d accepted)\n", document.Triage.Outcome, document.Triage.Findings, document.Triage.Active, document.Triage.Blocking, document.Triage.Accepted); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "SUMMARY %d rules, %d findings, %d blocking\n", len(document.Summary), len(document.Findings), blockingFindings(document.Summary)); err != nil {
		return err
	}
	for _, summary := range document.Summary {
		if _, err := fmt.Fprintf(w, "  %s (%s/%s): %d findings, %d active, %d blocking\n", summary.RuleID, summary.Kind, summary.Severity, summary.Findings, summary.Active, summary.Blocking); err != nil {
			return err
		}
	}
	for _, waiver := range document.Waivers {
		if waiver.Status == "unused" || waiver.Status == "unused-expired" {
			if _, err := fmt.Fprintf(w, "WARNING: unused waiver %s from %s\n", waiver.RuleID, waiver.From); err != nil {
				return err
			}
		}
	}
	for index, finding := range document.Findings {
		rule := finding.Finding.RuleID
		kind := finding.Finding.Kind
		if finding.Rule != nil && finding.Rule.Kind != "" {
			kind = finding.Rule.Kind
		}
		if _, err := fmt.Fprintf(w, "%d. [%s] %s (%s)\n", index+1, finding.Status, rule, kind); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "   Observed: %s\n", finding.Observation); err != nil {
			return err
		}
		if len(finding.RelatedRules) > 0 {
			if _, err := fmt.Fprintf(w, "   Related: also reported by %s\n", strings.Join(finding.RelatedRules, ", ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "   Why: %s\n", finding.Why); err != nil {
			return err
		}
		for _, action := range finding.SuggestedActions {
			if _, err := fmt.Fprintf(w, "   Next: %s\n", action); err != nil {
				return err
			}
		}
	}
	return nil
}

// relatedRulesByFinding identifies duplicate policy signals for the same
// dependency edge without collapsing the underlying findings. A dependency
// edge is keyed by its source/target and source location, so two imports of
// the same package from different lines remain separate issues.
func relatedRulesByFinding(findings []*model.Finding) map[int][]string {
	type edgeKey struct {
		from string
		to   string
		file string
		line int
	}

	rulesByEdge := make(map[edgeKey]map[string]struct{})
	for _, finding := range findings {
		if finding == nil || finding.To == "" {
			continue
		}
		key := edgeKey{from: finding.From, to: finding.To, file: finding.File, line: finding.Line}
		if rulesByEdge[key] == nil {
			rulesByEdge[key] = make(map[string]struct{})
		}
		rulesByEdge[key][finding.RuleID] = struct{}{}
	}

	result := make(map[int][]string)
	for index, finding := range findings {
		if finding == nil || finding.To == "" {
			continue
		}
		key := edgeKey{from: finding.From, to: finding.To, file: finding.File, line: finding.Line}
		for ruleID := range rulesByEdge[key] {
			if ruleID != finding.RuleID {
				result[index] = append(result[index], ruleID)
			}
		}
		sort.Strings(result[index])
	}
	return result
}

func summarizeFindings(findings []*model.Finding, rules map[string]*model.RuleSummary) []FindingSummary {
	byRule := make(map[string]FindingSummary)
	for _, finding := range findings {
		if finding == nil {
			continue
		}
		addFindingSummary(byRule, finding, rules[finding.RuleID])
	}
	return sortedFindingSummaries(byRule)
}

func summarizeExplanations(findings []FindingExplanation) []FindingSummary {
	byRule := make(map[string]FindingSummary)
	for _, explanation := range findings {
		if explanation.Finding == nil {
			continue
		}
		addFindingSummary(byRule, explanation.Finding, explanation.Rule)
	}
	return sortedFindingSummaries(byRule)
}

func addFindingSummary(byRule map[string]FindingSummary, finding *model.Finding, rule *model.RuleSummary) {
	summary := byRule[finding.RuleID]
	if summary.RuleID == "" {
		summary = FindingSummary{
			RuleID:   finding.RuleID,
			Kind:     finding.Kind,
			Severity: finding.Severity,
		}
		if rule != nil {
			summary.Kind = rule.Kind
			summary.Severity = rule.Severity
		}
	}
	summary.Findings++
	switch {
	case finding.Waived:
		summary.Waived++
	case finding.Baselined:
		summary.Baselined++
	default:
		summary.Active++
		if effectiveSeverity(finding, rule) == "error" {
			summary.Blocking++
		}
	}
	byRule[finding.RuleID] = summary
}

func sortedFindingSummaries(byRule map[string]FindingSummary) []FindingSummary {

	result := make([]FindingSummary, 0, len(byRule))
	for _, summary := range byRule {
		result = append(result, summary)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].RuleID < result[right].RuleID
	})
	return result
}

// ApplyFilter selects findings for an agent while preserving the original
// explanation status and evidence context. The top-level verdict therefore
// continues to describe the complete report, not just the selected subset.
func ApplyFilter(document Document, ruleID, status string) (Document, error) {
	if status == "all" {
		status = ""
	}
	if status != "" && !validFilterStatus(status) {
		return Document{}, fmt.Errorf("unsupported finding status %q; use all, active, blocking, advisory, waived, baselined, or expired-waiver", status)
	}
	if ruleID == "" && status == "" {
		return document, nil
	}

	filtered := document
	filtered.Filter = &FindingFilter{RuleID: ruleID, Status: status}
	filtered.Findings = make([]FindingExplanation, 0, len(document.Findings))
	for _, finding := range document.Findings {
		if finding.Finding == nil {
			continue
		}
		if ruleID != "" && finding.Finding.RuleID != ruleID {
			continue
		}
		if status != "" && !matchesFilterStatus(finding, status) {
			continue
		}
		filtered.Findings = append(filtered.Findings, finding)
	}
	filtered.Summary = summarizeExplanations(filtered.Findings)
	filtered.Triage = triage(filtered.Summary)
	return filtered, nil
}

func triage(summaries []FindingSummary) TriageSummary {
	result := TriageSummary{}
	for _, summary := range summaries {
		result.Findings += summary.Findings
		result.Active += summary.Active
		result.Blocking += summary.Blocking
		result.Accepted += summary.Waived + summary.Baselined
	}
	switch {
	case result.Blocking > 0:
		result.Outcome = "remediate"
	case result.Active > 0:
		result.Outcome = "review"
	case result.Accepted > 0:
		result.Outcome = "accepted"
	default:
		result.Outcome = "clear"
	}
	return result
}

func validFilterStatus(status string) bool {
	switch status {
	case "active", "blocking", "advisory", "waived", "baselined", "expired-waiver":
		return true
	default:
		return false
	}
}

func matchesFilterStatus(finding FindingExplanation, status string) bool {
	switch status {
	case "active":
		return finding.Status != "waived" && finding.Status != "baselined"
	case "blocking":
		return effectiveSeverity(finding.Finding, finding.Rule) == "error" && (finding.Status == "blocking" || finding.Status == "expired-waiver")
	default:
		return finding.Status == status
	}
}

func blockingFindings(summaries []FindingSummary) int {
	blocking := 0
	for _, summary := range summaries {
		blocking += summary.Blocking
	}
	return blocking
}

func JSON(w io.Writer, document Document) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func resultStatus(result *model.Result) string {
	if result.OK() {
		return "PASS"
	}
	return "FAIL"
}

func findingStatus(finding *model.Finding) string {
	switch {
	case finding.WaiverStatus == "expired":
		return "expired-waiver"
	case finding.Waived:
		return "waived"
	case finding.Baselined:
		return "baselined"
	case finding.Severity != "error":
		return "advisory"
	default:
		return "blocking"
	}
}

func effectiveSeverity(finding *model.Finding, rule *model.RuleSummary) string {
	if finding.Severity != "" {
		return finding.Severity
	}
	if rule != nil {
		return rule.Severity
	}
	return ""
}

func observation(finding *model.Finding) string {
	from := finding.From
	if finding.FromComponent != "" {
		from += " [" + finding.FromComponent + "]"
	}
	to := finding.To
	if finding.ToComponent != "" {
		to += " [" + finding.ToComponent + "]"
	}
	if finding.File != "" {
		location := finding.File
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.Line)
		}
		if to != "" {
			return fmt.Sprintf("%s imports %s", location, to)
		}
		return location
	}
	if to != "" {
		return fmt.Sprintf("%s depends on %s", from, to)
	}
	if from != "" {
		return from
	}
	return "the dependency graph contains a policy violation"
}

func suggestedActions(finding *model.Finding, rule *model.RuleSummary) []string {
	if rule == nil {
		return []string{"Inspect the policy rule and move the dependency behind an approved architectural boundary."}
	}
	switch rule.Kind {
	case "allow-dependencies":
		if len(rule.Allow) > 0 {
			return []string{"Move the dependency behind an approved boundary or use one of the allowed targets: " + strings.Join(rule.Allow, ", ")}
		}
		return []string{"Move the dependency behind an approved architectural boundary."}
	case "deny-dependencies":
		action := "Remove the dependency or introduce a boundary that avoids the denied target."
		if len(rule.Deny) > 0 {
			return []string{action + " Denied targets include: " + strings.Join(rule.Deny, ", ") + "."}
		}
		return []string{action}
	case "layer-direction":
		return []string{"Reverse the dependency so it points " + directionText(rule.Direction) + "."}
	case "no-cross-context":
		return []string{"Keep the dependency within the same context or call the other context through its public boundary."}
	case "mediated-dependency":
		if len(rule.AllowTo) > 0 {
			return []string{"Route the dependency through one of the permitted boundaries: " + strings.Join(rule.AllowTo, ", ")}
		}
		return []string{"Route the dependency through an explicit public boundary or shared contract."}
	case "public-api-only":
		return []string{"Import the other context's public API instead of its internal implementation."}
	case "coverage":
		return []string{"Add or adjust a component selector so this source unit has exactly one architectural owner."}
	case "required-dependency":
		if len(rule.Allow) > 0 {
			return []string{"Add a dependency to one of the required targets: " + strings.Join(rule.Allow, ", ")}
		}
		return []string{"Add the required architectural boundary dependency."}
	case "unresolved":
		return []string{"Fix the import path, add the missing source file, or configure the project alias in tsconfig.json."}
	case "no-cycles":
		return []string{"Break the import cycle by extracting a stable boundary, moving shared types, or inverting the dependency."}
	default:
		return []string{"Move the dependency behind an approved architectural boundary."}
	}
}

func directionText(direction string) string {
	switch direction {
	case "toward-lower-layer":
		return "toward a lower layer"
	case "toward-higher-layer":
		return "toward a higher layer"
	default:
		return "in the direction required by the policy"
	}
}
