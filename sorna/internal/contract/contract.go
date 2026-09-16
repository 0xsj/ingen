// Package contract loads, validates, canonicalizes, and seals InGen contracts.
package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	allowedStatuses  = map[string]bool{"draft": true, "sealed": true, "superseded": true}
	allowedStrengths = map[string]bool{"must": true, "must_not": true, "may": true, "should": true, "unspecified": true}
	digestPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const Schema = "ingen.contract/v1"

// Document is the machine-readable contract document. The contract body is
// deliberately map-backed because rules contain extensible, nested predicates.
type Document struct {
	Contract map[string]any
}

// Sealed is the immutable artifact produced from a draft contract.
type Sealed struct {
	Document      Document
	CanonicalJSON []byte
	SHA256        string
}

// ValidationError contains every structural problem found in a document.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	var builder strings.Builder
	builder.WriteString("invalid contract:")
	for _, problem := range e.Problems {
		builder.WriteString("\n- ")
		builder.WriteString(problem)
	}
	return builder.String()
}

// LoadFile loads a JSON-compatible YAML document from a .yaml, .yml, or .json
// file and validates its structural contract shape.
func LoadFile(path string) (Document, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return Document{}, fmt.Errorf("contract file must use .json, .yaml, or .yml")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	value, err := decodeYAMLSubset(contents)
	if err != nil {
		return Document{}, fmt.Errorf("parse %s: %w", path, err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return Document{}, &ValidationError{Problems: []string{"document must be an object containing a contract"}}
	}
	body, ok := root["contract"].(map[string]any)
	if !ok {
		return Document{}, &ValidationError{Problems: []string{"document.contract must be an object"}}
	}
	document := Document{Contract: body}
	if problems := Validate(document); len(problems) > 0 {
		return Document{}, &ValidationError{Problems: problems}
	}
	return document, nil
}

// Validate returns all structural errors in the MVP contract shape.
func Validate(document Document) []string {
	problems := make([]string, 0)
	if document.Contract == nil {
		return []string{"document.contract must be an object"}
	}
	body := document.Contract

	for _, field := range []string{"schema", "id", "version", "status", "interface", "rules", "unspecified"} {
		if _, ok := body[field]; !ok {
			problems = append(problems, "contract."+field+" is required")
		}
	}

	if value, ok := body["schema"]; ok {
		schema, valid := value.(string)
		if !valid || schema != Schema {
			problems = append(problems, "contract.schema must be ingen.contract/v1")
		}
	}
	if value, ok := body["id"]; ok && !nonEmptyString(value) {
		problems = append(problems, "contract.id must be a non-empty string")
	}
	if value, ok := body["version"]; ok {
		version, valid := integer(value)
		if !valid || version < 1 {
			problems = append(problems, "contract.version must be a positive integer")
		}
	}
	if value, ok := body["status"]; ok {
		status, valid := value.(string)
		if !valid || !allowedStatuses[status] {
			problems = append(problems, "contract.status must be draft, sealed, or superseded")
		}
	}
	if value, ok := body["interface"]; ok {
		interfaceBody, valid := value.(map[string]any)
		if !valid {
			problems = append(problems, "contract.interface must be an object")
		} else if !nonEmptyString(interfaceBody["kind"]) {
			problems = append(problems, "contract.interface.kind must be a non-empty string")
		}
	}
	if value, ok := body["unspecified"]; ok {
		if _, valid := value.([]any); !valid {
			problems = append(problems, "contract.unspecified must be a list")
		}
	}

	rules, ok := body["rules"].([]any)
	if !ok {
		if _, present := body["rules"]; present {
			problems = append(problems, "contract.rules must be a list")
		}
	} else {
		seen := make(map[string]bool, len(rules))
		for index, value := range rules {
			path := fmt.Sprintf("contract.rules[%d]", index)
			rule, valid := value.(map[string]any)
			if !valid {
				problems = append(problems, path+" must be an object")
				continue
			}
			ruleID, validID := rule["id"].(string)
			if !validID || strings.TrimSpace(ruleID) == "" {
				problems = append(problems, path+".id must be a non-empty string")
			} else if seen[ruleID] {
				problems = append(problems, fmt.Sprintf("%s.id duplicates %q", path, ruleID))
			} else {
				seen[ruleID] = true
			}

			strength, validStrength := rule["strength"].(string)
			if !validStrength || !allowedStrengths[strength] {
				problems = append(problems, path+".strength is invalid")
			}
			if strength != "unspecified" && !nonEmptyString(rule["subject"]) {
				problems = append(problems, path+".subject is required for executable rules")
			}
			if given, present := rule["given"]; present {
				givenBody, validGiven := given.(map[string]any)
				if !validGiven {
					problems = append(problems, path+".given must be an object")
				} else {
					if state, hasState := givenBody["state"]; hasState {
						if !nonEmptyString(state) {
							problems = append(problems, path+".given.state must be a non-empty string")
						}
						if _, hasSetup := givenBody["setup"]; !hasSetup {
							problems = append(problems, path+".given.setup is required for stateful rules")
						}
					}
					if setup, hasSetup := givenBody["setup"]; hasSetup {
						validateSetup(setup, path+".given.setup", &problems)
					}
				}
			}
			if expect, present := rule["expect"]; present {
				validateExpectationShape(expect, path+".expect", &problems)
			}
			validateGeneratedValues(rule, path, &problems)
		}
	}

	if value, present := body["fixtures"]; present {
		fixtures, valid := value.([]any)
		if !valid {
			problems = append(problems, "contract.fixtures must be a list")
		} else {
			for index, value := range fixtures {
				path := fmt.Sprintf("contract.fixtures[%d]", index)
				fixture, valid := value.(map[string]any)
				if !valid {
					problems = append(problems, path+" must be an object")
					continue
				}
				if !nonEmptyString(fixture["path"]) && !nonEmptyString(fixture["id"]) {
					problems = append(problems, path+" needs a non-empty path or id")
				}
				if !nonEmptyString(fixture["purpose"]) {
					problems = append(problems, path+".purpose must be a non-empty string")
				}
				if digest, present := fixture["sha256"]; present {
					value, valid := digest.(string)
					if !valid || !digestPattern.MatchString(value) {
						problems = append(problems, path+".sha256 must be a lowercase SHA-256 hex digest")
					}
				}
			}
		}
	}

	return problems
}

// CanonicalJSON returns deterministic JSON bytes for a valid contract. Go's
// encoding/json sorts string map keys; the test suite makes that dependency
// visible and protects the artifact hash from map insertion order.
func CanonicalJSON(document Document) ([]byte, error) {
	if problems := Validate(document); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	root := map[string]any{"contract": document.Contract}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("canonicalize contract: %w", err)
	}
	return buffer.Bytes(), nil
}

// Seal changes only a draft document's status and returns the canonical bytes
// and digest. The input document is not mutated.
func Seal(document Document) (Sealed, error) {
	return seal(document, "")
}

// SealAt seals a contract while resolving relative fixture paths from baseDir.
// It is useful to an isolated child process that receives a contract path but
// must not write the ordinary local sealing artifacts.
func SealAt(document Document, baseDir string) (Sealed, error) {
	return seal(document, baseDir)
}

// SealFile loads a draft contract, attaches hashes for local fixtures, and
// writes canonical.json and hash.txt to outputDir.
func SealFile(path, outputDir string) (Sealed, error) {
	document, err := LoadFile(path)
	if err != nil {
		return Sealed{}, err
	}
	sealed, err := seal(document, filepath.Dir(path))
	if err != nil {
		return Sealed{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Sealed{}, err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "canonical.json"), sealed.CanonicalJSON, 0o644); err != nil {
		return Sealed{}, err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "hash.txt"), []byte(sealed.SHA256+"\n"), 0o644); err != nil {
		return Sealed{}, err
	}
	return sealed, nil
}

func seal(document Document, baseDir string) (Sealed, error) {
	if problems := Validate(document); len(problems) > 0 {
		return Sealed{}, &ValidationError{Problems: problems}
	}
	status, _ := document.Contract["status"].(string)
	if status != "draft" {
		return Sealed{}, &ValidationError{Problems: []string{fmt.Sprintf("only draft contracts can be sealed, got %q", status)}}
	}

	copyDocument, err := clone(document)
	if err != nil {
		return Sealed{}, err
	}
	copyDocument.Contract["status"] = "sealed"
	if err := attachFixtureHashes(copyDocument, baseDir); err != nil {
		return Sealed{}, err
	}
	canonical, err := CanonicalJSON(copyDocument)
	if err != nil {
		return Sealed{}, err
	}
	digest := sha256.Sum256(canonical)
	return Sealed{
		Document:      copyDocument,
		CanonicalJSON: canonical,
		SHA256:        hex.EncodeToString(digest[:]),
	}, nil
}

func attachFixtureHashes(document Document, baseDir string) error {
	value, present := document.Contract["fixtures"]
	if !present {
		return nil
	}
	fixtures, ok := value.([]any)
	if !ok {
		return &ValidationError{Problems: []string{"contract.fixtures must be a list"}}
	}
	problems := make([]string, 0)
	for index, value := range fixtures {
		fixture, ok := value.(map[string]any)
		if !ok {
			continue
		}
		path, hasPath := fixture["path"].(string)
		existing, hasDigest := fixture["sha256"].(string)
		if !hasDigest {
			if baseDir == "" || !hasPath || strings.TrimSpace(path) == "" {
				problems = append(problems, fmt.Sprintf("contract.fixtures[%d].sha256 is required before sealing", index))
				continue
			}
			pathDigest, err := fileSHA256(filepath.Join(baseDir, path))
			if err != nil {
				problems = append(problems, fmt.Sprintf("contract.fixtures[%d]: %v", index, err))
				continue
			}
			fixture["sha256"] = pathDigest
			continue
		}
		if hasPath && baseDir != "" {
			pathDigest, err := fileSHA256(filepath.Join(baseDir, path))
			if err != nil {
				problems = append(problems, fmt.Sprintf("contract.fixtures[%d]: %v", index, err))
			} else if pathDigest != existing {
				problems = append(problems, fmt.Sprintf("contract.fixtures[%d].sha256 does not match %s", index, path))
			}
		}
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read fixture %s: %w", path, err)
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), nil
}

func clone(document Document) (Document, error) {
	contents, err := json.Marshal(map[string]any{"contract": document.Contract})
	if err != nil {
		return Document{}, fmt.Errorf("copy contract: %w", err)
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return Document{}, fmt.Errorf("copy contract: %w", err)
	}
	body, ok := root["contract"].(map[string]any)
	if !ok {
		return Document{}, errors.New("copy contract: contract body is not an object")
	}
	return Document{Contract: body}, nil
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func integer(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case json.Number:
		parsed, err := strconv.ParseInt(string(value), 10, 64)
		return parsed, err == nil
	case float64:
		parsed := int64(value)
		return parsed, value == float64(parsed)
	default:
		return 0, false
	}
}

func validateGeneratedValues(value any, path string, problems *[]string) {
	switch value := value.(type) {
	case map[string]any:
		if generated, present := value["generated"]; present {
			generator, ok := generated.(map[string]any)
			if !ok {
				*problems = append(*problems, path+".generated must be an object")
			} else {
				kind, ok := generator["kind"].(string)
				if !ok || kind != "repeat" {
					*problems = append(*problems, path+".generated.kind must be repeat in the MVP")
				}
				if _, present := generator["value"]; !present {
					*problems = append(*problems, path+".generated.value is required")
				}
				count, valid := integer(generator["count"])
				if !valid || count < 1 {
					*problems = append(*problems, path+".generated.count must be a positive integer")
				}
			}
		}
		for key, child := range value {
			validateGeneratedValues(child, path+"."+key, problems)
		}
	case []any:
		for index, child := range value {
			validateGeneratedValues(child, fmt.Sprintf("%s[%d]", path, index), problems)
		}
	}
}

// Materialize expands the deterministic value generators allowed by the
// contract. The result is a new tree for maps and slices, so callers can
// safely use it as an execution input without mutating the sealed contract.
func Materialize(value any) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		if generated, present := value["generated"]; present {
			generator, ok := generated.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("generated value must be an object")
			}
			kind, _ := generator["kind"].(string)
			if kind != "repeat" {
				return nil, fmt.Errorf("unsupported generator kind %q", kind)
			}
			text, ok := generator["value"].(string)
			if !ok {
				return nil, fmt.Errorf("repeat generator value must be a string")
			}
			count, ok := integer(generator["count"])
			if !ok || count < 1 || count > 1_000_000 {
				return nil, fmt.Errorf("repeat generator count must be between 1 and 1000000")
			}
			return strings.Repeat(text, int(count)), nil
		}
		object := make(map[string]any, len(value))
		for key, child := range value {
			materialized, err := Materialize(child)
			if err != nil {
				return nil, err
			}
			object[key] = materialized
		}
		return object, nil
	case []any:
		sequence := make([]any, len(value))
		for index, child := range value {
			materialized, err := Materialize(child)
			if err != nil {
				return nil, err
			}
			sequence[index] = materialized
		}
		return sequence, nil
	default:
		return value, nil
	}
}

func validateSetup(value any, path string, problems *[]string) {
	steps, ok := value.([]any)
	if !ok || len(steps) == 0 {
		*problems = append(*problems, path+" must be a non-empty list")
		return
	}
	seen := make(map[string]bool, len(steps))
	for index, value := range steps {
		stepPath := fmt.Sprintf("%s[%d]", path, index)
		step, validStep := value.(map[string]any)
		if !validStep {
			*problems = append(*problems, stepPath+" must be an object")
			continue
		}
		stepID, validID := step["id"].(string)
		if !validID || strings.TrimSpace(stepID) == "" {
			*problems = append(*problems, stepPath+".id must be a non-empty string")
		} else if seen[stepID] {
			*problems = append(*problems, stepPath+".id duplicates "+fmt.Sprintf("%q", stepID))
		} else {
			seen[stepID] = true
		}

		request, validRequest := step["request"].(map[string]any)
		if !validRequest {
			*problems = append(*problems, stepPath+".request must be an object")
		} else {
			if !nonEmptyString(request["method"]) {
				*problems = append(*problems, stepPath+".request.method must be a non-empty string")
			}
			requestPath, validPath := request["path"].(string)
			if !validPath || !strings.HasPrefix(requestPath, "/") {
				*problems = append(*problems, stepPath+".request.path must start with /")
			}
		}
		if expect, validExpect := step["expect"].(map[string]any); !validExpect {
			*problems = append(*problems, stepPath+".expect must be an object")
		} else {
			validateExpectationShape(expect, stepPath+".expect", problems)
		}
		if capture, present := step["capture"]; present {
			captures, validCaptures := capture.(map[string]any)
			if !validCaptures {
				*problems = append(*problems, stepPath+".capture must be an object")
			} else {
				for name, selector := range captures {
					if strings.TrimSpace(name) == "" {
						*problems = append(*problems, stepPath+".capture names must be non-empty")
					}
					selectorText, validSelector := selector.(string)
					if !validSelector || !strings.HasPrefix(selectorText, "body.") {
						*problems = append(*problems, stepPath+".capture."+name+" must select a body field")
					}
				}
			}
		}
	}
}

func validateExpectationShape(value any, path string, problems *[]string) {
	expect, ok := value.(map[string]any)
	if !ok {
		*problems = append(*problems, path+" must be an object")
		return
	}
	if body, present := expect["body"]; present {
		validateShapeSpec(body, path+".body", problems)
	}
	if events, present := expect["events"]; present {
		validateEventSpec(events, path+".events", problems)
	}
}

func validateEventSpec(value any, path string, problems *[]string) {
	spec, ok := value.(map[string]any)
	if !ok {
		*problems = append(*problems, path+" must be an object")
		return
	}
	rawRequired, present := spec["required"]
	if !present {
		*problems = append(*problems, path+".required is required")
		return
	}
	required, ok := rawRequired.([]any)
	if !ok {
		*problems = append(*problems, path+".required must be a list")
		return
	}
	if len(required) == 0 {
		*problems = append(*problems, path+".required must contain at least one event")
	}
	for index, value := range required {
		if !nonEmptyString(value) {
			*problems = append(*problems, fmt.Sprintf("%s.required[%d] must be a non-empty string", path, index))
		}
	}
}

func validateShapeSpec(value any, path string, problems *[]string) {
	spec, ok := value.(map[string]any)
	if !ok {
		return
	}
	if additional, present := spec["additional_properties"]; present {
		if _, ok := additional.(bool); !ok {
			*problems = append(*problems, path+".additional_properties must be a boolean")
		}
	}
	properties, ok := spec["properties"].(map[string]any)
	if !ok {
		return
	}
	for field, child := range properties {
		validateShapeSpec(child, path+".properties."+field, problems)
	}
}

func decodeYAMLSubset(contents []byte) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple YAML documents are not supported")
		}
		return nil, err
	}
	if len(document.Content) != 1 {
		return nil, errors.New("document is empty")
	}
	return yamlValue(document.Content[0])
}

func yamlValue(node *yaml.Node) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		object := make(map[string]any, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, errors.New("mapping keys must be strings")
			}
			if _, exists := object[key.Value]; exists {
				return nil, fmt.Errorf("duplicate mapping key %q", key.Value)
			}
			value, err := yamlValue(node.Content[index+1])
			if err != nil {
				return nil, err
			}
			object[key.Value] = value
		}
		return object, nil
	case yaml.SequenceNode:
		sequence := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := yamlValue(child)
			if err != nil {
				return nil, err
			}
			sequence = append(sequence, value)
		}
		return sequence, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			return node.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool":
			value, err := strconv.ParseBool(node.Value)
			return value, err
		case "!!int":
			value, err := strconv.ParseInt(node.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("integer %q is not a decimal 64-bit integer", node.Value)
			}
			return value, nil
		case "!!float":
			value, err := strconv.ParseFloat(node.Value, 64)
			return value, err
		default:
			return nil, fmt.Errorf("YAML scalar type %s is not supported", node.Tag)
		}
	case yaml.AliasNode:
		return nil, errors.New("YAML aliases are not supported")
	default:
		return nil, fmt.Errorf("YAML node kind %d is not supported", node.Kind)
	}
}
