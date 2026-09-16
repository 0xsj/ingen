package mutation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/oracle"
)

// ContractMutationResultSchema is the versioned report produced by the
// contract-mutation inspection path. Contract mutations challenge the
// contract/oracle boundary; they do not prepare or launch a subject.
const ContractMutationResultSchema = "ingen.contract-mutation-result/v1"

// ContractMutationReport is an analysis artifact, not a subject verdict. A
// visible mutation changed the regenerated oracle; an equivalent mutation did
// not; an invalid mutation could not produce a valid contract/oracle pair.
type ContractMutationReport struct {
	Schema   string                          `json:"schema"`
	Status   string                          `json:"status"`
	Contract ContractMutationReference       `json:"contract"`
	Oracle   ContractMutationOracleReference `json:"oracle"`
	Summary  ContractMutationSummary         `json:"summary"`
	Entries  []ContractMutationEntry         `json:"entries"`
}

// ContractMutationReference binds the report to the original sealed
// contract. It deliberately mirrors the runner reference without importing
// runner into the mutation package.
type ContractMutationReference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

// ContractMutationOracleReference binds the report to the original frozen
// oracle and its policy identity.
type ContractMutationOracleReference struct {
	Schema       string `json:"schema"`
	SHA256       string `json:"sha256"`
	PolicySHA256 string `json:"policy_sha256"`
}

// ContractMutationSummary counts analysis outcomes without pretending that
// contract mutation visibility is a correctness score.
type ContractMutationSummary struct {
	Analyzed   int `json:"analyzed"`
	Visible    int `json:"visible"`
	Equivalent int `json:"equivalent"`
	Invalid    int `json:"invalid"`
}

// ContractMutationEntry records the regenerated artifact identities and the
// rule-level difference for one contract mutation.
type ContractMutationEntry struct {
	ID                    string   `json:"id"`
	Plane                 string   `json:"plane"`
	Operator              string   `json:"operator"`
	Target                string   `json:"target"`
	Outcome               string   `json:"outcome"`
	MutatedContractSHA256 string   `json:"mutated_contract_sha256,omitempty"`
	MutatedOracleSHA256   string   `json:"mutated_oracle_sha256,omitempty"`
	ChangedRuleIDs        []string `json:"changed_rule_ids,omitempty"`
	RemovedRuleIDs        []string `json:"removed_rule_ids,omitempty"`
	AddedRuleIDs          []string `json:"added_rule_ids,omitempty"`
	Reason                string   `json:"reason,omitempty"`
}

// ApplyContract applies one reviewed contract-plane mutation to a copy of a
// contract document. The input document is never modified. Targets use the
// explicit form `rule:<rule-id>` so a contract mutation cannot accidentally
// resolve against an HTTP subject or source location.
func ApplyContract(document contract.Document, spec Spec) (contract.Document, error) {
	if spec.Plane != "contract" {
		return contract.Document{}, fmt.Errorf("contract mutation %q must use plane contract", spec.ID)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		return contract.Document{}, fmt.Errorf("invalid contract before mutation: %s", strings.Join(problems, "; "))
	}
	mutated, err := cloneDocument(document)
	if err != nil {
		return contract.Document{}, err
	}
	// A sealed input is an immutable source artifact. Re-sealing the mutated
	// copy must create the next artifact rather than fail on status alone.
	mutated.Contract["status"] = "draft"
	rules, ok := mutated.Contract["rules"].([]any)
	if !ok {
		return contract.Document{}, fmt.Errorf("contract.rules must be a list")
	}
	ruleIndex, rule, err := findTargetRule(rules, spec.Target)
	if err != nil {
		return contract.Document{}, fmt.Errorf("apply contract mutation %q: %w", spec.ID, err)
	}

	switch spec.Operator {
	case "contract.rule.remove":
		if declared, present := spec.Change["rule_id"]; present {
			value, ok := declared.(string)
			if !ok || value != ruleID(rule) {
				return contract.Document{}, fmt.Errorf("change.rule_id must match target rule %q", ruleID(rule))
			}
		}
		mutated.Contract["rules"] = append(rules[:ruleIndex], rules[ruleIndex+1:]...)
	case "contract.rule.strength.replace":
		from, err := requiredStringChange(spec, "from")
		if err != nil {
			return contract.Document{}, err
		}
		to, err := requiredStringChange(spec, "to")
		if err != nil {
			return contract.Document{}, err
		}
		current, ok := rule["strength"].(string)
		if !ok || current != from {
			return contract.Document{}, fmt.Errorf("target rule %q strength is %q, want declared from %q", ruleID(rule), current, from)
		}
		rule["strength"] = to
	case "contract.rule.expect.status.replace":
		from, err := requiredIntegerChange(spec, "from")
		if err != nil {
			return contract.Document{}, err
		}
		to, err := requiredIntegerChange(spec, "to")
		if err != nil {
			return contract.Document{}, err
		}
		expect, ok := rule["expect"].(map[string]any)
		if !ok {
			return contract.Document{}, fmt.Errorf("target rule %q does not contain an expect object", ruleID(rule))
		}
		current, ok := integerValue(expect["status"])
		if !ok || current != from {
			return contract.Document{}, fmt.Errorf("target rule %q status is %v, want declared from %d", ruleID(rule), expect["status"], from)
		}
		expect["status"] = to
	case "contract.rule.expect.required.remove":
		field, err := requiredStringChange(spec, "field")
		if err != nil {
			return contract.Document{}, err
		}
		expect, ok := rule["expect"].(map[string]any)
		if !ok {
			return contract.Document{}, fmt.Errorf("target rule %q does not contain an expect object", ruleID(rule))
		}
		body, ok := expect["body"].(map[string]any)
		if !ok {
			return contract.Document{}, fmt.Errorf("target rule %q does not contain an expect.body object", ruleID(rule))
		}
		required, ok := body["required"].([]any)
		if !ok {
			return contract.Document{}, fmt.Errorf("target rule %q does not contain an expect.body.required list", ruleID(rule))
		}
		filtered := make([]any, 0, len(required))
		removed := false
		for _, raw := range required {
			value, ok := raw.(string)
			if ok && value == field {
				removed = true
				continue
			}
			filtered = append(filtered, raw)
		}
		if !removed {
			return contract.Document{}, fmt.Errorf("target rule %q does not require field %q", ruleID(rule), field)
		}
		body["required"] = filtered
	default:
		return contract.Document{}, fmt.Errorf("unsupported contract mutation operator %q", spec.Operator)
	}

	if problems := contract.Validate(mutated); len(problems) > 0 {
		return contract.Document{}, fmt.Errorf("contract mutation %q produced an invalid contract: %s", spec.ID, strings.Join(problems, "; "))
	}
	return mutated, nil
}

// AnalyzeContractMutations applies contract-plane mutations and regenerates
// the oracle under the original policy identity. It intentionally reports
// visibility instead of running the subject: changing the contract changes
// the authority against which a subject would be judged.
func AnalyzeContractMutations(original contract.Sealed, originalOracle oracle.Artifact, specs []Spec) (ContractMutationReport, error) {
	if problems := contract.Validate(original.Document); len(problems) > 0 {
		return ContractMutationReport{}, fmt.Errorf("invalid original contract: %s", strings.Join(problems, "; "))
	}
	if problems := oracle.Validate(originalOracle); len(problems) > 0 {
		return ContractMutationReport{}, fmt.Errorf("invalid original oracle: %s", strings.Join(problems, "; "))
	}
	if originalOracle.Contract.ID != contractID(original.Document) || originalOracle.Contract.Version != contractVersion(original.Document) || originalOracle.Contract.SHA256 != original.SHA256 {
		return ContractMutationReport{}, fmt.Errorf("original oracle does not match the sealed contract")
	}
	originalOracleHash, err := oracle.Hash(originalOracle)
	if err != nil {
		return ContractMutationReport{}, fmt.Errorf("hash original oracle: %w", err)
	}
	report := ContractMutationReport{
		Schema: ContractMutationResultSchema,
		Status: "completed",
		Contract: ContractMutationReference{
			ID:      contractID(original.Document),
			Version: contractVersion(original.Document),
			SHA256:  original.SHA256,
		},
		Oracle: ContractMutationOracleReference{
			Schema:       originalOracle.Schema,
			SHA256:       originalOracleHash,
			PolicySHA256: originalOracle.PolicySHA256,
		},
		Entries: make([]ContractMutationEntry, 0, len(specs)),
	}
	for _, spec := range specs {
		if spec.Plane != "contract" {
			continue
		}
		entry := ContractMutationEntry{
			ID:       spec.ID,
			Plane:    spec.Plane,
			Operator: spec.Operator,
			Target:   spec.Target,
			Outcome:  "invalid",
		}
		report.Summary.Analyzed++
		mutatedDocument, applyErr := ApplyContract(original.Document, spec)
		if applyErr != nil {
			entry.Reason = applyErr.Error()
			report.Summary.Invalid++
			report.Entries = append(report.Entries, entry)
			continue
		}
		mutated, sealErr := contract.Seal(mutatedDocument)
		if sealErr != nil {
			entry.Reason = fmt.Sprintf("seal mutated contract: %v", sealErr)
			report.Summary.Invalid++
			report.Entries = append(report.Entries, entry)
			continue
		}
		entry.MutatedContractSHA256 = mutated.SHA256
		mutatedOracle, generateErr := oracle.Generate(mutated, originalOracle.PolicySHA256)
		if generateErr != nil {
			entry.Reason = fmt.Sprintf("generate mutated oracle: %v", generateErr)
			report.Summary.Invalid++
			report.Entries = append(report.Entries, entry)
			continue
		}
		mutatedOracleHash, hashErr := oracle.Hash(mutatedOracle)
		if hashErr != nil {
			entry.Reason = fmt.Sprintf("hash mutated oracle: %v", hashErr)
			report.Summary.Invalid++
			report.Entries = append(report.Entries, entry)
			continue
		}
		entry.MutatedOracleSHA256 = mutatedOracleHash
		entry.ChangedRuleIDs, entry.RemovedRuleIDs, entry.AddedRuleIDs = compareCases(originalOracle.Cases, mutatedOracle.Cases)
		if len(entry.ChangedRuleIDs) == 0 && len(entry.RemovedRuleIDs) == 0 && len(entry.AddedRuleIDs) == 0 {
			entry.Outcome = "equivalent"
			entry.Reason = "regenerated oracle cases are unchanged by rule ID"
			report.Summary.Equivalent++
		} else {
			entry.Outcome = "visible"
			entry.Reason = "regenerated oracle cases changed"
			report.Summary.Visible++
		}
		report.Entries = append(report.Entries, entry)
	}
	if report.Summary.Analyzed == 0 {
		return ContractMutationReport{}, fmt.Errorf("catalogue contains no contract-plane mutations")
	}
	return report, nil
}

// WriteContractMutationReport writes canonical JSON and refuses to overwrite
// an existing report so an old analysis cannot be mistaken for a fresh one.
func WriteContractMutationReport(path string, report ContractMutationReport) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("contract mutation report path must not be empty")
	}
	contents, err := CanonicalContractMutationReport(report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// LoadContractMutationReport loads a canonical contract-mutation report. The
// canonical-byte check prevents a CI consumer from silently reviewing a
// report different from the bytes that were produced by inspection.
func LoadContractMutationReport(path string) (ContractMutationReport, error) {
	if strings.TrimSpace(path) == "" {
		return ContractMutationReport{}, fmt.Errorf("contract mutation report path must not be empty")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ContractMutationReport{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var report ContractMutationReport
	if err := decoder.Decode(&report); err != nil {
		return ContractMutationReport{}, fmt.Errorf("parse contract mutation report %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ContractMutationReport{}, fmt.Errorf("parse contract mutation report %s: multiple JSON values are not supported", path)
		}
		return ContractMutationReport{}, fmt.Errorf("parse contract mutation report %s: %w", path, err)
	}
	canonical, err := CanonicalContractMutationReport(report)
	if err != nil {
		return ContractMutationReport{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return ContractMutationReport{}, fmt.Errorf("contract mutation report %s is not canonical JSON", path)
	}
	return report, nil
}

// CanonicalContractMutationReport encodes a structurally complete report.
func CanonicalContractMutationReport(report ContractMutationReport) ([]byte, error) {
	if report.Schema != ContractMutationResultSchema {
		return nil, fmt.Errorf("contract mutation report schema must be %s", ContractMutationResultSchema)
	}
	if report.Status != "completed" {
		return nil, fmt.Errorf("contract mutation report status must be completed")
	}
	if strings.TrimSpace(report.Contract.ID) == "" || report.Contract.Version < 1 || !digest(report.Contract.SHA256) {
		return nil, fmt.Errorf("contract mutation report contract reference is invalid")
	}
	if strings.TrimSpace(report.Oracle.Schema) == "" || !digest(report.Oracle.SHA256) || !digest(report.Oracle.PolicySHA256) {
		return nil, fmt.Errorf("contract mutation report oracle reference is invalid")
	}
	if len(report.Entries) == 0 {
		return nil, fmt.Errorf("contract mutation report entries must not be empty")
	}
	if report.Summary.Analyzed < 0 || report.Summary.Visible < 0 || report.Summary.Equivalent < 0 || report.Summary.Invalid < 0 {
		return nil, fmt.Errorf("contract mutation report summary counts must not be negative")
	}
	if report.Summary.Analyzed != len(report.Entries) || report.Summary.Visible+report.Summary.Equivalent+report.Summary.Invalid != report.Summary.Analyzed {
		return nil, fmt.Errorf("contract mutation report summary does not match entries")
	}
	seen := make(map[string]bool, len(report.Entries))
	for index, entry := range report.Entries {
		if strings.TrimSpace(entry.ID) == "" {
			return nil, fmt.Errorf("contract mutation report entries[%d].id must be non-empty", index)
		}
		if seen[entry.ID] {
			return nil, fmt.Errorf("contract mutation report entries[%d].id duplicates %q", index, entry.ID)
		}
		seen[entry.ID] = true
		if entry.Plane != "contract" {
			return nil, fmt.Errorf("contract mutation report entries[%d].plane must be contract", index)
		}
		switch entry.Outcome {
		case "visible", "equivalent":
			if !digest(entry.MutatedContractSHA256) || !digest(entry.MutatedOracleSHA256) {
				return nil, fmt.Errorf("contract mutation report entries[%d] must include mutated contract and oracle hashes", index)
			}
		case "invalid":
		default:
			return nil, fmt.Errorf("contract mutation report entries[%d].outcome is invalid", index)
		}
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return nil, fmt.Errorf("encode contract mutation report: %w", err)
	}
	return buffer.Bytes(), nil
}

func cloneDocument(document contract.Document) (contract.Document, error) {
	contents, err := json.Marshal(map[string]any{"contract": document.Contract})
	if err != nil {
		return contract.Document{}, fmt.Errorf("copy contract for mutation: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return contract.Document{}, fmt.Errorf("copy contract for mutation: %w", err)
	}
	body, ok := root["contract"].(map[string]any)
	if !ok {
		return contract.Document{}, fmt.Errorf("copy contract for mutation: contract body is not an object")
	}
	return contract.Document{Contract: body}, nil
}

func findTargetRule(rules []any, target string) (int, map[string]any, error) {
	if !strings.HasPrefix(target, "rule:") || strings.TrimSpace(strings.TrimPrefix(target, "rule:")) == "" {
		return 0, nil, fmt.Errorf("target must use the form rule:<rule-id>")
	}
	wanted := strings.TrimPrefix(target, "rule:")
	found := -1
	var selected map[string]any
	for index, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			return 0, nil, fmt.Errorf("contract rule %d is not an object", index)
		}
		if ruleID(rule) != wanted {
			continue
		}
		if found >= 0 {
			return 0, nil, fmt.Errorf("target rule %q is ambiguous", wanted)
		}
		found = index
		selected = rule
	}
	if found < 0 {
		return 0, nil, fmt.Errorf("target rule %q was not found", wanted)
	}
	return found, selected, nil
}

func ruleID(rule map[string]any) string {
	id, _ := rule["id"].(string)
	return id
}

func requiredStringChange(spec Spec, key string) (string, error) {
	value, present := spec.Change[key]
	if !present {
		return "", fmt.Errorf("mutation %q change.%s is required", spec.ID, key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("mutation %q change.%s must be a non-empty string", spec.ID, key)
	}
	return text, nil
}

func requiredIntegerChange(spec Spec, key string) (int64, error) {
	value, present := spec.Change[key]
	if !present {
		return 0, fmt.Errorf("mutation %q change.%s is required", spec.ID, key)
	}
	converted, ok := integerValue(value)
	if !ok {
		return 0, fmt.Errorf("mutation %q change.%s must be an integer", spec.ID, key)
	}
	return converted, nil
}

func integerValue(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		return int64(value), uint64(value) <= uint64(^uint64(0)>>1)
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		return int64(value), value <= uint64(^uint64(0)>>1)
	case json.Number:
		var parsed int64
		_, err := fmt.Sscan(string(value), &parsed)
		return parsed, err == nil
	case float64:
		converted := int64(value)
		return converted, value == float64(converted)
	default:
		return 0, false
	}
}

func contractID(document contract.Document) string {
	id, _ := document.Contract["id"].(string)
	return id
}

func contractVersion(document contract.Document) int64 {
	version, _ := integerValue(document.Contract["version"])
	return version
}

func compareCases(original, mutated []oracle.Case) (changed, removed, added []string) {
	originalByRule := make(map[string]oracle.Case, len(original))
	mutatedByRule := make(map[string]oracle.Case, len(mutated))
	for _, item := range original {
		originalByRule[item.RuleID] = item
	}
	for _, item := range mutated {
		mutatedByRule[item.RuleID] = item
	}
	for ruleID, item := range originalByRule {
		other, ok := mutatedByRule[ruleID]
		if !ok {
			removed = append(removed, ruleID)
			continue
		}
		item.CaseID = ""
		other.CaseID = ""
		if !reflect.DeepEqual(item, other) {
			changed = append(changed, ruleID)
		}
	}
	for ruleID := range mutatedByRule {
		if _, ok := originalByRule[ruleID]; !ok {
			added = append(added, ruleID)
		}
	}
	for _, values := range [][]string{changed, removed, added} {
		sort.Strings(values)
	}
	return changed, removed, added
}

func digest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
