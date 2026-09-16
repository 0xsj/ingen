package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Policy struct {
	Schema     string               `yaml:"schema" json:"schema"`
	Project    string               `yaml:"project" json:"project"`
	Source     Source               `yaml:"source" json:"source"`
	Components map[string]Component `yaml:"components" json:"components"`
	Rules      []Rule               `yaml:"rules" json:"rules"`
	Waivers    []Waiver             `yaml:"waivers" json:"waivers"`
}

const ValidationSchema = "paddock.policy-validation/v1"

// ValidationDocument is emitted by policy validate when a policy cannot be
// loaded. The normalized policy remains the successful JSON output; this
// separate schema keeps invalid-policy diagnostics machine-readable without
// changing that existing contract.
type ValidationDocument struct {
	Schema    string            `json:"schema"`
	Policy    string            `json:"policy"`
	Operation string            `json:"operation,omitempty"`
	Before    string            `json:"before,omitempty"`
	After     string            `json:"after,omitempty"`
	Valid     bool              `json:"valid"`
	Errors    []ValidationIssue `json:"errors"`
}

type ValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "policy validation failed"
	}
	messages := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		messages = append(messages, issue.Message)
	}
	return strings.Join(messages, "; ")
}

func ValidationDocumentForError(policyPath string, err error) ValidationDocument {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		issues := append([]ValidationIssue(nil), validationErr.Issues...)
		return ValidationDocument{
			Schema: ValidationSchema,
			Policy: policyPath,
			Valid:  false,
			Errors: issues,
		}
	}
	code := "invalid-policy"
	message := err.Error()
	if strings.HasPrefix(message, "read policy:") {
		code = "read-policy"
	} else if strings.HasPrefix(message, "parse policy:") || strings.HasPrefix(message, "parse policy JSON:") {
		code = "parse-policy"
	}
	return ValidationDocument{
		Schema: ValidationSchema,
		Policy: policyPath,
		Valid:  false,
		Errors: []ValidationIssue{{Code: code, Message: message}},
	}
}

func ValidationDocumentForComparison(operation, policyPath, beforePath, afterPath string, err error) ValidationDocument {
	document := ValidationDocumentForError(policyPath, err)
	document.Operation = operation
	document.Before = beforePath
	document.After = afterPath
	return document
}

type Source struct {
	Language string   `yaml:"language" json:"language"`
	Roots    []string `yaml:"roots" json:"roots"`
	Unit     string   `yaml:"unit" json:"unit"`
	Include  []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

type Component struct {
	Match  Patterns       `yaml:"match" json:"match"`
	Labels map[string]any `yaml:"labels" json:"labels"`
}

type Rule struct {
	ID           string    `yaml:"id" json:"id"`
	Kind         string    `yaml:"kind" json:"kind"`
	Severity     string    `yaml:"severity" json:"severity"`
	From         Selectors `yaml:"from" json:"from"`
	To           Selectors `yaml:"to" json:"to"`
	Allow        Targets   `yaml:"allow" json:"allow"`
	Deny         Targets   `yaml:"deny" json:"deny"`
	AllowTo      Targets   `yaml:"allow-to" json:"allow_to"`
	Transitive   bool      `yaml:"transitive" json:"transitive"`
	Direction    string    `yaml:"direction" json:"direction"`
	ContextLabel string    `yaml:"context-label" json:"context_label"`
	Message      string    `yaml:"message" json:"message"`
}

type Waiver struct {
	Rule    string `yaml:"rule" json:"rule"`
	From    string `yaml:"from" json:"from"`
	To      string `yaml:"to" json:"to"`
	Reason  string `yaml:"reason" json:"reason"`
	Owner   string `yaml:"owner" json:"owner"`
	Expires string `yaml:"expires" json:"expires"`
}

// Patterns accepts either match: path/** or a list of path patterns.
type Patterns []string

func (p *Patterns) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var value string
		if err := node.Decode(&value); err != nil {
			return err
		}
		*p = []string{value}
		return nil
	case yaml.SequenceNode:
		var values []string
		if err := node.Decode(&values); err != nil {
			return err
		}
		*p = values
		return nil
	default:
		return fmt.Errorf("match must be a string or list of strings")
	}
}

// Selector is a set of labels. The usual keys are role, context, and feature.
type Selector map[string]string
type Selectors []Selector

func (s *Selectors) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		var value map[string]string
		if err := node.Decode(&value); err != nil {
			return err
		}
		*s = []Selector{value}
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		var values []map[string]string
		if err := node.Decode(&values); err != nil {
			return err
		}
		*s = make([]Selector, 0, len(values))
		for _, value := range values {
			*s = append(*s, value)
		}
		return nil
	}
	return fmt.Errorf("selector must be a mapping or list of mappings")
}

type Target struct {
	Literal string            `json:"literal,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	Kind    string            `json:"kind,omitempty"`
	Value   string            `json:"value,omitempty"`
}

type Targets []Target

func (t *Targets) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var value string
		if err := node.Decode(&value); err != nil {
			return err
		}
		*t = []Target{{Literal: value}}
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("target must be a string or list")
	}

	var values []yaml.Node
	if err := node.Decode(&values); err != nil {
		return err
	}
	result := make([]Target, 0, len(values))
	for _, value := range values {
		switch value.Kind {
		case yaml.ScalarNode:
			var literal string
			if err := value.Decode(&literal); err != nil {
				return err
			}
			result = append(result, Target{Literal: literal})
		case yaml.MappingNode:
			var labels map[string]string
			if err := value.Decode(&labels); err != nil {
				return err
			}
			for key, item := range labels {
				if key == "standard-library" || key == "external" {
					result = append(result, Target{Kind: key, Value: item})
					labels = nil
					break
				}
			}
			if labels != nil {
				result = append(result, Target{Labels: labels})
			}
		default:
			return fmt.Errorf("target entries must be strings or mappings")
		}
	}
	*t = result
	return nil
}

func Load(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy: %w", err)
	}
	var result Policy
	if err := yaml.Unmarshal(data, &result); err != nil {
		return Policy{}, fmt.Errorf("parse policy: %w", err)
	}
	result.Normalize()
	if err := result.Validate(); err != nil {
		return Policy{}, err
	}
	return result, nil
}

func ParseJSON(data []byte) (Policy, error) {
	var result Policy
	if err := json.Unmarshal(data, &result); err != nil {
		return Policy{}, fmt.Errorf("parse policy JSON: %w", err)
	}
	result.Normalize()
	if err := result.Validate(); err != nil {
		return Policy{}, err
	}
	return result, nil
}

func CanonicalJSON(input Policy) ([]byte, error) {
	input.Source.Roots = append([]string(nil), input.Source.Roots...)
	sort.Strings(input.Source.Roots)
	input.Source.Include = append([]string(nil), input.Source.Include...)
	sort.Strings(input.Source.Include)
	input.Source.Exclude = append([]string(nil), input.Source.Exclude...)
	sort.Strings(input.Source.Exclude)
	input.Rules = append([]Rule(nil), input.Rules...)
	sort.SliceStable(input.Rules, func(i, j int) bool {
		return input.Rules[i].ID < input.Rules[j].ID
	})
	if input.Rules == nil {
		input.Rules = []Rule{}
	}
	input.Waivers = append([]Waiver(nil), input.Waivers...)
	if input.Waivers == nil {
		input.Waivers = []Waiver{}
	}
	input.Normalize()
	return json.Marshal(input)
}

func CanonicalSHA256(input Policy) (string, error) {
	data, err := CanonicalJSON(input)
	if err != nil {
		return "", fmt.Errorf("canonicalize policy: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (p *Policy) Normalize() {
	if p.Source.Unit == "" {
		if p.Source.Language == "go" {
			p.Source.Unit = "package"
		} else if p.Source.Language == "typescript" || p.Source.Language == "python" {
			p.Source.Unit = "file"
		}
	}
	for index := range p.Rules {
		if p.Rules[index].Severity == "" {
			p.Rules[index].Severity = "error"
		}
	}
}

func (p Policy) Validate() error {
	issues := p.ValidationIssues()
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func (p Policy) ValidationIssues() []ValidationIssue {
	issues := []ValidationIssue{}
	add := func(code, path, message string) {
		issues = append(issues, ValidationIssue{Code: code, Path: path, Message: message})
	}
	if p.Schema != "paddock.architecture/v1" {
		add("schema", "schema", fmt.Sprintf("policy schema must be paddock.architecture/v1, got %q", p.Schema))
	}
	if p.Source.Language == "" {
		add("source-language", "source.language", "source.language is required")
	}
	if p.Source.Unit != "package" && p.Source.Unit != "file" {
		add("source-unit", "source.unit", fmt.Sprintf("source.unit must be package or file, got %q", p.Source.Unit))
	}
	if p.Source.Language == "go" && p.Source.Unit != "package" {
		add("source-unit", "source.unit", "Go source.unit must be package")
	}
	if (p.Source.Language == "typescript" || p.Source.Language == "python") && p.Source.Unit != "file" {
		add("source-unit", "source.unit", fmt.Sprintf("%s source.unit must be file", p.Source.Language))
	}
	if len(p.Source.Roots) == 0 {
		add("source-roots", "source.roots", "source.roots must not be empty")
	}
	validateSourcePatterns(&issues, "include", p.Source.Include)
	validateSourcePatterns(&issues, "exclude", p.Source.Exclude)
	if len(p.Components) == 0 {
		add("components", "components", "components must not be empty")
	}
	seen := map[string]bool{}
	for index, rule := range p.Rules {
		rulePath := fmt.Sprintf("rules[%d]", index)
		if rule.ID != "" {
			rulePath = "rules." + rule.ID
		}
		if rule.ID == "" {
			add("rule-id", rulePath, "every rule needs an id")
		}
		if rule.ID != "" && seen[rule.ID] {
			add("rule-id", rulePath, fmt.Sprintf("duplicate rule id %q", rule.ID))
		}
		if rule.ID != "" {
			seen[rule.ID] = true
		}
		switch rule.Severity {
		case "error", "warning", "info":
		default:
			add("rule-severity", rulePath+".severity", fmt.Sprintf("rule %q has unsupported severity %q", rule.ID, rule.Severity))
		}
		switch rule.Kind {
		case "allow-dependencies", "deny-dependencies", "layer-direction", "no-cross-context", "mediated-dependency", "public-api-only", "no-cycles", "coverage", "required-dependency", "component-owns", "unresolved":
		default:
			add("rule-kind", rulePath+".kind", fmt.Sprintf("rule %q has unsupported kind %q", rule.ID, rule.Kind))
			continue
		}
		if err := validateRuleSemantics(rule); err != nil {
			add("rule-semantics", rulePath, err.Error())
		}
	}
	for index, waiver := range p.Waivers {
		waiverPath := fmt.Sprintf("waivers[%d]", index)
		if waiver.Rule == "" {
			add("waiver-rule", waiverPath+".rule", "every waiver needs a rule")
		}
		if waiver.Rule != "" && !seen[waiver.Rule] {
			add("waiver-rule", waiverPath+".rule", fmt.Sprintf("waiver references unknown rule %q", waiver.Rule))
		}
		if strings.TrimSpace(waiver.From) == "" {
			add("waiver-from", waiverPath+".from", fmt.Sprintf("waiver for rule %q needs from", waiver.Rule))
		}
		if strings.TrimSpace(waiver.Reason) == "" {
			add("waiver-reason", waiverPath+".reason", fmt.Sprintf("waiver for rule %q needs a reason", waiver.Rule))
		}
		if strings.TrimSpace(waiver.Owner) == "" {
			add("waiver-owner", waiverPath+".owner", fmt.Sprintf("waiver for rule %q needs an owner", waiver.Rule))
		}
		expires, err := time.Parse("2006-01-02", waiver.Expires)
		if err != nil || expires.Format("2006-01-02") != waiver.Expires {
			add("waiver-expires", waiverPath+".expires", fmt.Sprintf("waiver for rule %q has invalid expires date %q; use YYYY-MM-DD", waiver.Rule, waiver.Expires))
		}
	}
	return issues
}

func validateSourcePatterns(issues *[]ValidationIssue, kind string, patterns []string) {
	for index, pattern := range patterns {
		field := fmt.Sprintf("source.%s[%d]", kind, index)
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			*issues = append(*issues, ValidationIssue{Code: "source-scan-pattern", Path: field, Message: fmt.Sprintf("%s must not be empty", field)})
			continue
		}
		if strings.HasPrefix(pattern, "/") || pattern == ".." || strings.HasPrefix(pattern, "../") || strings.Contains(pattern, "/../") {
			*issues = append(*issues, ValidationIssue{Code: "source-scan-pattern", Path: field, Message: fmt.Sprintf("%s must be a relative path pattern", field)})
			continue
		}
		for _, segment := range strings.Split(strings.Trim(pattern, "/"), "/") {
			if segment == "" || segment == "**" {
				continue
			}
			if _, err := path.Match(segment, ""); err != nil {
				*issues = append(*issues, ValidationIssue{Code: "source-scan-pattern", Path: field, Message: fmt.Sprintf("%s has invalid pattern %q: %v", field, pattern, err)})
				break
			}
		}
	}
}

type ruleField struct {
	name string
	set  bool
}

func validateRuleSemantics(rule Rule) error {
	if rule.Transitive && rule.Kind != "required-dependency" {
		return fmt.Errorf("rule %q may use transitive only with required-dependency", rule.ID)
	}

	switch rule.Kind {
	case "allow-dependencies":
		if len(rule.Allow) == 0 {
			return fmt.Errorf("rule %q needs allow targets", rule.ID)
		}
		return rejectRuleFields(rule,
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "deny-dependencies":
		if len(rule.Deny) == 0 {
			return fmt.Errorf("rule %q needs deny targets", rule.ID)
		}
		return rejectRuleFields(rule,
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "layer-direction":
		if rule.Direction == "" {
			return fmt.Errorf("rule %q needs direction", rule.ID)
		}
		if rule.Direction != "toward-lower-layer" && rule.Direction != "toward-higher-layer" {
			return fmt.Errorf("rule %q has invalid direction %q", rule.ID, rule.Direction)
		}
		return rejectRuleFields(rule,
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "no-cross-context":
		return rejectRuleFields(rule,
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
		)
	case "mediated-dependency":
		if len(rule.AllowTo) == 0 {
			return fmt.Errorf("rule %q needs allow-to targets", rule.ID)
		}
		return rejectRuleFields(rule,
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
		)
	case "public-api-only":
		return rejectRuleFields(rule,
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
		)
	case "no-cycles":
		return rejectRuleFields(rule,
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "coverage":
		return rejectRuleFields(rule,
			ruleField{name: "from", set: len(rule.From) > 0},
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "required-dependency":
		if len(rule.Allow) == 0 {
			return fmt.Errorf("rule %q needs allow targets", rule.ID)
		}
		return rejectRuleFields(rule,
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "component-owns":
		if len(rule.Allow) == 0 {
			return fmt.Errorf("rule %q needs allow targets", rule.ID)
		}
		return rejectRuleFields(rule,
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	case "unresolved":
		return rejectRuleFields(rule,
			ruleField{name: "to", set: len(rule.To) > 0},
			ruleField{name: "allow", set: len(rule.Allow) > 0},
			ruleField{name: "deny", set: len(rule.Deny) > 0},
			ruleField{name: "allow-to", set: len(rule.AllowTo) > 0},
			ruleField{name: "transitive", set: rule.Transitive},
			ruleField{name: "direction", set: rule.Direction != ""},
			ruleField{name: "context-label", set: rule.ContextLabel != ""},
		)
	}
	return nil
}

func rejectRuleFields(rule Rule, fields ...ruleField) error {
	for _, field := range fields {
		if field.set {
			return fmt.Errorf("rule %q kind %q cannot use %s", rule.ID, rule.Kind, field.name)
		}
	}
	return nil
}

func (t Target) String() string {
	if t.Literal != "" {
		return t.Literal
	}
	if t.Kind != "" {
		return t.Kind + ":" + t.Value
	}
	return fmt.Sprint(t.Labels)
}

func LabelString(labels map[string]any, key string) string {
	value, ok := labels[key]
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}

func Layer(labels map[string]string) (int, bool) {
	value, ok := labels["layer"]
	if !ok {
		return 0, false
	}
	layer, err := strconv.Atoi(value)
	return layer, err == nil
}

func Substitute(value string, captures map[string]string) string {
	for key, item := range captures {
		value = strings.ReplaceAll(value, "{"+key+"}", item)
	}
	return value
}
