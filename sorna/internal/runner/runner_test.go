package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	status200create "ingen/examples/document-pipeline-lab/defects/status-200-create"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
)

func TestExecuteDocumentPipelineContractAgainstCleanSubject(t *testing.T) {
	path := filepath.Join("..", "..", "..", "examples", "document-pipeline-lab", "contract", "contract.yaml")
	document, err := contract.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := contract.Seal(document)
	if err != nil {
		t.Fatal(err)
	}

	record, err := Execute(context.Background(), sealed, Config{
		BaseURL: "http://subject.invalid",
		Client: &http.Client{Transport: handlerTransport{
			handler: documentpipeline.NewHandler(documentpipeline.NewStore()),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Summary.Passed != 7 || record.Summary.Failed != 0 || record.Summary.Errors != 0 || record.Summary.Inconclusive != 0 {
		t.Fatalf("summary = %+v, rules = %+v; want all seven rules to pass", record.Summary, record.Rules)
	}
	if record.Verdict.Status != "pass" {
		t.Fatalf("contract verdict = %+v, want pass", record.Verdict)
	}
}

func TestExecuteMarksHostEnforcedManagedSubjectWithoutOverclaiming(t *testing.T) {
	path := filepath.Join("..", "..", "..", "examples", "document-pipeline-lab", "contract", "contract.yaml")
	document, err := contract.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := contract.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	record, err := Execute(context.Background(), sealed, Config{
		BaseURL: "http://subject.invalid",
		Lifecycle: &lifecycle.Record{
			Mode:    "managed-process",
			Sandbox: &lifecycle.SandboxRecord{Backend: "test", Enforcement: "host-enforced", PolicySHA256: "policy"},
		},
		Client: &http.Client{Transport: handlerTransport{
			handler: documentpipeline.NewHandler(documentpipeline.NewStore()),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Assurance.Level != 0 || record.Assurance.Status != "host-enforced-subject" {
		t.Fatalf("assurance = %+v, want level 0 host-enforced-subject", record.Assurance)
	}
	if len(record.Assurance.Limitations) != 1 || !strings.Contains(record.Assurance.Limitations[0], "not independently attested") {
		t.Fatalf("assurance limitations = %+v, want conservative host-boundary limitation", record.Assurance.Limitations)
	}
}

func TestExecuteOracleUsesFrozenCasesAndRecordsOracleLineage(t *testing.T) {
	path := filepath.Join("..", "..", "..", "examples", "document-pipeline-lab", "contract", "contract.yaml")
	document, err := contract.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := contract.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := oracle.Generate(sealed, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	oracleHash, err := oracle.Hash(artifact)
	if err != nil {
		t.Fatal(err)
	}

	record, err := ExecuteOracle(context.Background(), artifact, Config{
		BaseURL: "http://subject.invalid",
		Client: &http.Client{Transport: handlerTransport{
			handler: documentpipeline.NewHandler(documentpipeline.NewStore()),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Summary.Passed != 7 || record.Verdict.Status != "pass" {
		t.Fatalf("summary = %+v, verdict = %+v; want all frozen cases to pass", record.Summary, record.Verdict)
	}
	if record.Oracle == nil || record.Oracle.Schema != oracle.Schema || record.Oracle.SHA256 != oracleHash {
		t.Fatalf("oracle reference = %+v, want schema %s and hash %s", record.Oracle, oracle.Schema, oracleHash)
	}
	if record.Rules[0].CaseID != artifact.Cases[0].CaseID || record.Contract.SHA256 != artifact.Contract.SHA256 {
		t.Fatalf("record lineage = contract %s, case %s; want contract %s, case %s", record.Contract.SHA256, record.Rules[0].CaseID, artifact.Contract.SHA256, artifact.Cases[0].CaseID)
	}
}

func TestExecuteDetectsStatusMutation(t *testing.T) {
	path := filepath.Join("..", "..", "..", "examples", "document-pipeline-lab", "contract", "contract.yaml")
	document, err := contract.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := contract.Seal(document)
	if err != nil {
		t.Fatal(err)
	}

	cleanSubject := documentpipeline.NewHandler(documentpipeline.NewStore())
	defectSubject := status200create.NewHandler(cleanSubject)
	record, err := Execute(context.Background(), sealed, Config{
		BaseURL: "http://subject.invalid",
		Variant: "status-200-create",
		Mutation: &mutation.Spec{
			ID:              "status-200-create",
			Plane:           "behavior",
			Description:     "valid document creation returns 200 instead of 202",
			ExpectedRuleIDs: []string{"document.create.valid.accepted"},
		},
		Client: &http.Client{Transport: handlerTransport{
			handler: defectSubject,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Subject.Variant != "status-200-create" {
		t.Fatalf("subject variant = %q, want status-200-create", record.Subject.Variant)
	}
	if record.Mutation == nil || record.Mutation.Outcome != "killed" {
		t.Fatalf("mutation = %+v, want status-200-create killed", record.Mutation)
	}
	if record.Verdict.Status != "fail" {
		t.Fatalf("contract verdict = %+v, want fail while mutation is killed", record.Verdict)
	}
	if record.Rules[0].Status != "fail" {
		t.Fatalf("valid create result = %+v, want fail against status mutation", record.Rules[0])
	}
	if record.Rules[0].Assertions[0].Expected != json.Number("202") || record.Rules[0].Assertions[0].Actual != 200 {
		t.Fatalf("status assertion = %+v, want expected 202 and actual 200", record.Rules[0].Assertions[0])
	}
	if record.Summary.Failed != 1 {
		t.Fatalf("summary = %+v, want exactly one directly failed rule", record.Summary)
	}
}

func TestExecuteUsesOnlyTheHTTPBoundaryAndExecutesStateSetup(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.URL.Path == "/documents" {
			if r.ContentLength == 0 {
				t.Fatal("runner did not send a request body")
			}
			return responseFor(r, 202, `{"id":"doc-1","name":"welcome.md","status":"queued"}`), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == "/documents/doc-1" {
			return responseFor(r, 200, `{"id":"doc-1","name":"welcome.md","status":"queued"}`), nil
		}
		t.Fatalf("request = %s %s, want POST /documents or GET /documents/doc-1", r.Method, r.URL.Path)
		return nil, nil
	})}

	sealed := sealForTest(t, []any{
		validCreateRule(),
		map[string]any{
			"id":       "document.status.requires-state-setup",
			"strength": "must",
			"subject":  "GET /documents/{document_id}",
			"given": map[string]any{
				"state": "document_accepted",
				"setup": []any{map[string]any{
					"id": "accept-document",
					"request": map[string]any{
						"method": "POST",
						"path":   "/documents",
						"body": map[string]any{
							"name":    "welcome.md",
							"content": "Read the contract.",
						},
					},
					"expect":  map[string]any{"status": int64(202)},
					"capture": map[string]any{"document_id": "body.id"},
				}},
			},
			"expect": map[string]any{"status": int64(200)},
		},
	})
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	record, err := Execute(context.Background(), sealed, Config{
		BaseURL: "http://subject.invalid",
		Client:  client,
		Now:     func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	if record.Schema != "ingen.run/v1" || record.Contract.SHA256 != sealed.SHA256 {
		t.Fatalf("record metadata = %+v, want run schema and sealed hash", record)
	}
	if record.Summary.Passed != 2 || record.Summary.Skipped != 0 || record.Summary.Failed != 0 || record.Summary.Errors != 0 {
		t.Fatalf("summary = %+v, want 2 passed rules", record.Summary)
	}
	if record.Rules[0].Status != "pass" || record.Rules[0].ObservationSHA256 == "" {
		t.Fatalf("first rule = %+v, want pass with observation hash", record.Rules[0])
	}
	if record.Rules[1].Status != "pass" || len(record.Rules[1].Setup) != 1 {
		t.Fatalf("second rule = %+v, want pass with one setup step", record.Rules[1])
	}
	if !strings.HasSuffix(record.Rules[1].Request.URL, "/documents/doc-1") {
		t.Fatalf("target URL = %q, want captured document id in path", record.Rules[1].Request.URL)
	}

	contents, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal run record: %v", err)
	}
	if !strings.Contains(string(contents), `"status":"queued"`) {
		t.Fatalf("run record does not contain normalized observation: %s", contents)
	}
}

func TestExecuteSendsNestedRequestBodyValues(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		metadata := body["metadata"].(map[string]any)
		if metadata["source"] != "malcolm" || metadata["priority"] != float64(2) || metadata["reviewed"] != true {
			t.Fatalf("metadata = %#v", metadata)
		}
		tags := body["tags"].([]any)
		if len(tags) != 2 || tags[0] != "docs" || tags[1] != "contract" {
			t.Fatalf("tags = %#v", tags)
		}
		return responseFor(r, 202, "{\"status\":\"queued\"}"), nil
	})}

	sealed := sealForTest(t, []any{map[string]any{
		"id":       "document.create.nested",
		"strength": "must",
		"subject":  "POST /documents",
		"given": map[string]any{
			"body": map[string]any{
				"metadata": map[string]any{
					"source":   "malcolm",
					"priority": int64(2),
					"reviewed": true,
				},
				"tags": []any{"docs", "contract"},
			},
		},
		"expect": map[string]any{"status": int64(202)},
	}})
	record, err := Execute(context.Background(), sealed, Config{
		BaseURL: "http://subject.invalid",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Summary.Passed != 1 || record.Rules[0].Status != "pass" {
		t.Fatalf("record = %+v, want one passing nested-body rule", record)
	}
}

func TestExecuteTreatsNegativeSetupExpectationAsPrecondition(t *testing.T) {
	tests := []struct {
		name          string
		setupBody     string
		wantRule      string
		wantSetup     string
		wantInconcl   int
		wantRuleError bool
	}{
		{
			name:        "prohibited field is absent",
			setupBody:   "{\"id\":\"doc-1\"}",
			wantRule:    "pass",
			wantSetup:   "pass",
			wantInconcl: 0,
		},
		{
			name:          "prohibited field is present",
			setupBody:     "{\"id\":\"doc-1\",\"error\":{\"code\":\"unexpected\"}}",
			wantRule:      "inconclusive",
			wantSetup:     "fail",
			wantInconcl:   1,
			wantRuleError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost && r.URL.Path == "/documents" {
					return responseFor(r, 202, test.setupBody), nil
				}
				if r.Method == http.MethodGet && r.URL.Path == "/documents/doc-1" {
					return responseFor(r, 200, "{\"status\":\"queued\"}"), nil
				}
				t.Fatalf("request = %s %s, want setup POST or target GET", r.Method, r.URL.Path)
				return nil, nil
			})}
			sealed := sealForTest(t, []any{map[string]any{
				"id":       "document.state-precondition",
				"strength": "must",
				"subject":  "GET /documents/{document_id}",
				"given": map[string]any{
					"state": "document_accepted",
					"setup": []any{map[string]any{
						"id": "accept-document",
						"request": map[string]any{
							"method": "POST",
							"path":   "/documents",
						},
						"expect": map[string]any{"status": int64(202)},
						"expect_not": []any{map[string]any{
							"body": map[string]any{"required": []any{"error"}},
						}, map[string]any{
							"body": map[string]any{"required": []any{"rejected"}},
						}},
						"capture": map[string]any{"document_id": "body.id"},
					}},
				},
				"expect": map[string]any{"status": int64(200)},
			}})
			record, err := Execute(context.Background(), sealed, Config{
				BaseURL: "http://subject.invalid",
				Client:  client,
			})
			if err != nil {
				t.Fatal(err)
			}
			if record.Rules[0].Status != test.wantRule || record.Rules[0].Setup[0].Status != test.wantSetup {
				t.Fatalf("rule = %+v, want rule %s and setup %s", record.Rules[0], test.wantRule, test.wantSetup)
			}
			if record.Summary.Inconclusive != test.wantInconcl {
				t.Fatalf("summary = %+v, want inconclusive %d", record.Summary, test.wantInconcl)
			}
			if test.wantRuleError && !strings.Contains(record.Rules[0].Reason, "did not establish") {
				t.Fatalf("rule reason = %q, want setup precondition reason", record.Rules[0].Reason)
			}
		})
	}
}

func TestExecuteHonorsMustNotRuleStrength(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus string
	}{
		{
			name:       "prohibited field is absent",
			body:       `{"status":"ok"}`,
			wantStatus: "pass",
		},
		{
			name:       "prohibited field is present",
			body:       `{"error":{"code":"unexpected"}}`,
			wantStatus: "fail",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sealed := sealForTest(t, []any{map[string]any{
				"id":       "health.no-error",
				"strength": "must_not",
				"subject":  "GET /healthz",
				"expect": map[string]any{
					"body": map[string]any{
						"required": []any{"error"},
					},
				},
			}})
			record, err := Execute(context.Background(), sealed, Config{
				BaseURL: "http://subject.invalid",
				Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return responseFor(request, 200, test.body), nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			if record.Rules[0].Status != test.wantStatus {
				t.Fatalf("rule result = %+v, want status %s", record.Rules[0], test.wantStatus)
			}
		})
	}
}

func TestExecuteEvaluatesSubjectEventAssertions(t *testing.T) {
	tests := []struct {
		name       string
		strength   string
		header     string
		wantStatus string
	}{
		{
			name:       "must event is emitted",
			strength:   "must",
			header:     "document.accepted",
			wantStatus: "pass",
		},
		{
			name:       "must event is missing",
			strength:   "must",
			wantStatus: "fail",
		},
		{
			name:       "must_not event is missing",
			strength:   "must_not",
			wantStatus: "pass",
		},
		{
			name:       "must_not event is emitted",
			strength:   "must_not",
			header:     "document.accepted",
			wantStatus: "fail",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sealed := sealForTest(t, []any{map[string]any{
				"id":       "document.event",
				"strength": test.strength,
				"subject":  "POST /documents",
				"expect": map[string]any{
					"events": map[string]any{
						"required": []any{"document.accepted"},
					},
				},
			}})
			record, err := Execute(context.Background(), sealed, Config{
				BaseURL: "http://subject.invalid",
				Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					response := responseFor(request, 202, `{"status":"queued"}`)
					if test.header != "" {
						response.Header.Set(subjectEventHeader, test.header)
					}
					return response, nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			rule := record.Rules[0]
			if rule.Status != test.wantStatus {
				t.Fatalf("rule result = %+v, want status %s", rule, test.wantStatus)
			}
			if len(rule.Observation.Events) != boolToInt(test.header != "") {
				t.Fatalf("observed events = %#v, want header-derived event", rule.Observation.Events)
			}
		})
	}
}

func TestResponseEventsNormalizesMultipleHeaderValues(t *testing.T) {
	headers := make(http.Header)
	headers.Add(subjectEventHeader, "document.accepted, document.queued")
	headers.Add(subjectEventHeader, "document.accepted")

	got := responseEvents(headers)
	if strings.Join(got, ",") != "document.accepted,document.queued" {
		t.Fatalf("normalized events = %#v, want accepted then queued once", got)
	}
}

func TestEvaluateChecksMultipleRequiredEvents(t *testing.T) {
	assertions, err := evaluate(map[string]any{
		"events": map[string]any{
			"required": []any{"document.accepted", "document.queued"},
		},
	}, Observation{
		Events: []string{"document.accepted", "document.queued"},
		Body:   json.RawMessage("null"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(assertions) != 2 || assertions[0].Status != "pass" || assertions[1].Status != "pass" {
		t.Fatalf("event assertions = %#v, want two passes", assertions)
	}
}

func TestEvaluateChecksOrderedEventsAsRelativeSequence(t *testing.T) {
	tests := []struct {
		name       string
		actual     []string
		wantStatus string
	}{
		{
			name:       "same order",
			actual:     []string{"document.accepted", "document.queued"},
			wantStatus: "pass",
		},
		{
			name:       "extra event between expected events",
			actual:     []string{"document.accepted", "audit.recorded", "document.queued"},
			wantStatus: "pass",
		},
		{
			name:       "wrong order",
			actual:     []string{"document.queued", "document.accepted"},
			wantStatus: "fail",
		},
		{
			name:       "missing event",
			actual:     []string{"document.accepted"},
			wantStatus: "fail",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertions, err := evaluate(map[string]any{
				"events": map[string]any{
					"ordered": []any{"document.accepted", "document.queued"},
				},
			}, Observation{
				Events: test.actual,
				Body:   json.RawMessage("null"),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(assertions) != 1 || assertions[0].Path != "events.order" || assertions[0].Status != test.wantStatus {
				t.Fatalf("event assertions = %#v, want one events.order %s assertion", assertions, test.wantStatus)
			}
		})
	}
}

func TestExecuteMaterializesRepeatGeneratorAndReportsFailedAssertions(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Content) != 4097 {
			t.Errorf("content length = %d, want 4097", len(body.Content))
		}
		return responseFor(r, 400, `{"error":{"code":"wrong_code"}}`), nil
	})}

	rule := map[string]any{
		"id":       "document.create.oversize",
		"strength": "must",
		"subject":  "POST /documents",
		"given": map[string]any{
			"body": map[string]any{
				"name": "too-large.txt",
				"content": map[string]any{
					"generated": map[string]any{"kind": "repeat", "value": "a", "count": int64(4097)},
				},
			},
		},
		"expect": map[string]any{
			"status": int64(413),
			"error":  map[string]any{"code": "document_too_large"},
		},
	}
	record, err := Execute(context.Background(), sealForTest(t, []any{rule}), Config{BaseURL: "http://subject.invalid", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if record.Summary.Failed != 1 || record.Rules[0].Status != "fail" {
		t.Fatalf("record = %+v, want one failed rule", record)
	}
	if len(record.Rules[0].Assertions) != 2 || record.Rules[0].Assertions[0].Status != "fail" {
		t.Fatalf("assertions = %+v, want status mismatch and error mismatch", record.Rules[0].Assertions)
	}
}

func TestEvaluateShapeRejectsUndeclaredAdditionalProperties(t *testing.T) {
	assertions := evaluateShape("body", map[string]any{
		"id": "doc-1", "name": "welcome.md", "status": "queued", "debug": "mutation",
	}, map[string]any{
		"type":                  "object",
		"additional_properties": false,
		"required":              []any{"id", "name", "status"},
		"properties": map[string]any{
			"id":     map[string]any{"type": "string"},
			"name":   map[string]any{"equals": "welcome.md"},
			"status": map[string]any{"equals": "queued"},
		},
	})
	var extra *Assertion
	for index := range assertions {
		if assertions[index].Path == "body.debug" {
			extra = &assertions[index]
			break
		}
	}
	if extra == nil || extra.Status != "fail" || extra.Reason != "additional property is not allowed" {
		t.Fatalf("assertions = %+v, want a failed additional-property assertion", assertions)
	}
}

func TestEvaluateShapeSortsPropertyAssertions(t *testing.T) {
	assertions := evaluateShape("body", map[string]any{"zeta": "last", "alpha": "first"}, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"zeta":  map[string]any{"equals": "last"},
			"alpha": map[string]any{"equals": "first"},
		},
	})
	paths := make([]string, 0, len(assertions))
	for _, assertion := range assertions {
		paths = append(paths, assertion.Path)
	}
	if got, want := strings.Join(paths, ","), "body,body.alpha,body.zeta"; got != want {
		t.Fatalf("assertion paths = %q, want %q", got, want)
	}
}

func TestEvaluateExactObjectSortsFieldAssertions(t *testing.T) {
	assertions := evaluateExactObject("body.error", map[string]any{"code": "bad", "message": "invalid"}, map[string]any{
		"message": "invalid",
		"code":    "bad",
	})
	paths := make([]string, 0, len(assertions))
	for _, assertion := range assertions {
		paths = append(paths, assertion.Path)
	}
	if got, want := strings.Join(paths, ","), "body.error.code,body.error.message"; got != want {
		t.Fatalf("assertion paths = %q, want %q", got, want)
	}
}

func TestExecuteOracleSendsMaterializedFrozenBody(t *testing.T) {
	rule := map[string]any{
		"id":       "document.create.oversize",
		"strength": "must",
		"subject":  "POST /documents",
		"given": map[string]any{
			"body": map[string]any{
				"name": "too-large.txt",
				"content": map[string]any{
					"generated": map[string]any{"kind": "repeat", "value": "a", "count": int64(4097)},
				},
			},
		},
		"expect": map[string]any{"status": int64(400)},
	}
	sealed := sealForTest(t, []any{rule})
	artifact, err := oracle.Generate(sealed, strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	body := artifact.Cases[0].Given["body"].(map[string]any)
	if body["content"] != strings.Repeat("a", 4097) {
		t.Fatalf("frozen content = %v, want materialized input", body["content"])
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var requestBody struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(requestBody.Content) != 4097 {
			t.Fatalf("request content length = %d, want 4097", len(requestBody.Content))
		}
		return responseFor(r, 400, `{"error":{"code":"wrong_code"}}`), nil
	})}
	if _, err := ExecuteOracle(context.Background(), artifact, Config{BaseURL: "http://subject.invalid", Client: client}); err != nil {
		t.Fatal(err)
	}
}

func TestWritePersistsRunRecord(t *testing.T) {
	directory := t.TempDir()
	path, err := Write(directory, RunRecord{Schema: "ingen.run/v1", RunID: "run-test"})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(directory, "run.json") {
		t.Fatalf("path = %q, want run.json in output directory", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat run record: %v", err)
	}
}

func TestLoadFileRejectsUnknownFields(t *testing.T) {
	directory := t.TempDir()
	path, err := Write(directory, RunRecord{
		Schema:    Schema,
		RunID:     "run-load-test",
		CreatedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Contract:  ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("a", 64)},
		Subject:   SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(contents))
	text = strings.TrimSuffix(text, "}") + ",\n  \"unexpected\": true\n}"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("LoadFile() = %v, want unknown-field error", err)
	}
}

func TestValidateRejectsInvalidRunIdentity(t *testing.T) {
	record := RunRecord{
		Schema:   "sorna.run/v1",
		RunID:    "",
		Contract: ContractReference{ID: "contract-test", Version: 0, SHA256: "bad"},
		Subject:  SubjectReference{BaseURL: "not-a-url", Adapter: ""},
	}
	err := record.Validate()
	if err == nil || !strings.Contains(err.Error(), "schema must be ingen.run/v1") || !strings.Contains(err.Error(), "contract") || !strings.Contains(err.Error(), "subject.base_url") {
		t.Fatalf("Validate() = %v, want identity validation errors", err)
	}
}

func validCreateRule() map[string]any {
	return map[string]any{
		"id":       "document.create.valid.accepted",
		"strength": "must",
		"subject":  "POST /documents",
		"given": map[string]any{
			"body": map[string]any{
				"name":    "welcome.md",
				"content": "Read the contract.",
			},
		},
		"expect": map[string]any{
			"status": int64(202),
			"body": map[string]any{
				"type":                  "object",
				"additional_properties": false,
				"required":              []any{"id", "name", "status"},
				"properties": map[string]any{
					"id":     map[string]any{"type": "string", "non_empty": true},
					"name":   map[string]any{"equals": "welcome.md"},
					"status": map[string]any{"equals": "queued"},
				},
			},
		},
	}
}

func sealForTest(t *testing.T, rules []any) contract.Sealed {
	t.Helper()
	sealed, err := contract.Seal(contract.Document{Contract: map[string]any{
		"schema":      "ingen.contract/v1",
		"id":          "runner-test",
		"version":     int64(1),
		"status":      "draft",
		"interface":   map[string]any{"kind": "http-json"},
		"rules":       rules,
		"unspecified": []any{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func responseFor(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}

type handlerTransport struct {
	handler http.Handler
}

func (transport handlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response := httptest.NewRecorder()
	transport.handler.ServeHTTP(response, request)
	return response.Result(), nil
}
