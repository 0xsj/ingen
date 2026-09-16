// Package malcolm translates the first Malcolm IR subset into a Sorna draft
// contract. It is deliberately a rejecting adapter: unsupported Malcolm
// meaning must not become an empty or weaker Sorna expectation.
package malcolm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"ingen/sorna/internal/contract"
)

const (
	IRSchema      = "malcolm.ir/v1"
	SornaSchema   = "ingen.contract/v1"
	SornaStatus   = "draft"
	SornaHTTPJSON = "http-json"
)

// IR is the JSON representation emitted by Malcolm's Rust compiler.
type IR struct {
	Schema        string        `json:"schema"`
	Specification Specification `json:"specification"`
}

type Specification struct {
	Name      string     `json:"name"`
	Version   *string    `json:"version"`
	Subject   *string    `json:"subject"`
	Scenarios []Scenario `json:"scenarios"`
}

type Scenario struct {
	Name         string        `json:"name"`
	Given        []string      `json:"given"`
	State        *string       `json:"state"`
	Request      *Request      `json:"request"`
	When         When          `json:"when"`
	Setups       []Setup       `json:"setups"`
	Requirements []Requirement `json:"requirements"`
}

type Request struct {
	Body map[string]any `json:"body"`
}

type Setup struct {
	Name         string        `json:"name"`
	Request      *Request      `json:"request"`
	When         When          `json:"when"`
	Requirements []Requirement `json:"requirements"`
	Captures     []Capture     `json:"captures"`
}

type Capture struct {
	Name     string `json:"name"`
	Selector string `json:"selector"`
}

type When struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type Requirement struct {
	Kind       string `json:"kind"`
	Expression string `json:"expression"`
}

var (
	statusExpression = regexp.MustCompile("^response\\.status\\s*==\\s*([0-9]+)$")
	bodyExists       = regexp.MustCompile("^response\\.body\\.([A-Za-z_][A-Za-z0-9_]*)\\s+exists$")
	bodyEquals       = regexp.MustCompile("^response\\.body\\.([A-Za-z_][A-Za-z0-9_]*)\\s*==\\s*(.+)$")
	eventEmit        = regexp.MustCompile("^emit\\s+\"([A-Za-z_][A-Za-z0-9_.:-]*)\"$")
	eventOrder       = regexp.MustCompile("^emit\\s+in\\s+order\\s+\\[(.*)\\]$")
	eventName        = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_.:-]*$")
	bodySelector     = regexp.MustCompile("^body\\.([A-Za-z_][A-Za-z0-9_]*)$")
)

// LoadFile reads and validates one Malcolm IR JSON artifact.
func LoadFile(path string) (IR, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return IR{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	var ir IR
	if err := decoder.Decode(&ir); err != nil {
		return IR{}, fmt.Errorf("parse Malcolm IR %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return IR{}, fmt.Errorf("parse Malcolm IR %s: multiple JSON values are not supported", path)
		}
		return IR{}, fmt.Errorf("parse Malcolm IR %s: %w", path, err)
	}
	if problems := ValidateIR(ir); len(problems) > 0 {
		return IR{}, fmt.Errorf("invalid Malcolm IR: %s", strings.Join(problems, "; "))
	}
	return ir, nil
}

// ValidateIR checks the structural values the Sorna adapter needs.
func ValidateIR(ir IR) []string {
	problems := make([]string, 0)
	if ir.Schema != IRSchema {
		problems = append(problems, "schema must be "+IRSchema)
	}
	if strings.TrimSpace(ir.Specification.Name) == "" {
		problems = append(problems, "specification.name must be non-empty")
	}
	if ir.Specification.Version == nil {
		problems = append(problems, "specification.version is required by the Sorna adapter")
	} else if _, err := versionNumber(*ir.Specification.Version); err != nil {
		problems = append(problems, "specification.version must use the vN form")
	}
	if len(ir.Specification.Scenarios) == 0 {
		problems = append(problems, "specification.scenarios must contain at least one scenario")
	}

	seenNames := make(map[string]bool, len(ir.Specification.Scenarios))
	for index, scenario := range ir.Specification.Scenarios {
		path := fmt.Sprintf("specification.scenarios[%d]", index)
		name := strings.TrimSpace(scenario.Name)
		if name == "" {
			problems = append(problems, path+".name must be non-empty")
		} else if seenNames[name] {
			problems = append(problems, path+".name duplicates "+strconv.Quote(name))
		} else {
			seenNames[name] = true
		}
		if strings.TrimSpace(scenario.When.Method) == "" {
			problems = append(problems, path+".when.method must be non-empty")
		}
		if strings.TrimSpace(scenario.When.Path) == "" {
			problems = append(problems, path+".when.path must be non-empty")
		}
		if scenario.State != nil && strings.TrimSpace(*scenario.State) == "" {
			problems = append(problems, path+".state must be non-empty when provided")
		}
		if scenario.State != nil && len(scenario.Setups) == 0 {
			problems = append(problems, path+".setups must be non-empty when state is provided")
		}
		if scenario.Request != nil {
			validateRequest(scenario.Request, path+".request", &problems)
		}
		if len(scenario.Requirements) == 0 {
			problems = append(problems, path+".requirements must contain at least one requirement")
		}
		for requirementIndex, requirement := range scenario.Requirements {
			requirementPath := fmt.Sprintf("%s.requirements[%d]", path, requirementIndex)
			if requirement.Kind != "must" && requirement.Kind != "must_not" {
				problems = append(problems, requirementPath+".kind must be must or must_not")
			}
			if strings.TrimSpace(requirement.Expression) == "" {
				problems = append(problems, requirementPath+".expression must be non-empty")
			}
		}

		setupNames := make(map[string]bool, len(scenario.Setups))
		for setupIndex, setup := range scenario.Setups {
			setupPath := fmt.Sprintf("%s.setups[%d]", path, setupIndex)
			name := strings.TrimSpace(setup.Name)
			if name == "" {
				problems = append(problems, setupPath+".name must be non-empty")
			} else if setupNames[name] {
				problems = append(problems, setupPath+".name duplicates "+strconv.Quote(name))
			} else {
				setupNames[name] = true
			}
			if strings.TrimSpace(setup.When.Method) == "" {
				problems = append(problems, setupPath+".when.method must be non-empty")
			}
			if strings.TrimSpace(setup.When.Path) == "" {
				problems = append(problems, setupPath+".when.path must be non-empty")
			}
			if setup.Request != nil {
				validateRequest(setup.Request, setupPath+".request", &problems)
			}
			if len(setup.Requirements) == 0 {
				problems = append(problems, setupPath+".requirements must contain at least one requirement")
			}
			for requirementIndex, requirement := range setup.Requirements {
				requirementPath := fmt.Sprintf("%s.requirements[%d]", setupPath, requirementIndex)
				if requirement.Kind != "must" && requirement.Kind != "must_not" {
					problems = append(problems, requirementPath+".kind must be must or must_not")
				}
				if strings.TrimSpace(requirement.Expression) == "" {
					problems = append(problems, requirementPath+".expression must be non-empty")
				}
			}

			captureNames := make(map[string]bool, len(setup.Captures))
			for captureIndex, capture := range setup.Captures {
				capturePath := fmt.Sprintf("%s.captures[%d]", setupPath, captureIndex)
				name := strings.TrimSpace(capture.Name)
				if name == "" {
					problems = append(problems, capturePath+".name must be non-empty")
				} else if captureNames[name] {
					problems = append(problems, capturePath+".name duplicates "+strconv.Quote(name))
				} else {
					captureNames[name] = true
				}
				if !bodySelector.MatchString(strings.TrimSpace(capture.Selector)) {
					problems = append(problems, capturePath+".selector must be a body.FIELD selector")
				}
			}
		}
	}
	return problems
}

func validateRequest(request *Request, path string, problems *[]string) {
	if request.Body == nil || len(request.Body) == 0 {
		*problems = append(*problems, path+".body must contain at least one field")
	}
	for name := range request.Body {
		if strings.TrimSpace(name) == "" {
			*problems = append(*problems, path+".body field names must be non-empty")
		}
	}
}

// Translate lowers the supported Malcolm IR subset to a Sorna draft contract.
func Translate(ir IR) (contract.Document, error) {
	if problems := ValidateIR(ir); len(problems) > 0 {
		return contract.Document{}, fmt.Errorf("invalid Malcolm IR: %s", strings.Join(problems, "; "))
	}
	if ir.Specification.Subject != nil && strings.TrimSpace(*ir.Specification.Subject) == "" {
		return contract.Document{}, fmt.Errorf("Malcolm subject cannot be empty when provided")
	}

	version, _ := versionNumber(*ir.Specification.Version)
	rules := make([]any, 0)
	entrypoints := make([]any, 0, len(ir.Specification.Scenarios))
	seenEntrypoints := make(map[string]bool)

	for _, scenario := range ir.Specification.Scenarios {
		if len(scenario.Given) > 0 {
			return contract.Document{}, fmt.Errorf(
				"scenario %q has free-form given clauses; Sorna lowering requires executable request data",
				scenario.Name,
			)
		}
		given := make(map[string]any)
		if scenario.State != nil {
			given["state"] = strings.TrimSpace(*scenario.State)
		}
		if scenario.Request != nil {
			given["body"] = scenario.Request.Body
		}
		if len(scenario.Setups) > 0 {
			setupValues := make([]any, 0, len(scenario.Setups))
			for _, setup := range scenario.Setups {
				lowered, err := lowerSetup(setup)
				if err != nil {
					return contract.Document{}, fmt.Errorf(
						"scenario %q setup %q: %w",
						scenario.Name,
						setup.Name,
						err,
					)
				}
				setupValues = append(setupValues, lowered)
			}
			given["setup"] = setupValues
		}
		method := strings.ToUpper(strings.TrimSpace(scenario.When.Method))
		if !supportedMethod(method) {
			return contract.Document{}, fmt.Errorf(
				"scenario %q uses unsupported HTTP method %q",
				scenario.Name,
				method,
			)
		}
		path := strings.TrimSpace(scenario.When.Path)
		subject := method + " " + path
		if !seenEntrypoints[subject] {
			entrypoints = append(entrypoints, map[string]any{
				"method": method,
				"path":   path,
			})
			seenEntrypoints[subject] = true
		}

		for index, requirement := range scenario.Requirements {
			if requirement.Kind != "must" && requirement.Kind != "must_not" {
				return contract.Document{}, fmt.Errorf(
					"scenario %q requirement %d uses unsupported kind %q",
					scenario.Name,
					index+1,
					requirement.Kind,
				)
			}
			expect, err := lowerExpression(requirement.Expression)
			if err != nil {
				return contract.Document{}, fmt.Errorf(
					"scenario %q requirement %d: %w",
					scenario.Name,
					index+1,
					err,
				)
			}
			rule := map[string]any{
				"id":       fmt.Sprintf("%s.requirement.%d", scenario.Name, index+1),
				"strength": requirement.Kind,
				"subject":  subject,
				"expect":   expect,
			}
			if len(given) > 0 {
				rule["given"] = given
			}
			rules = append(rules, rule)
		}
	}

	interfaceBody := map[string]any{
		"kind":               SornaHTTPJSON,
		"public_entrypoints": entrypoints,
	}
	if ir.Specification.Subject != nil {
		interfaceBody["subject"] = strings.TrimSpace(*ir.Specification.Subject)
	}

	return contract.Document{Contract: map[string]any{
		"schema":      SornaSchema,
		"id":          ir.Specification.Name,
		"version":     version,
		"status":      SornaStatus,
		"title":       ir.Specification.Name,
		"interface":   interfaceBody,
		"rules":       rules,
		"unspecified": []any{},
	}}, nil
}

// WriteFile writes a Sorna contract as indented JSON.
func WriteFile(path string, document contract.Document) error {
	if problems := contract.Validate(document); len(problems) > 0 {
		return fmt.Errorf("translated contract is invalid: %s", strings.Join(problems, "; "))
	}
	contents, err := json.MarshalIndent(map[string]any{"contract": document.Contract}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode translated contract: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o644)
}

func lowerSetup(setup Setup) (map[string]any, error) {
	method := strings.ToUpper(strings.TrimSpace(setup.When.Method))
	if !supportedMethod(method) {
		return nil, fmt.Errorf("uses unsupported HTTP method %q", method)
	}

	expect := make(map[string]any)
	expectNot := make([]any, 0)
	for index, requirement := range setup.Requirements {
		if requirement.Kind != "must" && requirement.Kind != "must_not" {
			return nil, fmt.Errorf(
				"requirement %d uses %q; stateful setup supports only must and must_not",
				index+1,
				requirement.Kind,
			)
		}
		lowered, err := lowerExpression(requirement.Expression)
		if err != nil {
			return nil, fmt.Errorf("requirement %d: %w", index+1, err)
		}
		if requirement.Kind == "must_not" {
			expectNot = append(expectNot, lowered)
			continue
		}
		if err := mergeExpectation(expect, lowered); err != nil {
			return nil, fmt.Errorf("requirement %d: %w", index+1, err)
		}
	}

	request := map[string]any{
		"method": method,
		"path":   strings.TrimSpace(setup.When.Path),
	}
	if setup.Request != nil {
		request["body"] = setup.Request.Body
	}

	lowered := map[string]any{
		"id":      setup.Name,
		"request": request,
	}
	if len(expect) > 0 || len(expectNot) == 0 {
		lowered["expect"] = expect
	}
	if len(expectNot) > 0 {
		lowered["expect_not"] = expectNot
	}
	if len(setup.Captures) > 0 {
		captures := make(map[string]any, len(setup.Captures))
		for _, capture := range setup.Captures {
			captures[capture.Name] = capture.Selector
		}
		lowered["capture"] = captures
	}
	return lowered, nil
}

func mergeExpectation(target, addition map[string]any) error {
	for name, value := range addition {
		existing, present := target[name]
		if !present {
			target[name] = value
			continue
		}
		if name == "body" {
			targetBody, ok := existing.(map[string]any)
			if !ok {
				return fmt.Errorf("existing body expectation is not an object")
			}
			additionBody, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("body expectation is not an object")
			}
			if err := mergeBodyExpectation(targetBody, additionBody); err != nil {
				return err
			}
			continue
		}
		if !reflect.DeepEqual(existing, value) {
			return fmt.Errorf("conflicting expectations for %s", name)
		}
	}
	return nil
}

func mergeBodyExpectation(target, addition map[string]any) error {
	for name, value := range addition {
		existing, present := target[name]
		if !present {
			target[name] = value
			continue
		}
		switch name {
		case "required":
			targetRequired, ok := existing.([]any)
			if !ok {
				return fmt.Errorf("existing body.required is not a list")
			}
			additionRequired, ok := value.([]any)
			if !ok {
				return fmt.Errorf("body.required is not a list")
			}
			for _, field := range additionRequired {
				alreadyPresent := false
				for _, existingField := range targetRequired {
					if reflect.DeepEqual(existingField, field) {
						alreadyPresent = true
						break
					}
				}
				if !alreadyPresent {
					targetRequired = append(targetRequired, field)
				}
			}
			target[name] = targetRequired
		case "properties":
			targetProperties, ok := existing.(map[string]any)
			if !ok {
				return fmt.Errorf("existing body.properties is not an object")
			}
			additionProperties, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("body.properties is not an object")
			}
			for field, property := range additionProperties {
				if oldProperty, present := targetProperties[field]; present {
					if !reflect.DeepEqual(oldProperty, property) {
						return fmt.Errorf("conflicting expectation for body property %s", field)
					}
				} else {
					targetProperties[field] = property
				}
			}
		default:
			if !reflect.DeepEqual(existing, value) {
				return fmt.Errorf("conflicting body expectations for %s", name)
			}
		}
	}
	return nil
}

func lowerExpression(expression string) (map[string]any, error) {
	expression = strings.TrimSpace(expression)
	if match := statusExpression.FindStringSubmatch(expression); match != nil {
		status, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("status value is invalid: %q", match[1])
		}
		return map[string]any{"status": status}, nil
	}
	if match := bodyExists.FindStringSubmatch(expression); match != nil {
		return map[string]any{
			"body": map[string]any{
				"required": []any{match[1]},
			},
		}, nil
	}
	if match := bodyEquals.FindStringSubmatch(expression); match != nil {
		value, err := parseLiteral(match[2])
		if err != nil {
			return nil, fmt.Errorf("body equality value: %w", err)
		}
		return map[string]any{
			"body": map[string]any{
				"properties": map[string]any{
					match[1]: map[string]any{"equals": value},
				},
			},
		}, nil
	}
	if match := eventEmit.FindStringSubmatch(expression); match != nil {
		return map[string]any{
			"events": map[string]any{
				"required": []any{match[1]},
			},
		}, nil
	}
	if match := eventOrder.FindStringSubmatch(expression); match != nil {
		ordered, err := parseEventOrder(match[1])
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"events": map[string]any{
				"ordered": ordered,
			},
		}, nil
	}
	return nil, fmt.Errorf(
		"expression %q is not lowerable; supported forms are response.status == N, response.body.FIELD exists, response.body.FIELD == VALUE, emit \"event.name\", and emit in order [\"event.a\", \"event.b\"]",
		expression,
	)
}

func parseEventOrder(raw string) ([]any, error) {
	parts := strings.Split(raw, ",")
	ordered := make([]any, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("event order entry %d must be a quoted non-empty event name", index)
		}
		event, err := strconv.Unquote(part)
		if err != nil || !eventName.MatchString(event) {
			return nil, fmt.Errorf("event order entry %d must be a quoted identifier-like event name", index)
		}
		if _, present := seen[event]; present {
			return nil, fmt.Errorf("event order contains duplicate event %q", event)
		}
		seen[event] = struct{}{}
		ordered = append(ordered, event)
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("event order must contain at least one event")
	}
	return ordered, nil
}

func parseLiteral(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return nil, fmt.Errorf("quoted value is invalid: %q", raw)
		}
		return value, nil
	}
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return value, nil
	}
	return nil, fmt.Errorf("value must be true, false, an integer, or a quoted string: %q", raw)
}

func versionNumber(version string) (int64, error) {
	if len(version) < 2 || version[0] != 'v' {
		return 0, fmt.Errorf("version must use the vN form")
	}
	value, err := strconv.ParseInt(version[1:], 10, 64)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("version must use the vN form")
	}
	return value, nil
}

func supportedMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
