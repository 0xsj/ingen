package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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

type Source struct {
	Language string   `yaml:"language" json:"language"`
	Roots    []string `yaml:"roots" json:"roots"`
	Unit     string   `yaml:"unit" json:"unit"`
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
	if p.Schema != "paddock.architecture/v1" {
		return fmt.Errorf("policy schema must be paddock.architecture/v1, got %q", p.Schema)
	}
	if p.Source.Language == "" {
		return fmt.Errorf("source.language is required")
	}
	if p.Source.Language != "go" && p.Source.Language != "typescript" && p.Source.Language != "python" {
		return fmt.Errorf("source.language %q is not supported yet", p.Source.Language)
	}
	if p.Source.Unit != "package" && p.Source.Unit != "file" {
		return fmt.Errorf("source.unit must be package or file, got %q", p.Source.Unit)
	}
	if p.Source.Language == "go" && p.Source.Unit != "package" {
		return fmt.Errorf("Go source.unit must be package")
	}
	if (p.Source.Language == "typescript" || p.Source.Language == "python") && p.Source.Unit != "file" {
		return fmt.Errorf("%s source.unit must be file", p.Source.Language)
	}
	if len(p.Source.Roots) == 0 {
		return fmt.Errorf("source.roots must not be empty")
	}
	if len(p.Components) == 0 {
		return fmt.Errorf("components must not be empty")
	}
	seen := map[string]bool{}
	for _, rule := range p.Rules {
		if rule.ID == "" {
			return fmt.Errorf("every rule needs an id")
		}
		if seen[rule.ID] {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = true
		switch rule.Severity {
		case "error", "warning", "info":
		default:
			return fmt.Errorf("rule %q has unsupported severity %q", rule.ID, rule.Severity)
		}
		switch rule.Kind {
		case "allow-dependencies", "deny-dependencies", "layer-direction", "no-cross-context", "mediated-dependency", "public-api-only", "no-cycles", "coverage", "required-dependency", "unresolved":
		default:
			return fmt.Errorf("rule %q has unsupported kind %q", rule.ID, rule.Kind)
		}
		if rule.Kind == "layer-direction" && rule.Direction != "toward-lower-layer" && rule.Direction != "toward-higher-layer" {
			return fmt.Errorf("rule %q has invalid direction %q", rule.ID, rule.Direction)
		}
		if (rule.Kind == "allow-dependencies" || rule.Kind == "required-dependency") && len(rule.Allow) == 0 {
			return fmt.Errorf("rule %q needs allow targets", rule.ID)
		}
		if rule.Transitive && rule.Kind != "required-dependency" {
			return fmt.Errorf("rule %q may use transitive only with required-dependency", rule.ID)
		}
	}
	for _, waiver := range p.Waivers {
		if waiver.Rule == "" {
			return fmt.Errorf("every waiver needs a rule")
		}
		if !seen[waiver.Rule] {
			return fmt.Errorf("waiver references unknown rule %q", waiver.Rule)
		}
		if strings.TrimSpace(waiver.From) == "" {
			return fmt.Errorf("waiver for rule %q needs from", waiver.Rule)
		}
		if strings.TrimSpace(waiver.Reason) == "" {
			return fmt.Errorf("waiver for rule %q needs a reason", waiver.Rule)
		}
		if strings.TrimSpace(waiver.Owner) == "" {
			return fmt.Errorf("waiver for rule %q needs an owner", waiver.Rule)
		}
		expires, err := time.Parse("2006-01-02", waiver.Expires)
		if err != nil || expires.Format("2006-01-02") != waiver.Expires {
			return fmt.Errorf("waiver for rule %q has invalid expires date %q; use YYYY-MM-DD", waiver.Rule, waiver.Expires)
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
