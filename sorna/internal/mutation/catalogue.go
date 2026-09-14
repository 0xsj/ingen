package mutation

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/sorna/internal/contract"
)

// Schema is the versioned file shape for a reviewable mutation catalogue.
const Schema = "ingen.mutation-catalogue/v1"

var allowedPlanes = map[string]bool{
	"implementation": true,
	"contract":       true,
}

var allowedStatuses = map[string]bool{
	"candidate":  true,
	"validated":  true,
	"executed":   true,
	"classified": true,
	"reported":   true,
}

// Catalogue is the declarative input to a future mutation campaign. It names
// the experiment before any subject process is launched.
type Catalogue struct {
	Schema          string `json:"schema" yaml:"schema"`
	ID              string `json:"id" yaml:"id"`
	Version         int64  `json:"version" yaml:"version"`
	ContractID      string `json:"contract_id" yaml:"contract_id"`
	ContractVersion int64  `json:"contract_version" yaml:"contract_version"`
	Mutations       []Spec `json:"mutations" yaml:"mutations"`
}

type catalogueDocument struct {
	Catalogue Catalogue `json:"mutation_catalogue" yaml:"mutation_catalogue"`
}

// ValidationError contains every structural catalogue problem found.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	var builder strings.Builder
	builder.WriteString("invalid mutation catalogue:")
	for _, problem := range e.Problems {
		builder.WriteString("\n- ")
		builder.WriteString(problem)
	}
	return builder.String()
}

// LoadFile loads a YAML or JSON mutation catalogue and validates its shape.
func LoadFile(path string) (Catalogue, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return Catalogue{}, fmt.Errorf("mutation catalogue must use .json, .yaml, or .yml")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Catalogue{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var document catalogueDocument
	if err := decoder.Decode(&document); err != nil {
		return Catalogue{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Catalogue{}, fmt.Errorf("parse %s: multiple documents are not supported", path)
		}
		return Catalogue{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if problems := Validate(document.Catalogue); len(problems) > 0 {
		return Catalogue{}, &ValidationError{Problems: problems}
	}
	return document.Catalogue, nil
}

// Validate returns all structural errors in the v1 mutation catalogue shape.
func Validate(catalogue Catalogue) []string {
	problems := make([]string, 0)
	if catalogue.Schema != Schema {
		problems = append(problems, fmt.Sprintf("mutation_catalogue.schema must be %s", Schema))
	}
	if strings.TrimSpace(catalogue.ID) == "" {
		problems = append(problems, "mutation_catalogue.id must be non-empty")
	}
	if catalogue.Version < 1 {
		problems = append(problems, "mutation_catalogue.version must be positive")
	}
	if strings.TrimSpace(catalogue.ContractID) == "" {
		problems = append(problems, "mutation_catalogue.contract_id must be non-empty")
	}
	if catalogue.ContractVersion < 1 {
		problems = append(problems, "mutation_catalogue.contract_version must be positive")
	}
	if len(catalogue.Mutations) == 0 {
		problems = append(problems, "mutation_catalogue.mutations must contain at least one mutation")
	}

	seen := make(map[string]bool, len(catalogue.Mutations))
	for index, spec := range catalogue.Mutations {
		path := fmt.Sprintf("mutation_catalogue.mutations[%d]", index)
		if strings.TrimSpace(spec.ID) == "" {
			problems = append(problems, path+".id must be non-empty")
		} else if seen[spec.ID] {
			problems = append(problems, fmt.Sprintf("%s.id duplicates %q", path, spec.ID))
		} else {
			seen[spec.ID] = true
		}
		if !allowedPlanes[spec.Plane] {
			problems = append(problems, path+".plane must be implementation or contract")
		}
		if strings.TrimSpace(spec.Operator) == "" {
			problems = append(problems, path+".operator must be non-empty")
		}
		if strings.TrimSpace(spec.Target) == "" {
			problems = append(problems, path+".target must be non-empty")
		}
		if strings.TrimSpace(spec.Description) == "" {
			problems = append(problems, path+".description must be non-empty")
		}
		if len(spec.Change) == 0 {
			problems = append(problems, path+".change must contain a reproducible change")
		}
		if len(spec.ExpectedRuleIDs) == 0 {
			problems = append(problems, path+".expected_rule_ids must contain at least one rule ID")
		}
		seenRules := make(map[string]bool, len(spec.ExpectedRuleIDs))
		for ruleIndex, ruleID := range spec.ExpectedRuleIDs {
			rulePath := fmt.Sprintf("%s.expected_rule_ids[%d]", path, ruleIndex)
			if strings.TrimSpace(ruleID) == "" {
				problems = append(problems, rulePath+" must be non-empty")
			} else if seenRules[ruleID] {
				problems = append(problems, fmt.Sprintf("%s duplicates %q", rulePath, ruleID))
			} else {
				seenRules[ruleID] = true
			}
		}
		if !allowedStatuses[spec.Status] {
			problems = append(problems, path+".status is invalid")
		}
	}
	return problems
}

// ValidateAgainstContract checks the catalogue's declared contract binding
// and every expected rule ID against a validated contract document.
func ValidateAgainstContract(catalogue Catalogue, document contract.Document) []string {
	problems := Validate(catalogue)
	contractID, _ := document.Contract["id"].(string)
	if catalogue.ContractID != contractID {
		problems = append(problems, fmt.Sprintf("mutation_catalogue.contract_id %q does not match contract.id %q", catalogue.ContractID, contractID))
	}
	contractVersion, ok := contractInteger(document.Contract["version"])
	if !ok || catalogue.ContractVersion != contractVersion {
		problems = append(problems, fmt.Sprintf("mutation_catalogue.contract_version %d does not match contract.version %d", catalogue.ContractVersion, contractVersion))
	}

	ruleIDs := make(map[string]bool)
	if rules, ok := document.Contract["rules"].([]any); ok {
		for _, rawRule := range rules {
			if rule, ok := rawRule.(map[string]any); ok {
				if ruleID, ok := rule["id"].(string); ok {
					ruleIDs[ruleID] = true
				}
			}
		}
	}
	for index, spec := range catalogue.Mutations {
		for ruleIndex, ruleID := range spec.ExpectedRuleIDs {
			if !ruleIDs[ruleID] {
				problems = append(problems, fmt.Sprintf("mutation_catalogue.mutations[%d].expected_rule_ids[%d] %q is not in contract.rules", index, ruleIndex, ruleID))
			}
		}
	}
	return problems
}

func contractInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		converted := int64(value)
		return converted, float64(converted) == value
	default:
		return 0, false
	}
}

// Find returns the named mutation without changing the catalogue.
func (catalogue Catalogue) Find(id string) (Spec, bool) {
	for _, spec := range catalogue.Mutations {
		if spec.ID == id {
			return spec, true
		}
	}
	return Spec{}, false
}
