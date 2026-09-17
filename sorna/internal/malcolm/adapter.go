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
	sornamutation "ingen/sorna/internal/mutation"
)

const (
	IRSchema                      = "malcolm.ir/v1"
	SornaSchema                   = "ingen.contract/v1"
	SornaStatus                   = "draft"
	SornaHTTPJSON                 = "http-json"
	maxGeneratedRepeatCount int64 = 1_000_000
)

// IR is the JSON representation emitted by Malcolm's Rust compiler.
type IR struct {
	Schema        string        `json:"schema"`
	Specification Specification `json:"specification"`
}

type Specification struct {
	Name       string      `json:"name"`
	Version    *string     `json:"version"`
	Subject    *string     `json:"subject"`
	Scenarios  []Scenario  `json:"scenarios"`
	Fixtures   []Fixture   `json:"fixtures,omitempty"`
	Mutations  []Mutation  `json:"mutations,omitempty"`
	Provenance *Provenance `json:"provenance,omitempty"`
	Isolation  *Isolation  `json:"isolation,omitempty"`
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

type Mutation struct {
	ID           string         `json:"id"`
	Scenario     string         `json:"scenario"`
	Change       MutationChange `json:"change"`
	ExpectedRule string         `json:"expected_rule"`
}

type MutationChange struct {
	Field string `json:"field"`
	From  int64  `json:"from"`
	To    int64  `json:"to"`
}

type Fixture struct {
	ID      string `json:"id"`
	Owner   string `json:"owner"`
	Purpose string `json:"purpose"`
	SHA256  string `json:"sha256"`
}

type Provenance struct {
	Requirements []ProvenanceRequirement `json:"requirements"`
}

type ProvenanceRequirement struct {
	Kind  string `json:"kind"`
	Field string `json:"field"`
}

type Isolation struct {
	Scope string `json:"scope"`
	Reset Reset  `json:"reset"`
}

type Reset struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

var (
	bodyPathPattern  = "[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*"
	statusExpression = regexp.MustCompile("^response\\.status\\s*==\\s*([0-9]+)$")
	bodyExists       = regexp.MustCompile("^response\\.body\\.(" + bodyPathPattern + ")\\s+exists$")
	bodyEquals       = regexp.MustCompile("^response\\.body\\.(" + bodyPathPattern + ")\\s*==\\s*(.+)$")
	eventEmit        = regexp.MustCompile("^emit\\s+\"([A-Za-z_][A-Za-z0-9_.:-]*)\"$")
	eventOrder       = regexp.MustCompile("^emit\\s+in\\s+order\\s+\\[(.*)\\]$")
	eventName        = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_.:-]*$")
	captureName      = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
	bodySelector     = regexp.MustCompile("^body\\.(" + bodyPathPattern + ")$")
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
	seenFixtureIDs := make(map[string]bool, len(ir.Specification.Fixtures))
	for index, fixture := range ir.Specification.Fixtures {
		path := fmt.Sprintf("specification.fixtures[%d]", index)
		id := strings.TrimSpace(fixture.ID)
		if id == "" {
			problems = append(problems, path+".id must be non-empty")
		} else if seenFixtureIDs[id] {
			problems = append(problems, path+".id duplicates "+strconv.Quote(id))
		} else {
			seenFixtureIDs[id] = true
		}
		if fixture.Owner != "oracle" {
			problems = append(problems, path+".owner must be oracle")
		}
		if strings.TrimSpace(fixture.Purpose) == "" {
			problems = append(problems, path+".purpose must be non-empty")
		}
		if !isSHA256(fixture.SHA256) {
			problems = append(problems, path+".sha256 must be a lowercase SHA-256 digest")
		}
	}

	seenNames := make(map[string]bool, len(ir.Specification.Scenarios))
	scenariosByName := make(map[string]Scenario, len(ir.Specification.Scenarios))
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
		if name != "" {
			scenariosByName[name] = scenario
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
		scenarioCaptures := make(map[string]struct{})
		for _, setup := range scenario.Setups {
			for _, capture := range setup.Captures {
				capture := strings.TrimSpace(capture.Name)
				if captureName.MatchString(capture) {
					scenarioCaptures[capture] = struct{}{}
				}
			}
		}
		if scenario.Request != nil {
			validateRequest(scenario.Request, path+".request", scenarioCaptures, &problems)
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
		availableCaptures := make(map[string]struct{})
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
				validateRequest(setup.Request, setupPath+".request", availableCaptures, &problems)
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
					problems = append(problems, capturePath+".selector must be a body.FIELD[.FIELD...] selector")
				}
				if captureName.MatchString(name) {
					availableCaptures[name] = struct{}{}
				}
			}
		}
	}

	seenMutationIDs := make(map[string]bool, len(ir.Specification.Mutations))
	for index, declaration := range ir.Specification.Mutations {
		path := fmt.Sprintf("specification.mutations[%d]", index)
		id := strings.TrimSpace(declaration.ID)
		if id == "" {
			problems = append(problems, path+".id must be non-empty")
		} else if seenMutationIDs[id] {
			problems = append(problems, path+".id duplicates "+strconv.Quote(id))
		} else {
			seenMutationIDs[id] = true
		}

		target, ok := scenariosByName[strings.TrimSpace(declaration.Scenario)]
		if !ok {
			problems = append(problems, path+".scenario must name an existing scenario")
		}
		if declaration.Change.Field != "response.status" {
			problems = append(problems, path+".change.field must be response.status")
		}
		if declaration.Change.From < 100 || declaration.Change.From > 599 {
			problems = append(problems, path+".change.from must be between 100 and 599")
		}
		if declaration.Change.To < 100 || declaration.Change.To > 599 {
			problems = append(problems, path+".change.to must be between 100 and 599")
		}
		if declaration.Change.From == declaration.Change.To {
			problems = append(problems, path+".change must change the response status")
		}

		expected := strings.TrimSpace(declaration.ExpectedRule)
		prefix := strings.TrimSpace(declaration.Scenario) + ".requirement."
		ruleIndex, err := strconv.Atoi(strings.TrimPrefix(expected, prefix))
		if expected == "" || !strings.HasPrefix(expected, prefix) || err != nil || ruleIndex < 1 || (ok && ruleIndex > len(target.Requirements)) {
			problems = append(problems, path+".expected_rule must name an existing scenario requirement")
		}
	}

	if provenance := ir.Specification.Provenance; provenance != nil {
		if len(provenance.Requirements) == 0 {
			problems = append(problems, "specification.provenance.requirements must contain at least one requirement")
		}
		seen := make(map[string]bool, len(provenance.Requirements))
		for index, requirement := range provenance.Requirements {
			path := fmt.Sprintf("specification.provenance.requirements[%d]", index)
			key := requirement.Kind + ":" + requirement.Field
			if seen[key] {
				problems = append(problems, path+" duplicates a provenance requirement")
			} else {
				seen[key] = true
			}
			if requirement.Kind != "create" {
				problems = append(problems, path+".kind must be create")
			}
			if requirement.Field != "execution_id" {
				problems = append(problems, path+".field must be execution_id")
			}
		}
	}

	if isolation := ir.Specification.Isolation; isolation != nil {
		if isolation.Scope != "scenario" {
			problems = append(problems, "specification.isolation.scope must be scenario")
		}
		if isolation.Reset.Method != "POST" {
			problems = append(problems, "specification.isolation.reset.method must be POST")
		}
		if !strings.HasPrefix(isolation.Reset.Path, "/") {
			problems = append(problems, "specification.isolation.reset.path must start with /")
		}
	}

	return problems
}

func validateRequest(request *Request, path string, availableCaptures map[string]struct{}, problems *[]string) {
	if request.Body == nil || len(request.Body) == 0 {
		*problems = append(*problems, path+".body must contain at least one field")
	}
	for name := range request.Body {
		if strings.TrimSpace(name) == "" {
			*problems = append(*problems, path+".body field names must be non-empty")
		}
		if name == "generated" {
			*problems = append(*problems, path+".body field name generated is reserved for generator values")
		}
		validateBodyValue(request.Body[name], path+".body."+name, availableCaptures, problems)
	}
}

func validateBodyValue(value any, path string, availableCaptures map[string]struct{}, problems *[]string) {
	switch value := value.(type) {
	case string:
		validateTemplateReferences(value, path, availableCaptures, problems)
	case bool, int, int64:
		return
	case json.Number:
		if _, err := strconv.ParseInt(string(value), 10, 64); err != nil {
			*problems = append(*problems, path+" must be an integer")
		}
	case map[string]any:
		if generated, present := value["generated"]; present {
			validateGeneratedValue(generated, path, len(value), problems)
			return
		}
		for name, child := range value {
			if strings.TrimSpace(name) == "" {
				*problems = append(*problems, path+" field names must be non-empty")
			}
			validateBodyValue(child, path+"."+name, availableCaptures, problems)
		}
	case []any:
		for index, child := range value {
			validateBodyValue(child, fmt.Sprintf("%s[%d]", path, index), availableCaptures, problems)
		}
	default:
		*problems = append(*problems, path+" must be a string, integer, boolean, object, or array")
	}
}

func validateTemplateReferences(value, path string, availableCaptures map[string]struct{}, problems *[]string) {
	remaining := value
	for {
		start := strings.IndexByte(remaining, '{')
		if start < 0 {
			return
		}
		afterOpen := remaining[start+1:]
		end := strings.IndexByte(afterOpen, '}')
		if end < 0 {
			*problems = append(*problems, path+" contains an unclosed capture placeholder")
			return
		}
		name := strings.TrimSpace(afterOpen[:end])
		if !captureName.MatchString(name) {
			*problems = append(*problems, path+" capture placeholder name must be an identifier")
		} else if _, ok := availableCaptures[name]; !ok {
			*problems = append(*problems, path+" capture "+strconv.Quote(name)+" is not available from an earlier setup")
		}
		remaining = afterOpen[end+1:]
	}
}

func validateGeneratedValue(value any, path string, outerFieldCount int, problems *[]string) {
	generator, ok := value.(map[string]any)
	if !ok {
		*problems = append(*problems, path+".generated must be an object")
		return
	}
	if outerFieldCount != 1 {
		*problems = append(*problems, path+" generated value must contain only the generated marker")
	}
	for name := range generator {
		if name != "kind" && name != "value" && name != "count" {
			*problems = append(*problems, path+".generated."+name+" is not supported")
		}
	}
	if kind, ok := generator["kind"].(string); !ok || kind != "repeat" {
		*problems = append(*problems, path+".generated.kind must be repeat")
	}
	if _, ok := generator["value"].(string); !ok {
		*problems = append(*problems, path+".generated.value must be a string")
	}
	count, valid := integerValue(generator["count"])
	if !valid || count < 1 || count > maxGeneratedRepeatCount {
		*problems = append(*problems, path+".generated.count must be between 1 and 1000000")
	}
}

func integerValue(value any) (int64, bool) {
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
	provenanceExpectation := lowerProvenanceExpectation(ir.Specification.Provenance)
	isolation := lowerIsolation(ir.Specification.Isolation)

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
		if isolation != nil {
			given["isolation"] = isolation
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
			if provenanceExpectation != nil {
				if err := mergeExpectation(expect, map[string]any{
					"provenance": provenanceExpectation,
				}); err != nil {
					return contract.Document{}, fmt.Errorf(
						"scenario %q requirement %d: %w",
						scenario.Name,
						index+1,
						err,
					)
				}
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

	contractBody := map[string]any{
		"schema":      SornaSchema,
		"id":          ir.Specification.Name,
		"version":     version,
		"status":      SornaStatus,
		"title":       ir.Specification.Name,
		"interface":   interfaceBody,
		"rules":       rules,
		"unspecified": []any{},
	}
	if len(ir.Specification.Fixtures) > 0 {
		fixtures := make([]any, 0, len(ir.Specification.Fixtures))
		for _, fixture := range ir.Specification.Fixtures {
			fixtures = append(fixtures, map[string]any{
				"id":      fixture.ID,
				"owner":   fixture.Owner,
				"purpose": fixture.Purpose,
				"sha256":  fixture.SHA256,
			})
		}
		contractBody["fixtures"] = fixtures
	}
	return contract.Document{Contract: contractBody}, nil
}

func isSHA256(value string) bool {
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

func lowerProvenanceExpectation(provenance *Provenance) map[string]any {
	if provenance == nil {
		return nil
	}
	required := make([]any, 0, len(provenance.Requirements))
	for _, requirement := range provenance.Requirements {
		if requirement.Kind == "create" && requirement.Field == "execution_id" {
			required = append(required, requirement.Field)
		}
	}
	return map[string]any{"required": required}
}

func lowerIsolation(isolation *Isolation) map[string]any {
	if isolation == nil {
		return nil
	}
	return map[string]any{
		"scope": isolation.Scope,
		"reset": map[string]any{
			"method": isolation.Reset.Method,
			"path":   isolation.Reset.Path,
		},
	}
}

// TranslateMutationCatalogue lowers Malcolm's reviewed mutation declarations
// into Sorna's separate mutation-catalogue artifact. Mutation declarations are
// not copied into the behavioral contract because Sorna owns mutation
// planning, provider selection, execution, and evidence classification.
func TranslateMutationCatalogue(ir IR) (sornamutation.Catalogue, error) {
	if problems := ValidateIR(ir); len(problems) > 0 {
		return sornamutation.Catalogue{}, fmt.Errorf("invalid Malcolm IR: %s", strings.Join(problems, "; "))
	}

	version, _ := versionNumber(*ir.Specification.Version)
	scenarios := make(map[string]Scenario, len(ir.Specification.Scenarios))
	for _, scenario := range ir.Specification.Scenarios {
		scenarios[scenario.Name] = scenario
	}

	catalogue := sornamutation.Catalogue{
		Schema:          sornamutation.Schema,
		ID:              ir.Specification.Name + "-mutations",
		Version:         1,
		ContractID:      ir.Specification.Name,
		ContractVersion: version,
		Mutations:       make([]sornamutation.Spec, 0, len(ir.Specification.Mutations)),
	}
	for _, declaration := range ir.Specification.Mutations {
		scenario := scenarios[declaration.Scenario]
		method := strings.ToUpper(strings.TrimSpace(scenario.When.Method))
		target := method + " " + strings.TrimSpace(scenario.When.Path)
		catalogue.Mutations = append(catalogue.Mutations, sornamutation.Spec{
			ID:       declaration.ID,
			Plane:    "implementation",
			Operator: "response.status.replace",
			Target:   target,
			Description: fmt.Sprintf(
				"Change response.status from %d to %d for scenario %s.",
				declaration.Change.From,
				declaration.Change.To,
				declaration.Scenario,
			),
			Change: map[string]any{
				"from": declaration.Change.From,
				"to":   declaration.Change.To,
			},
			ExpectedRuleIDs: []string{declaration.ExpectedRule},
			Status:          "candidate",
		})
	}
	if problems := sornamutation.Validate(catalogue); len(problems) > 0 {
		return sornamutation.Catalogue{}, fmt.Errorf(
			"translated mutation catalogue is invalid: %s",
			strings.Join(problems, "; "),
		)
	}
	return catalogue, nil
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

// WriteMutationCatalogue writes the Sorna mutation catalogue as indented JSON.
func WriteMutationCatalogue(path string, catalogue sornamutation.Catalogue) error {
	if problems := sornamutation.Validate(catalogue); len(problems) > 0 {
		return fmt.Errorf("mutation catalogue is invalid: %s", strings.Join(problems, "; "))
	}
	contents, err := json.MarshalIndent(
		map[string]any{"mutation_catalogue": catalogue},
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf("encode mutation catalogue: %w", err)
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
	return mergeBodyShape(target, addition, "body")
}

func mergeBodyShape(target, addition map[string]any, path string) error {
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
				return fmt.Errorf("existing %s.required is not a list", path)
			}
			additionRequired, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%s.required is not a list", path)
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
				return fmt.Errorf("existing %s.properties is not an object", path)
			}
			additionProperties, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.properties is not an object", path)
			}
			for field, property := range additionProperties {
				if oldProperty, present := targetProperties[field]; present {
					oldShape, oldIsShape := oldProperty.(map[string]any)
					newShape, newIsShape := property.(map[string]any)
					if oldIsShape && newIsShape {
						if err := mergeBodyShape(oldShape, newShape, path+"."+field); err != nil {
							return err
						}
					} else if !reflect.DeepEqual(oldProperty, property) {
						return fmt.Errorf("conflicting body expectations for %s.%s", path, field)
					}
				} else {
					targetProperties[field] = property
				}
			}
		default:
			if !reflect.DeepEqual(existing, value) {
				return fmt.Errorf("conflicting expectations for %s.%s", path, name)
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
		return lowerBodyPathExpectation(match[1], nil, true), nil
	}
	if match := bodyEquals.FindStringSubmatch(expression); match != nil {
		value, err := parseLiteral(match[3])
		if err != nil {
			return nil, fmt.Errorf("body equality value: %w", err)
		}
		return lowerBodyPathExpectation(match[1], map[string]any{"equals": value}, false), nil
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
		"expression %q is not lowerable; supported forms are response.status == N, response.body.FIELD[.FIELD...] exists, response.body.FIELD[.FIELD...] == VALUE, emit \"event.name\", and emit in order [\"event.a\", \"event.b\"]",
		expression,
	)
}

func lowerBodyPathExpectation(path string, leaf map[string]any, exists bool) map[string]any {
	parts := strings.Split(path, ".")
	body := make(map[string]any)
	if len(parts) == 1 {
		if exists {
			body["required"] = []any{parts[0]}
		} else {
			body["properties"] = map[string]any{
				parts[0]: leaf,
			}
		}
		return map[string]any{"body": body}
	}

	var nested map[string]any
	if exists {
		nested = map[string]any{
			"type":     "object",
			"required": []any{parts[len(parts)-1]},
		}
		for index := len(parts) - 2; index >= 1; index-- {
			nested = map[string]any{
				"type": "object",
				"properties": map[string]any{
					parts[index]: nested,
				},
			}
		}
	} else {
		nested = leaf
		for index := len(parts) - 1; index >= 1; index-- {
			nested = map[string]any{
				"type": "object",
				"properties": map[string]any{
					parts[index]: nested,
				},
			}
		}
	}
	body["properties"] = map[string]any{
		parts[0]: nested,
	}
	return map[string]any{"body": body}
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
