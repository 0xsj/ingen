// Package runner executes the small, public-boundary portion of a Sorna run.
package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
)

// Config controls the public HTTP subject that a run observes.
type Config struct {
	BaseURL   string
	Client    *http.Client
	Now       func() time.Time
	Variant   string
	Mutation  *mutation.Spec
	Lifecycle *lifecycle.Record
}

// RunRecord is the first machine-readable Sorna run artifact. Its assurance
// status describes the subject boundary that was actually requested; level 0
// remains conservative until that boundary has independent attestation.
type RunRecord struct {
	Schema    string            `json:"schema"`
	RunID     string            `json:"run_id"`
	CreatedAt time.Time         `json:"created_at"`
	Assurance Assurance         `json:"assurance"`
	Contract  ContractReference `json:"contract"`
	Oracle    *OracleReference  `json:"oracle,omitempty"`
	Verdict   ContractVerdict   `json:"contract_verdict"`
	Subject   SubjectReference  `json:"subject"`
	Lifecycle *lifecycle.Record `json:"lifecycle,omitempty"`
	Summary   Summary           `json:"summary"`
	Rules     []RuleResult      `json:"rules"`
	Mutation  *mutation.Result  `json:"mutation,omitempty"`
}

type ContractVerdict struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type Assurance struct {
	Level       int      `json:"level"`
	Status      string   `json:"status"`
	Limitations []string `json:"limitations"`
}

type ContractReference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

// OracleReference binds a run to the exact canonical frozen oracle it
// consumed. The artifact itself is kept in the separate oracle evidence
// bundle, while this reference preserves the lineage in the subject run.
type OracleReference struct {
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

type SubjectReference struct {
	BaseURL string `json:"base_url"`
	Adapter string `json:"adapter"`
	Variant string `json:"variant,omitempty"`
}

type Summary struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Errors       int `json:"errors"`
	Inconclusive int `json:"inconclusive"`
	Skipped      int `json:"skipped"`
}

type RuleResult struct {
	RuleID            string       `json:"rule_id"`
	CaseID            string       `json:"case_id"`
	Subject           string       `json:"subject"`
	Status            string       `json:"status"`
	Setup             []StepResult `json:"setup,omitempty"`
	Request           Request      `json:"request"`
	Observation       Observation  `json:"observation"`
	ObservationSHA256 string       `json:"observation_sha256"`
	Assertions        []Assertion  `json:"assertions,omitempty"`
	Reason            string       `json:"reason,omitempty"`
}

type StepResult struct {
	ID                string      `json:"id"`
	Status            string      `json:"status"`
	Request           Request     `json:"request"`
	Observation       Observation `json:"observation"`
	ObservationSHA256 string      `json:"observation_sha256"`
	Assertions        []Assertion `json:"assertions,omitempty"`
	Reason            string      `json:"reason,omitempty"`
}

type Request struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   any    `json:"body,omitempty"`
}

type Observation struct {
	Status   int             `json:"status"`
	Body     json.RawMessage `json:"body,omitempty"`
	BodyText string          `json:"body_text,omitempty"`
}

type Assertion struct {
	Path     string `json:"path"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// Execute evaluates executable rules against a subject URL. Stateful rules
// declare public setup requests and captures in their given.setup sequence;
// the setup is recorded alongside the target observation.
func Execute(ctx context.Context, sealed contract.Sealed, config Config) (RunRecord, error) {
	cases, err := casesFromContract(sealed)
	if err != nil {
		return RunRecord{}, err
	}
	contractID, _ := sealed.Document.Contract["id"].(string)
	version, _ := integer(sealed.Document.Contract["version"])
	return executeCases(ctx, ContractReference{
		ID:      contractID,
		Version: version,
		SHA256:  sealed.SHA256,
	}, cases, config, nil)
}

// ExecuteOracle evaluates a previously frozen oracle against a subject URL.
// It deliberately accepts the oracle artifact rather than a contract.Sealed so
// the verified execution path cannot silently reopen contract source.
func ExecuteOracle(ctx context.Context, artifact oracle.Artifact, config Config) (RunRecord, error) {
	if problems := oracle.Validate(artifact); len(problems) > 0 {
		return RunRecord{}, fmt.Errorf("invalid oracle: %s", strings.Join(problems, "; "))
	}
	oracleHash, err := oracle.Hash(artifact)
	if err != nil {
		return RunRecord{}, err
	}
	cases := make([]executionCase, 0, len(artifact.Cases))
	for _, item := range artifact.Cases {
		cases = append(cases, executionCase{
			CaseID:   item.CaseID,
			RuleID:   item.RuleID,
			Strength: item.Strength,
			Subject:  item.Subject,
			Given:    item.Given,
			Expect:   item.Expect,
		})
	}
	return executeCases(ctx, ContractReference{
		ID:      artifact.Contract.ID,
		Version: artifact.Contract.Version,
		SHA256:  artifact.Contract.SHA256,
	}, cases, config, &OracleReference{Schema: artifact.Schema, SHA256: oracleHash})
}

type executionCase struct {
	CaseID   string
	RuleID   string
	Strength string
	Subject  string
	Given    map[string]any
	Expect   map[string]any
}

func casesFromContract(sealed contract.Sealed) ([]executionCase, error) {
	rules, ok := sealed.Document.Contract["rules"].([]any)
	if !ok {
		return nil, fmt.Errorf("sealed contract rules must be a list")
	}
	cases := make([]executionCase, 0, len(rules))
	for index, value := range rules {
		rule, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contract rule %d is not an object", index)
		}
		cases = append(cases, executionCaseFromRule(index, rule))
	}
	return cases, nil
}

func executionCaseFromRule(index int, rule map[string]any) executionCase {
	ruleID, _ := rule["id"].(string)
	strength, _ := rule["strength"].(string)
	subject, _ := rule["subject"].(string)
	given, _ := rule["given"].(map[string]any)
	expect, _ := rule["expect"].(map[string]any)
	return executionCase{
		CaseID:   fmt.Sprintf("case-%04d", index+1),
		RuleID:   ruleID,
		Strength: strength,
		Subject:  subject,
		Given:    given,
		Expect:   expect,
	}
}

func executeCases(ctx context.Context, contractReference ContractReference, cases []executionCase, config Config, oracleReference *OracleReference) (RunRecord, error) {
	base, err := parseBaseURL(config.BaseURL)
	if err != nil {
		return RunRecord{}, err
	}

	now := time.Now
	if config.Now != nil {
		now = config.Now
	}
	createdAt := now().UTC()
	limitations := make([]string, 0, 2)
	assuranceStatus := "self-reported"
	if config.Lifecycle == nil || config.Lifecycle.Mode != "managed-process" {
		limitations = append(limitations, "the subject was supplied as an already-running URL")
		limitations = append(limitations, "the runner does not attest to a capability boundary")
	} else if config.Lifecycle.Sandbox == nil {
		limitations = append(limitations, "Sorna managed process startup and teardown, but the process was not capability-isolated")
		limitations = append(limitations, "the runner does not attest to a capability boundary")
	} else if config.Lifecycle.Sandbox.Enforcement != "host-enforced" {
		limitations = append(limitations, "the managed subject policy was recorded but not host-enforced")
		limitations = append(limitations, "the runner does not attest to a capability boundary")
	} else {
		assuranceStatus = "host-enforced-subject"
		limitations = append(limitations, "subject policy was host-enforced, but access completeness and process-tree/subject-identity enforcement are not independently attested")
	}
	record := RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-" + strconv.FormatInt(createdAt.UnixNano(), 10),
		CreatedAt: createdAt,
		Assurance: Assurance{
			Level:       0,
			Status:      assuranceStatus,
			Limitations: limitations,
		},
		Contract: contractReference,
		Oracle:   oracleReference,
		Subject: SubjectReference{
			BaseURL: strings.TrimRight(base.String(), "/"),
			Adapter: "http-json-v1",
			Variant: config.Variant,
		},
		Lifecycle: config.Lifecycle,
		Rules:     make([]RuleResult, 0),
	}

	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	for index, testCase := range cases {
		result := executeCase(ctx, base, client, index, testCase)
		record.Rules = append(record.Rules, result)
		switch result.Status {
		case "pass":
			record.Summary.Passed++
		case "fail":
			record.Summary.Failed++
		case "error":
			record.Summary.Errors++
		case "inconclusive":
			record.Summary.Inconclusive++
		case "skipped":
			record.Summary.Skipped++
		}
	}
	if config.Mutation != nil {
		observations := make([]mutation.RuleObservation, 0, len(record.Rules))
		for _, rule := range record.Rules {
			observations = append(observations, mutation.RuleObservation{RuleID: rule.RuleID, Status: rule.Status})
		}
		classified := mutation.Classify(*config.Mutation, observations)
		record.Mutation = &classified
	}
	record.Verdict = verdictFor(record.Summary)
	return record, nil
}

func verdictFor(summary Summary) ContractVerdict {
	switch {
	case summary.Errors > 0:
		return ContractVerdict{Status: "error", Reason: "one or more rules could not be evaluated"}
	case summary.Failed > 0:
		return ContractVerdict{Status: "fail", Reason: "one or more rules violated the contract"}
	case summary.Inconclusive > 0 || summary.Skipped > 0:
		return ContractVerdict{Status: "inconclusive", Reason: "one or more rules were not fully evaluated"}
	default:
		return ContractVerdict{Status: "pass", Reason: "all evaluated rules satisfied the contract"}
	}
}

// Write writes the run record as JSON and returns its path.
func Write(outputDir string, record RunRecord) (string, error) {
	if strings.TrimSpace(outputDir) == "" {
		return "", fmt.Errorf("run output directory must not be empty")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	contents, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode run record: %w", err)
	}
	contents = append(contents, '\n')
	path := filepath.Join(outputDir, "run.json")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func executeRule(ctx context.Context, base *url.URL, client *http.Client, index int, value any) RuleResult {
	rule, ok := value.(map[string]any)
	if !ok {
		return errorResult(index, "", "", "contract rule is not an object")
	}
	return executeCase(ctx, base, client, index, executionCaseFromRule(index, rule))
}

func executeCase(ctx context.Context, base *url.URL, client *http.Client, index int, testCase executionCase) RuleResult {
	result := RuleResult{
		RuleID:  testCase.RuleID,
		CaseID:  testCase.CaseID,
		Subject: testCase.Subject,
		Status:  "error",
	}

	given := testCase.Given
	captures := make(map[string]any)
	if rawSetup, hasSetup := given["setup"]; hasSetup {
		setup, ok := rawSetup.([]any)
		if !ok || len(setup) == 0 {
			result.Reason = "given.setup must be a non-empty list"
			return result
		}
		for stepIndex, rawStep := range setup {
			step := executeSetupStep(ctx, base, client, stepIndex, rawStep, captures)
			result.Setup = append(result.Setup, step)
			if step.Status == "error" {
				result.Reason = fmt.Sprintf("setup step %q errored: %s", step.ID, step.Reason)
				return result
			}
			if step.Status != "pass" {
				result.Status = "inconclusive"
				result.Reason = fmt.Sprintf("setup step %q did not establish its precondition", step.ID)
				return result
			}
		}
	}
	method, path, err := parseSubject(testCase.Subject)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	var bodyValue any
	if rawBody, hasBody := given["body"]; hasBody {
		bodyValue = rawBody
	}
	requestRecord, observation, err := performRequest(ctx, base, client, method, path, bodyValue, captures)
	result.Request = requestRecord
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Observation = observation
	result.ObservationSHA256 = observationHash(observation)

	assertions, err := evaluate(testCase.Expect, observation)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Assertions = assertions
	result.Status = "pass"
	for _, assertion := range assertions {
		if assertion.Status != "pass" {
			result.Status = "fail"
			break
		}
	}
	return result
}

func executeSetupStep(ctx context.Context, base *url.URL, client *http.Client, index int, value any, captures map[string]any) StepResult {
	step, ok := value.(map[string]any)
	if !ok {
		return StepResult{ID: fmt.Sprintf("step-%04d", index+1), Status: "error", Reason: "setup step is not an object"}
	}
	stepID, _ := step["id"].(string)
	result := StepResult{ID: stepID, Status: "error"}
	requestSpec, ok := step["request"].(map[string]any)
	if !ok {
		result.Reason = "setup request is not an object"
		return result
	}
	method, _ := requestSpec["method"].(string)
	path, _ := requestSpec["path"].(string)
	var bodyValue any
	if body, present := requestSpec["body"]; present {
		bodyValue = body
	}
	requestRecord, observation, err := performRequest(ctx, base, client, strings.ToUpper(method), path, bodyValue, captures)
	result.Request = requestRecord
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Observation = observation
	result.ObservationSHA256 = observationHash(observation)

	expect, _ := step["expect"].(map[string]any)
	assertions, err := evaluate(expect, observation)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Assertions = assertions
	result.Status = "pass"
	for _, assertion := range assertions {
		if assertion.Status != "pass" {
			result.Status = "fail"
			result.Reason = "setup expectation did not match the observation"
			return result
		}
	}

	if rawCapture, present := step["capture"]; present {
		capture, ok := rawCapture.(map[string]any)
		if !ok {
			result.Status = "fail"
			result.Reason = "setup capture is not an object"
			return result
		}
		for name, rawSelector := range capture {
			selector, ok := rawSelector.(string)
			if !ok {
				result.Status = "fail"
				result.Reason = "setup capture selector is not a string: " + name
				return result
			}
			captured, err := selectBodyValue(observation, selector)
			if err != nil {
				result.Status = "fail"
				result.Reason = "capture " + name + ": " + err.Error()
				return result
			}
			captures[name] = captured
		}
	}
	return result
}

func performRequest(ctx context.Context, base *url.URL, client *http.Client, method, path string, bodyValue any, captures map[string]any) (Request, Observation, error) {
	resolvedPath, err := resolveTemplates(path, captures)
	if err != nil {
		return Request{Method: method}, Observation{}, fmt.Errorf("resolve request path: %w", err)
	}
	requestURL := resolveURL(base, resolvedPath)
	requestRecord := Request{Method: method, URL: requestURL}
	var body []byte
	if bodyValue != nil {
		materialized, err := materialize(bodyValue)
		if err != nil {
			return requestRecord, Observation{}, fmt.Errorf("materialize request body: %w", err)
		}
		materialized, err = resolveTemplatesValue(materialized, captures)
		if err != nil {
			return requestRecord, Observation{}, fmt.Errorf("resolve request body: %w", err)
		}
		requestRecord.Body = materialized
		body, err = json.Marshal(materialized)
		if err != nil {
			return requestRecord, Observation{}, fmt.Errorf("encode request body: %w", err)
		}
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return requestRecord, Observation{}, fmt.Errorf("create request: %w", err)
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return requestRecord, Observation{}, fmt.Errorf("request subject: %w", err)
	}
	defer response.Body.Close()
	observation, err := readObservation(response)
	if err != nil {
		observation.Status = response.StatusCode
		return requestRecord, observation, fmt.Errorf("read subject response: %w", err)
	}
	return requestRecord, observation, nil
}

func errorResult(index int, ruleID, subject, reason string) RuleResult {
	return RuleResult{
		RuleID:  ruleID,
		CaseID:  fmt.Sprintf("case-%04d", index+1),
		Subject: subject,
		Status:  "error",
		Reason:  reason,
	}
}

func parseBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base URL must be an absolute URL: %q", raw)
	}
	return parsed, nil
}

func parseSubject(raw string) (string, string, error) {
	parts := strings.Fields(raw)
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "/") {
		return "", "", fmt.Errorf("subject must have the form METHOD /path: %q", raw)
	}
	method := strings.ToUpper(parts[0])
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return method, parts[1], nil
	default:
		return "", "", fmt.Errorf("unsupported HTTP method in subject: %q", method)
	}
}

func resolveURL(base *url.URL, path string) string {
	resolved := *base
	resolved.Path = strings.TrimRight(base.Path, "/") + path
	resolved.RawPath = ""
	resolved.RawQuery = ""
	resolved.Fragment = ""
	return resolved.String()
}

func resolveTemplates(raw string, captures map[string]any) (string, error) {
	var builder strings.Builder
	for len(raw) > 0 {
		start := strings.IndexByte(raw, '{')
		if start < 0 {
			builder.WriteString(raw)
			break
		}
		builder.WriteString(raw[:start])
		raw = raw[start+1:]
		end := strings.IndexByte(raw, '}')
		if end < 0 {
			return "", fmt.Errorf("unclosed capture placeholder")
		}
		name := strings.TrimSpace(raw[:end])
		if name == "" {
			return "", fmt.Errorf("capture placeholder must have a name")
		}
		capture, ok := captures[name]
		if !ok {
			return "", fmt.Errorf("capture %q is not available", name)
		}
		text, ok := capture.(string)
		if !ok {
			return "", fmt.Errorf("capture %q is not a string", name)
		}
		builder.WriteString(url.PathEscape(text))
		raw = raw[end+1:]
	}
	return builder.String(), nil
}

func resolveTemplatesValue(value any, captures map[string]any) (any, error) {
	switch value := value.(type) {
	case string:
		if !strings.Contains(value, "{") {
			return value, nil
		}
		return resolveTemplates(value, captures)
	case map[string]any:
		object := make(map[string]any, len(value))
		for key, child := range value {
			resolved, err := resolveTemplatesValue(child, captures)
			if err != nil {
				return nil, err
			}
			object[key] = resolved
		}
		return object, nil
	case []any:
		sequence := make([]any, len(value))
		for index, child := range value {
			resolved, err := resolveTemplatesValue(child, captures)
			if err != nil {
				return nil, err
			}
			sequence[index] = resolved
		}
		return sequence, nil
	default:
		return value, nil
	}
}

func readObservation(response *http.Response) (Observation, error) {
	// The response body is read through the standard io.Reader below. Keeping
	// this helper separate makes the observation normalization boundary explicit.
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Observation{}, err
	}
	observation := Observation{Status: response.StatusCode}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		observation.Body = json.RawMessage("null")
		return observation, nil
	}
	if !json.Valid(trimmed) {
		observation.BodyText = string(body)
		observation.Body = json.RawMessage("null")
		return observation, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return Observation{}, err
	}
	observation.Body = json.RawMessage(compact.Bytes())
	return observation, nil
}

func observationHash(observation Observation) string {
	contents, _ := json.Marshal(observation)
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func selectBodyValue(observation Observation, selector string) (any, error) {
	parts := strings.Split(selector, ".")
	if len(parts) < 2 || parts[0] != "body" {
		return nil, fmt.Errorf("selector must start with body.")
	}
	actual, err := decodeBody(observation.Body)
	if err != nil {
		return nil, err
	}
	for _, part := range parts[1:] {
		object, ok := actual.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%q is not an object while selecting %q", part, selector)
		}
		actual, ok = object[part]
		if !ok {
			return nil, fmt.Errorf("field %q is missing", selector)
		}
	}
	return actual, nil
}

func decodeBody(raw json.RawMessage) (any, error) {
	var actual any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&actual); err != nil {
		return nil, fmt.Errorf("decode subject JSON: %w", err)
	}
	return actual, nil
}

func evaluate(expect map[string]any, observation Observation) ([]Assertion, error) {
	actual, err := decodeBody(observation.Body)
	if err != nil {
		return nil, err
	}
	assertions := make([]Assertion, 0)
	if expectedStatus, present := expect["status"]; present {
		assertions = append(assertions, equalityAssertion("status", expectedStatus, observation.Status))
	}
	if expectedBody, present := expect["body"]; present {
		spec, ok := expectedBody.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expect.body must be an object")
		}
		assertions = append(assertions, evaluateShape("body", actual, spec)...)
	}
	if expectedError, present := expect["error"]; present {
		spec, ok := expectedError.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expect.error must be an object")
		}
		var errorValue any
		if object, ok := actual.(map[string]any); ok {
			errorValue = object["error"]
		}
		assertions = append(assertions, evaluateExactObject("body.error", errorValue, spec)...)
	}
	return assertions, nil
}

func evaluateShape(path string, actual any, spec map[string]any) []Assertion {
	assertions := make([]Assertion, 0)
	if expectedType, present := spec["type"]; present {
		actualType := jsonType(actual)
		assertions = append(assertions, Assertion{
			Path:     path,
			Expected: expectedType,
			Actual:   actualType,
			Status:   statusFor(equalJSON(expectedType, actualType)),
		})
	}
	object, isObject := actual.(map[string]any)
	if required, present := spec["required"]; present {
		fields, _ := required.([]any)
		for _, rawField := range fields {
			field, _ := rawField.(string)
			_, exists := object[field]
			assertions = append(assertions, Assertion{
				Path:     path + "." + field,
				Expected: "present",
				Actual:   existenceValue(exists),
				Status:   statusFor(exists),
				Reason:   missingReason(exists, field),
			})
		}
	}
	if properties, present := spec["properties"]; present {
		propertySpecs, _ := properties.(map[string]any)
		for field, rawSpec := range propertySpecs {
			value, exists := object[field]
			propertySpec, isSpec := rawSpec.(map[string]any)
			if !isSpec {
				assertions = append(assertions, equalityAssertion(path+"."+field, rawSpec, valueIfPresent(value, exists)))
				continue
			}
			if !isObject && exists {
				assertions = append(assertions, Assertion{Path: path + "." + field, Expected: "object property", Actual: jsonType(actual), Status: "fail", Reason: "actual value is not an object"})
				continue
			}
			assertions = append(assertions, evaluateValue(path+"."+field, value, exists, propertySpec)...)
		}
	}
	return assertions
}

func evaluateExactObject(path string, actual any, spec map[string]any) []Assertion {
	object, ok := actual.(map[string]any)
	if !ok {
		return []Assertion{{Path: path, Expected: "object", Actual: jsonType(actual), Status: "fail", Reason: "actual value is not an object"}}
	}
	assertions := make([]Assertion, 0)
	for field, expected := range spec {
		value, exists := object[field]
		if childSpec, isSpec := expected.(map[string]any); isSpec {
			assertions = append(assertions, evaluateValue(path+"."+field, value, exists, childSpec)...)
		} else {
			assertions = append(assertions, equalityAssertion(path+"."+field, expected, valueIfPresent(value, exists)))
		}
	}
	return assertions
}

func evaluateValue(path string, value any, exists bool, spec map[string]any) []Assertion {
	assertions := make([]Assertion, 0)
	actual := valueIfPresent(value, exists)
	if expected, present := spec["equals"]; present {
		assertions = append(assertions, equalityAssertion(path, expected, actual))
	}
	if expectedType, present := spec["type"]; present {
		assertions = append(assertions, Assertion{Path: path, Expected: expectedType, Actual: jsonType(actual), Status: statusFor(exists && equalJSON(expectedType, jsonType(actual)))})
	}
	if nonEmpty, present := spec["non_empty"]; present && nonEmpty == true {
		text, isString := actual.(string)
		assertions = append(assertions, Assertion{Path: path, Expected: true, Actual: isString && strings.TrimSpace(text) != "", Status: statusFor(isString && strings.TrimSpace(text) != "")})
	}
	if len(assertions) == 0 {
		assertions = append(assertions, equalityAssertion(path, spec, actual))
	}
	for index := range assertions {
		if !exists && assertions[index].Reason == "" {
			assertions[index].Reason = "field is missing"
		}
	}
	return assertions
}

func materialize(value any) (any, error) {
	return contract.Materialize(value)
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

func equalityAssertion(path string, expected, actual any) Assertion {
	equal := equalJSON(expected, actual)
	return Assertion{Path: path, Expected: expected, Actual: actual, Status: statusFor(equal)}
}

func equalJSON(expected, actual any) bool {
	if reflect.DeepEqual(expected, actual) {
		return true
	}
	expectedNumber, expectedOK := numberString(expected)
	actualNumber, actualOK := numberString(actual)
	return expectedOK && actualOK && expectedNumber == actualNumber
}

func numberString(value any) (string, bool) {
	switch value := value.(type) {
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case json.Number:
		return string(value), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	default:
		return "", false
	}
}

func jsonType(value any) string {
	switch value := value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		if strings.ContainsAny(string(value), ".eE") {
			return "number"
		}
		return "integer"
	default:
		return "unknown"
	}
}

func statusFor(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func existenceValue(exists bool) string {
	if exists {
		return "present"
	}
	return "missing"
}

func missingReason(exists bool, field string) string {
	if exists {
		return ""
	}
	return "required field is missing: " + field
}

func valueIfPresent(value any, exists bool) any {
	if exists {
		return value
	}
	return nil
}
