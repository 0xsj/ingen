package sattler

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// SornaRunSchema is the subject-run schema understood by this adapter.
const SornaRunSchema = "ingen.run/v1"

// SornaRunContractSummary identifies the contract used by a Sorna run.
type SornaRunContractSummary struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

// SornaRunOracleSummary identifies the frozen oracle when a run records one.
type SornaRunOracleSummary struct {
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

// SornaRunBaselineSummary identifies the baseline context for a run.
type SornaRunBaselineSummary struct {
	EvidencePath string                  `json:"evidence_path"`
	RunID        string                  `json:"run_id"`
	Contract     SornaRunContractSummary `json:"contract"`
}

// SornaRunVerdictSummary contains the contract verdict at the Sorna boundary.
type SornaRunVerdictSummary struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// SornaRunSubjectSummary contains the subject identity Sattler can compare.
type SornaRunSubjectSummary struct {
	BaseURL string `json:"base_url"`
	Adapter string `json:"adapter"`
	Variant string `json:"variant,omitempty"`
}

// SornaRunAssuranceSummary preserves assurance context without judging it.
type SornaRunAssuranceSummary struct {
	Level               int      `json:"level"`
	Status              string   `json:"status"`
	ObservationCoverage string   `json:"observation_coverage,omitempty"`
	Limitations         []string `json:"limitations"`
}

// SornaRunLifecycleSummary preserves selected lifecycle context.
type SornaRunLifecycleSummary struct {
	Mode     string `json:"mode"`
	Outcome  string `json:"outcome"`
	ExitCode int    `json:"exit_code"`
}

// SornaRunTotals contains the producer-owned rule counters.
type SornaRunTotals struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Errors       int `json:"errors"`
	Inconclusive int `json:"inconclusive"`
	Skipped      int `json:"skipped"`
}

// SornaRuleSummary is the detail-light representation of one rule result.
// Requests, observations, and assertions remain producer-owned and opaque.
type SornaRuleSummary struct {
	RuleID  string `json:"rule_id"`
	CaseID  string `json:"case_id"`
	Subject string `json:"subject"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
}

// SornaRuleStatusChange identifies a rule whose status changed or appeared or
// disappeared between two runs.
type SornaRuleStatusChange struct {
	RuleID string `json:"rule_id"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// SornaRunMutationSummary preserves optional mutation linkage from a subject
// run without reinterpreting the mutation outcome.
type SornaRunMutationSummary struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
}

// SornaRunSummary contains the fields Sattler can compare directly from an
// ingen.run/v1 record.
type SornaRunSummary struct {
	Path            string                      `json:"path,omitempty"`
	Schema          string                      `json:"schema"`
	RunID           string                      `json:"run_id"`
	CreatedAt       string                      `json:"created_at"`
	Contract        SornaRunContractSummary     `json:"contract"`
	Oracle          *SornaRunOracleSummary      `json:"oracle,omitempty"`
	Baseline        *SornaRunBaselineSummary    `json:"baseline,omitempty"`
	ContractVerdict SornaRunVerdictSummary      `json:"contract_verdict"`
	Subject         SornaRunSubjectSummary      `json:"subject"`
	Assurance       SornaRunAssuranceSummary    `json:"assurance"`
	Lifecycle       *SornaRunLifecycleSummary   `json:"lifecycle,omitempty"`
	Summary         SornaRunTotals              `json:"summary"`
	Rules           map[string]SornaRuleSummary `json:"rules"`
	Mutation        *SornaRunMutationSummary    `json:"mutation,omitempty"`
}

// SornaRunComparison is a deterministic comparison at the Sorna rule-run
// boundary. Detailed request and observation payloads are intentionally not
// copied into the report.
type SornaRunComparison struct {
	Schema               string                  `json:"schema"`
	Compatible           bool                    `json:"compatible"`
	CompatibilityReasons []string                `json:"compatibility_reasons,omitempty"`
	Before               SornaRunSummary         `json:"before"`
	After                SornaRunSummary         `json:"after"`
	Transition           StateTransition         `json:"transition"`
	ChangeIDFilter       []string                `json:"change_id_filter,omitempty"`
	Changes              []Change                `json:"changes,omitempty"`
	ChangedRules         []SornaRuleStatusChange `json:"changed_rules,omitempty"`
	ChangeSummary        ChangeSummary           `json:"change_summary"`
}

const sornaRunComparisonSchema = "ingen.sattler-sorna-run-comparison/v0"

// Validate checks the compatibility-treated Sorna comparison projection.
// Request, observation, and assertion payloads remain outside this contract.
func (report SornaRunComparison) Validate() error {
	if report.Schema != sornaRunComparisonSchema {
		return fmt.Errorf("Sorna comparison schema must be %s, got %q", sornaRunComparisonSchema, report.Schema)
	}
	if err := validateSornaRunSummary("before", report.Before); err != nil {
		return err
	}
	if err := validateSornaRunSummary("after", report.After); err != nil {
		return err
	}
	if err := validateBundleStringList(report.ChangeIDFilter, "Sorna comparison change ID filter"); err != nil {
		return err
	}
	if err := validateBundleAdapterEnvelope("sorna_run", sornaRunComparisonSchema, report.Schema, report.Compatible, report.CompatibilityReasons, report.Transition, report.ChangeIDFilter, report.Changes, report.ChangeSummary, nil); err != nil {
		return err
	}

	expectedRules := make(map[string]struct{})
	for _, change := range report.Changes {
		if change.Field != "status" || !strings.HasPrefix(change.Category, "rules.") {
			continue
		}
		ruleID := strings.TrimPrefix(change.Category, "rules.")
		if strings.TrimSpace(ruleID) == "" {
			return fmt.Errorf("Sorna comparison rule change has an empty rule ID")
		}
		expectedRules[ruleID] = struct{}{}
	}
	seenRules := make(map[string]struct{}, len(report.ChangedRules))
	for index, change := range report.ChangedRules {
		if strings.TrimSpace(change.RuleID) == "" {
			return fmt.Errorf("Sorna comparison changed_rules[%d] needs a rule ID", index)
		}
		if change.Before == "" && change.After == "" {
			return fmt.Errorf("Sorna comparison changed rule %q needs a before or after status", change.RuleID)
		}
		if change.Before != "" && !validSornaRuleStatus(change.Before) {
			return fmt.Errorf("Sorna comparison changed rule %q has invalid before status %q", change.RuleID, change.Before)
		}
		if change.After != "" && !validSornaRuleStatus(change.After) {
			return fmt.Errorf("Sorna comparison changed rule %q has invalid after status %q", change.RuleID, change.After)
		}
		if _, exists := seenRules[change.RuleID]; exists {
			return fmt.Errorf("Sorna comparison changed rule %q is duplicated", change.RuleID)
		}
		seenRules[change.RuleID] = struct{}{}
		if _, exists := expectedRules[change.RuleID]; !exists {
			return fmt.Errorf("Sorna comparison changed rule %q has no matching rule change", change.RuleID)
		}
	}
	if len(seenRules) != len(expectedRules) {
		return fmt.Errorf("Sorna comparison changed rule index does not match rule changes")
	}
	return nil
}

func validateSornaRunSummary(side string, summary SornaRunSummary) error {
	if summary.Schema != SornaRunSchema {
		return fmt.Errorf("Sorna %s schema must be %s, got %q", side, SornaRunSchema, summary.Schema)
	}
	if summary.Path != "" && strings.TrimSpace(summary.Path) == "" {
		return fmt.Errorf("Sorna %s path cannot be empty when present", side)
	}
	if strings.TrimSpace(summary.RunID) == "" || strings.TrimSpace(summary.CreatedAt) == "" {
		return fmt.Errorf("Sorna %s needs a run ID and creation timestamp", side)
	}
	if _, err := time.Parse(time.RFC3339Nano, summary.CreatedAt); err != nil {
		return fmt.Errorf("Sorna %s created_at must be RFC3339: %w", side, err)
	}
	if strings.TrimSpace(summary.Contract.ID) == "" || summary.Contract.Version < 1 {
		return fmt.Errorf("Sorna %s needs a contract ID and positive version", side)
	}
	if err := validateSornaSHA256("Sorna "+side+" contract", summary.Contract.SHA256); err != nil {
		return err
	}
	if summary.Oracle != nil {
		if summary.Oracle.Schema != "ingen.oracle/v1" {
			return fmt.Errorf("Sorna %s oracle schema must be ingen.oracle/v1, got %q", side, summary.Oracle.Schema)
		}
		if err := validateSornaSHA256("Sorna "+side+" oracle", summary.Oracle.SHA256); err != nil {
			return err
		}
	}
	if summary.Baseline != nil {
		if strings.TrimSpace(summary.Baseline.EvidencePath) == "" || strings.TrimSpace(summary.Baseline.RunID) == "" {
			return fmt.Errorf("Sorna %s baseline needs evidence path and run ID", side)
		}
		if strings.TrimSpace(summary.Baseline.Contract.ID) == "" || summary.Baseline.Contract.Version < 1 {
			return fmt.Errorf("Sorna %s baseline needs a contract ID and positive version", side)
		}
		if err := validateSornaSHA256("Sorna "+side+" baseline contract", summary.Baseline.Contract.SHA256); err != nil {
			return err
		}
	}
	if !validSornaVerdictStatus(summary.ContractVerdict.Status) {
		return fmt.Errorf("Sorna %s has invalid contract verdict status %q", side, summary.ContractVerdict.Status)
	}
	if strings.TrimSpace(summary.Subject.BaseURL) == "" || strings.TrimSpace(summary.Subject.Adapter) == "" {
		return fmt.Errorf("Sorna %s needs subject base URL and adapter", side)
	}
	if summary.Assurance.Level < 0 || strings.TrimSpace(summary.Assurance.Status) == "" {
		return fmt.Errorf("Sorna %s needs a valid assurance summary", side)
	}
	for _, limitation := range summary.Assurance.Limitations {
		if strings.TrimSpace(limitation) == "" {
			return fmt.Errorf("Sorna %s assurance limitations cannot be empty", side)
		}
	}
	if summary.Lifecycle != nil {
		if strings.TrimSpace(summary.Lifecycle.Mode) == "" || strings.TrimSpace(summary.Lifecycle.Outcome) == "" || summary.Lifecycle.ExitCode < 0 {
			return fmt.Errorf("Sorna %s lifecycle is incomplete", side)
		}
	}
	if summary.Summary.Passed < 0 || summary.Summary.Failed < 0 || summary.Summary.Errors < 0 || summary.Summary.Inconclusive < 0 || summary.Summary.Skipped < 0 {
		return fmt.Errorf("Sorna %s summary counts must not be negative", side)
	}
	if len(summary.Rules) == 0 {
		return fmt.Errorf("Sorna %s needs at least one rule", side)
	}
	for ruleID, rule := range summary.Rules {
		if strings.TrimSpace(ruleID) == "" || rule.RuleID != ruleID || strings.TrimSpace(rule.CaseID) == "" || strings.TrimSpace(rule.Subject) == "" {
			return fmt.Errorf("Sorna %s contains an invalid rule %q", side, ruleID)
		}
		if !validSornaRuleStatus(rule.Status) {
			return fmt.Errorf("Sorna %s rule %q has invalid status %q", side, ruleID, rule.Status)
		}
	}
	if summary.Mutation != nil {
		if strings.TrimSpace(summary.Mutation.ID) == "" || !validSornaMutationOutcome(summary.Mutation.Outcome) {
			return fmt.Errorf("Sorna %s has an invalid mutation summary", side)
		}
	}
	return nil
}

type sornaRunContract struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

type sornaRunOracle struct {
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

type sornaRunBaseline struct {
	EvidencePath string           `json:"evidence_path"`
	RunID        string           `json:"run_id"`
	Contract     sornaRunContract `json:"contract"`
}

type sornaRunVerdict struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type sornaRunSubject struct {
	BaseURL string `json:"base_url"`
	Adapter string `json:"adapter"`
	Variant string `json:"variant"`
}

type sornaRunAssurance struct {
	Level               int      `json:"level"`
	Status              string   `json:"status"`
	ObservationCoverage string   `json:"observation_coverage"`
	Limitations         []string `json:"limitations"`
}

type sornaRunLifecycle struct {
	Mode     string `json:"mode"`
	Outcome  string `json:"outcome"`
	ExitCode int    `json:"exit_code"`
}

type sornaRunTotals struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Errors       int `json:"errors"`
	Inconclusive int `json:"inconclusive"`
	Skipped      int `json:"skipped"`
}

type sornaRunRule struct {
	RuleID  string `json:"rule_id"`
	CaseID  string `json:"case_id"`
	Subject string `json:"subject"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

type sornaRunMutation struct {
	Spec struct {
		ID string `json:"id"`
	} `json:"spec"`
	Outcome string `json:"outcome"`
}

type sornaRunDocument struct {
	Schema          string             `json:"schema"`
	RunID           string             `json:"run_id"`
	CreatedAt       string             `json:"created_at"`
	Assurance       sornaRunAssurance  `json:"assurance"`
	Contract        sornaRunContract   `json:"contract"`
	Oracle          *sornaRunOracle    `json:"oracle"`
	Baseline        *sornaRunBaseline  `json:"baseline"`
	ContractVerdict sornaRunVerdict    `json:"contract_verdict"`
	Subject         sornaRunSubject    `json:"subject"`
	Lifecycle       *sornaRunLifecycle `json:"lifecycle"`
	Summary         sornaRunTotals     `json:"summary"`
	Rules           []sornaRunRule     `json:"rules"`
	Mutation        *sornaRunMutation  `json:"mutation"`
}

// CompareSornaRunFiles loads and compares two ingen.run/v1 records.
func CompareSornaRunFiles(beforePath, afterPath string) (SornaRunComparison, error) {
	before, err := loadSornaRun(beforePath)
	if err != nil {
		return SornaRunComparison{}, err
	}
	after, err := loadSornaRun(afterPath)
	if err != nil {
		return SornaRunComparison{}, err
	}
	report := compareSornaRuns(before, after)
	report.Before.Path = beforePath
	report.After.Path = afterPath
	return report, nil
}

// WriteSornaJSON writes the compatibility-treated machine-readable Sorna
// comparison.
func WriteSornaJSON(w io.Writer, report SornaRunComparison) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteSornaText writes a compact Sorna rule-run comparison.
func WriteSornaText(w io.Writer, report SornaRunComparison) error {
	if _, err := fmt.Fprintf(w, "Sattler Sorna run comparison\n  before: %s (%s, %s)\n  after:  %s (%s, %s)\n  compatible: %t\n  transition: %s\n", report.Before.Path, report.Before.RunID, report.Before.ContractVerdict.Status, report.After.Path, report.After.RunID, report.After.ContractVerdict.Status, report.Compatible, report.Transition); err != nil {
		return err
	}
	if len(report.ChangeIDFilter) > 0 {
		if _, err := fmt.Fprintf(w, "  change ID filter: %s\n", strings.Join(report.ChangeIDFilter, ", ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  contract: %s@%d -> %s@%d\n  summary: %d passed, %d failed, %d errors, %d inconclusive, %d skipped\n  change summary: %s\n", report.Before.Contract.ID, report.Before.Contract.Version, report.After.Contract.ID, report.After.Contract.Version, report.After.Summary.Passed, report.After.Summary.Failed, report.After.Summary.Errors, report.After.Summary.Inconclusive, report.After.Summary.Skipped, report.ChangeSummary); err != nil {
		return err
	}
	if len(report.CompatibilityReasons) > 0 {
		if _, err := fmt.Fprintln(w, "  compatibility reasons:"); err != nil {
			return err
		}
		for _, reason := range report.CompatibilityReasons {
			if _, err := fmt.Fprintf(w, "    - %s\n", reason); err != nil {
				return err
			}
		}
	}
	if len(report.Changes) == 0 {
		_, err := fmt.Fprintln(w, "  changes: none observable at the Sorna run boundary")
		return err
	}
	if _, err := fmt.Fprintln(w, "  changes:"); err != nil {
		return err
	}
	for _, change := range report.Changes {
		if _, err := fmt.Fprintf(w, "    - %s %s (id=%s): %s -> %s\n", change.Category, change.Field, change.StableID(), displayValue(change.Before), displayValue(change.After)); err != nil {
			return err
		}
	}
	return nil
}

func loadSornaRun(path string) (sornaRunDocument, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return sornaRunDocument{}, fmt.Errorf("read Sorna run %s: %w", path, err)
	}
	var document sornaRunDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		return sornaRunDocument{}, fmt.Errorf("parse Sorna run %s: %w", path, err)
	}
	if err := validateSornaRun(path, document); err != nil {
		return sornaRunDocument{}, err
	}
	return document, nil
}

func validateSornaRun(path string, document sornaRunDocument) error {
	if document.Schema != SornaRunSchema {
		return fmt.Errorf("Sorna run schema must be %s, got %q", SornaRunSchema, document.Schema)
	}
	if strings.TrimSpace(document.RunID) == "" {
		return fmt.Errorf("Sorna run %s needs a run_id", path)
	}
	if strings.TrimSpace(document.CreatedAt) == "" {
		return fmt.Errorf("Sorna run %s needs created_at", path)
	}
	if _, err := time.Parse(time.RFC3339Nano, document.CreatedAt); err != nil {
		return fmt.Errorf("Sorna run %s created_at must be RFC3339: %w", path, err)
	}
	if strings.TrimSpace(document.Contract.ID) == "" || document.Contract.Version < 1 {
		return fmt.Errorf("Sorna run %s needs a contract id and positive version", path)
	}
	if err := validateSornaSHA256(path+" contract", document.Contract.SHA256); err != nil {
		return err
	}
	if document.Oracle != nil {
		if document.Oracle.Schema != "ingen.oracle/v1" {
			return fmt.Errorf("Sorna run %s oracle schema must be ingen.oracle/v1, got %q", path, document.Oracle.Schema)
		}
		if err := validateSornaSHA256(path+" oracle", document.Oracle.SHA256); err != nil {
			return err
		}
	}
	if document.Baseline != nil {
		if strings.TrimSpace(document.Baseline.EvidencePath) == "" || strings.TrimSpace(document.Baseline.RunID) == "" {
			return fmt.Errorf("Sorna run %s baseline needs evidence_path and run_id", path)
		}
		if err := validateSornaSHA256(path+" baseline contract", document.Baseline.Contract.SHA256); err != nil {
			return err
		}
	}
	if !validSornaVerdictStatus(document.ContractVerdict.Status) {
		return fmt.Errorf("Sorna run %s has invalid contract_verdict status %q", path, document.ContractVerdict.Status)
	}
	if strings.TrimSpace(document.Subject.BaseURL) == "" || strings.TrimSpace(document.Subject.Adapter) == "" {
		return fmt.Errorf("Sorna run %s needs subject base_url and adapter", path)
	}
	if strings.TrimSpace(document.Assurance.Status) == "" || document.Assurance.Level < 0 {
		return fmt.Errorf("Sorna run %s needs a valid assurance summary", path)
	}
	if document.Lifecycle != nil && strings.TrimSpace(document.Lifecycle.Mode) == "" {
		return fmt.Errorf("Sorna run %s lifecycle needs a mode", path)
	}
	if document.Summary.Passed < 0 || document.Summary.Failed < 0 || document.Summary.Errors < 0 || document.Summary.Inconclusive < 0 || document.Summary.Skipped < 0 {
		return fmt.Errorf("Sorna run %s summary counts must not be negative", path)
	}
	if len(document.Rules) == 0 {
		return fmt.Errorf("Sorna run %s needs at least one rule", path)
	}
	seen := make(map[string]bool, len(document.Rules))
	for _, rule := range document.Rules {
		if strings.TrimSpace(rule.RuleID) == "" || strings.TrimSpace(rule.CaseID) == "" {
			return fmt.Errorf("Sorna run %s contains a rule without rule_id and case_id", path)
		}
		if seen[rule.RuleID] {
			return fmt.Errorf("Sorna run %s duplicates rule %q", path, rule.RuleID)
		}
		seen[rule.RuleID] = true
		if !validSornaRuleStatus(rule.Status) {
			return fmt.Errorf("Sorna run %s rule %q has invalid status %q", path, rule.RuleID, rule.Status)
		}
	}
	if document.Mutation != nil {
		if strings.TrimSpace(document.Mutation.Spec.ID) == "" || !validSornaMutationOutcome(document.Mutation.Outcome) {
			return fmt.Errorf("Sorna run %s has an invalid mutation summary", path)
		}
	}
	return nil
}

func validateSornaSHA256(name, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("%s sha256 must be 64 hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s sha256 is invalid: %w", name, err)
	}
	return nil
}

func validSornaVerdictStatus(status string) bool {
	switch status {
	case "pass", "fail", "error", "inconclusive":
		return true
	default:
		return false
	}
}

func validSornaRuleStatus(status string) bool {
	switch status {
	case "pass", "fail", "error", "inconclusive", "skipped":
		return true
	default:
		return false
	}
}

func validSornaMutationOutcome(outcome string) bool {
	switch outcome {
	case "killed", "survived", "inconclusive":
		return true
	default:
		return false
	}
}

func compareSornaRuns(before, after sornaRunDocument) SornaRunComparison {
	comparison := SornaRunComparison{
		Schema: sornaRunComparisonSchema,
		Before: summarizeSornaRun(before),
		After:  summarizeSornaRun(after),
	}
	if before.Contract.ID != after.Contract.ID {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("contract id changed from %q to %q", before.Contract.ID, after.Contract.ID))
	}
	if before.Contract.Version != after.Contract.Version {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("contract version changed from %d to %d", before.Contract.Version, after.Contract.Version))
	}
	if before.Contract.SHA256 != after.Contract.SHA256 {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("contract sha256 changed from %q to %q", before.Contract.SHA256, after.Contract.SHA256))
	}
	comparison.Compatible = len(comparison.CompatibilityReasons) == 0
	comparison.Transition = NewStateTransition("contract_verdict.status", before.ContractVerdict.Status, after.ContractVerdict.Status, comparison.Compatible)
	add := func(category, field string, oldValue, newValue any) {
		if valuesEqual(oldValue, newValue) {
			return
		}
		comparison.Changes = append(comparison.Changes, NewChange(category, field, oldValue, newValue))
	}
	add("contract", "id", before.Contract.ID, after.Contract.ID)
	add("contract", "version", before.Contract.Version, after.Contract.Version)
	add("contract", "sha256", before.Contract.SHA256, after.Contract.SHA256)
	add("context", "oracle", before.Oracle, after.Oracle)
	add("context", "subject.adapter", before.Subject.Adapter, after.Subject.Adapter)
	add("context", "subject.variant", before.Subject.Variant, after.Subject.Variant)
	add("verdict", "status", before.ContractVerdict.Status, after.ContractVerdict.Status)
	add("verdict", "reason", before.ContractVerdict.Reason, after.ContractVerdict.Reason)
	add("summary", "passed", before.Summary.Passed, after.Summary.Passed)
	add("summary", "failed", before.Summary.Failed, after.Summary.Failed)
	add("summary", "errors", before.Summary.Errors, after.Summary.Errors)
	add("summary", "inconclusive", before.Summary.Inconclusive, after.Summary.Inconclusive)
	add("summary", "skipped", before.Summary.Skipped, after.Summary.Skipped)
	add("assurance", "level", before.Assurance.Level, after.Assurance.Level)
	add("assurance", "status", before.Assurance.Status, after.Assurance.Status)
	add("assurance", "observation_coverage", before.Assurance.ObservationCoverage, after.Assurance.ObservationCoverage)
	var beforeLifecycle, afterLifecycle *SornaRunLifecycleSummary
	if before.Lifecycle != nil {
		value := summarizeSornaLifecycle(*before.Lifecycle)
		beforeLifecycle = &value
	}
	if after.Lifecycle != nil {
		value := summarizeSornaLifecycle(*after.Lifecycle)
		afterLifecycle = &value
	}
	if beforeLifecycle != nil || afterLifecycle != nil {
		add("lifecycle", "mode", lifecycleString(beforeLifecycle, "mode"), lifecycleString(afterLifecycle, "mode"))
		add("lifecycle", "outcome", lifecycleString(beforeLifecycle, "outcome"), lifecycleString(afterLifecycle, "outcome"))
		add("lifecycle", "exit_code", lifecycleExitCode(beforeLifecycle), lifecycleExitCode(afterLifecycle))
	}
	add("mutation", "id", mutationID(comparison.Before.Mutation), mutationID(comparison.After.Mutation))
	add("mutation", "outcome", mutationOutcome(comparison.Before.Mutation), mutationOutcome(comparison.After.Mutation))

	ruleIDs := make([]string, 0, len(comparison.Before.Rules)+len(comparison.After.Rules))
	seen := make(map[string]bool, len(comparison.Before.Rules)+len(comparison.After.Rules))
	for id := range comparison.Before.Rules {
		seen[id] = true
	}
	for id := range comparison.After.Rules {
		seen[id] = true
	}
	for id := range seen {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs {
		beforeRule, beforeOK := comparison.Before.Rules[id]
		afterRule, afterOK := comparison.After.Rules[id]
		var beforeStatus, afterStatus any
		if beforeOK {
			beforeStatus = beforeRule.Status
		}
		if afterOK {
			afterStatus = afterRule.Status
		}
		if valuesEqual(beforeStatus, afterStatus) {
			continue
		}
		add("rules."+id, "status", beforeStatus, afterStatus)
		change := SornaRuleStatusChange{RuleID: id}
		if beforeOK {
			change.Before = beforeRule.Status
		}
		if afterOK {
			change.After = afterRule.Status
		}
		comparison.ChangedRules = append(comparison.ChangedRules, change)
	}
	comparison.ChangeSummary = SummarizeChanges(comparison.Changes)
	return comparison
}

func summarizeSornaRun(document sornaRunDocument) SornaRunSummary {
	rules := make(map[string]SornaRuleSummary, len(document.Rules))
	for _, rule := range document.Rules {
		rules[rule.RuleID] = SornaRuleSummary{RuleID: rule.RuleID, CaseID: rule.CaseID, Subject: rule.Subject, Status: rule.Status, Reason: rule.Reason}
	}
	var oracle *SornaRunOracleSummary
	if document.Oracle != nil {
		value := SornaRunOracleSummary{Schema: document.Oracle.Schema, SHA256: document.Oracle.SHA256}
		oracle = &value
	}
	var baseline *SornaRunBaselineSummary
	if document.Baseline != nil {
		baseline = &SornaRunBaselineSummary{EvidencePath: document.Baseline.EvidencePath, RunID: document.Baseline.RunID, Contract: summarizeSornaContract(document.Baseline.Contract)}
	}
	var lifecycle *SornaRunLifecycleSummary
	if document.Lifecycle != nil {
		value := summarizeSornaLifecycle(*document.Lifecycle)
		lifecycle = &value
	}
	var mutation *SornaRunMutationSummary
	if document.Mutation != nil {
		mutation = &SornaRunMutationSummary{ID: document.Mutation.Spec.ID, Outcome: document.Mutation.Outcome}
	}
	limitations := append([]string{}, document.Assurance.Limitations...)
	return SornaRunSummary{
		Schema:          document.Schema,
		RunID:           document.RunID,
		CreatedAt:       document.CreatedAt,
		Contract:        summarizeSornaContract(document.Contract),
		Oracle:          oracle,
		Baseline:        baseline,
		ContractVerdict: SornaRunVerdictSummary{Status: document.ContractVerdict.Status, Reason: document.ContractVerdict.Reason},
		Subject:         SornaRunSubjectSummary{BaseURL: document.Subject.BaseURL, Adapter: document.Subject.Adapter, Variant: document.Subject.Variant},
		Assurance:       SornaRunAssuranceSummary{Level: document.Assurance.Level, Status: document.Assurance.Status, ObservationCoverage: document.Assurance.ObservationCoverage, Limitations: limitations},
		Lifecycle:       lifecycle,
		Summary:         SornaRunTotals{Passed: document.Summary.Passed, Failed: document.Summary.Failed, Errors: document.Summary.Errors, Inconclusive: document.Summary.Inconclusive, Skipped: document.Summary.Skipped},
		Rules:           rules,
		Mutation:        mutation,
	}
}

func summarizeSornaContract(contract sornaRunContract) SornaRunContractSummary {
	return SornaRunContractSummary{ID: contract.ID, Version: contract.Version, SHA256: contract.SHA256}
}

func summarizeSornaLifecycle(lifecycle sornaRunLifecycle) SornaRunLifecycleSummary {
	return SornaRunLifecycleSummary{Mode: lifecycle.Mode, Outcome: lifecycle.Outcome, ExitCode: lifecycle.ExitCode}
}

func lifecycleString(lifecycle *SornaRunLifecycleSummary, field string) string {
	if lifecycle == nil {
		return ""
	}
	if field == "mode" {
		return lifecycle.Mode
	}
	return lifecycle.Outcome
}

func lifecycleExitCode(lifecycle *SornaRunLifecycleSummary) any {
	if lifecycle == nil {
		return nil
	}
	return lifecycle.ExitCode
}

func mutationID(mutation *SornaRunMutationSummary) string {
	if mutation == nil {
		return ""
	}
	return mutation.ID
}

func mutationOutcome(mutation *SornaRunMutationSummary) string {
	if mutation == nil {
		return ""
	}
	return mutation.Outcome
}
