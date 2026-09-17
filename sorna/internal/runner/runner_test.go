package runner

import (
	"context"
	"encoding/base64"
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
	if record.Summary.Passed != 8 || record.Summary.Failed != 0 || record.Summary.Errors != 0 || record.Summary.Inconclusive != 0 {
		t.Fatalf("summary = %+v, rules = %+v; want all eight rules to pass", record.Summary, record.Rules)
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
	if record.Summary.Passed != 8 || record.Verdict.Status != "pass" {
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

func TestEvaluateChecksNestedResponseProperties(t *testing.T) {
	assertions, err := evaluate(map[string]any{
		"body": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"metadata": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"owner": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id": map[string]any{"equals": "owner-1"},
							},
						},
					},
				},
			},
		},
	}, Observation{
		Status: 200,
		Body:   json.RawMessage(`{"metadata":{"owner":{"id":"owner-1"}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, assertion := range assertions {
		if assertion.Status != "pass" {
			t.Fatalf("nested assertion = %+v, want pass", assertion)
		}
	}
	if len(assertions) != 4 {
		t.Fatalf("nested assertions = %#v, want type checks and leaf equality", assertions)
	}
}

func TestNestedSelectorDistinguishesMissingFromExplicitNull(t *testing.T) {
	observation := Observation{
		Body: json.RawMessage(`{"metadata":{"nullable":null}}`),
	}
	value, err := selectBodyValue(observation, "body.metadata.nullable")
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Fatalf("selected explicit null = %#v, want nil value", value)
	}
	if _, err := selectBodyValue(observation, "body.metadata.missing"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing selector error = %v, want missing-field error", err)
	}

	assertions := evaluateShape("body", map[string]any{
		"metadata": map[string]any{"nullable": nil},
	}, map[string]any{
		"properties": map[string]any{
			"metadata": map[string]any{
				"required": []any{"nullable"},
			},
		},
	})
	if len(assertions) != 1 || assertions[0].Status != "pass" {
		t.Fatalf("explicit-null existence assertion = %#v, want one pass", assertions)
	}
}

func TestExecuteResolvesCapturedValueInRequestBodyWithoutPathEscaping(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		switch body["content"] {
		case "seed":
			if body["name"] != "seed copy.txt" {
				t.Fatalf("setup body = %#v, want source name", body)
			}
			return responseFor(r, 202, `{"name":"seed copy.txt","status":"queued"}`), nil
		case "copied":
			if body["name"] != "seed copy.txt" {
				t.Fatalf("target body = %#v, want raw captured name", body)
			}
			return responseFor(r, 202, `{"name":"seed copy.txt","status":"queued"}`), nil
		default:
			t.Fatalf("request body = %#v, want setup or target content", body)
			return nil, nil
		}
	})}

	sealed := sealForTest(t, []any{map[string]any{
		"id":       "document.create.from-capture",
		"strength": "must",
		"subject":  "POST /documents",
		"given": map[string]any{
			"body": map[string]any{
				"name":    "{document_name}",
				"content": "copied",
			},
			"setup": []any{map[string]any{
				"id": "create-seed",
				"request": map[string]any{
					"method": "POST",
					"path":   "/documents",
					"body": map[string]any{
						"name":    "seed copy.txt",
						"content": "seed",
					},
				},
				"expect": map[string]any{"status": int64(202)},
				"capture": map[string]any{
					"document_name": "body.name",
				},
			}},
		},
		"expect": map[string]any{
			"status": int64(202),
			"body": map[string]any{
				"properties": map[string]any{
					"name": map[string]any{"equals": "seed copy.txt"},
				},
			},
		},
	}})
	record, err := Execute(context.Background(), sealed, Config{
		BaseURL: "http://subject.invalid",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Summary.Passed != 1 || record.Rules[0].Status != "pass" {
		t.Fatalf("record = %+v, want one passing interpolated-body rule", record)
	}
}

func TestResolveBodyTemplatesRejectsMissingOrNonStringCaptures(t *testing.T) {
	tests := []struct {
		name     string
		captures map[string]any
		want     string
	}{
		{
			name:     "missing capture",
			captures: map[string]any{},
			want:     `capture "document_name" is not available`,
		},
		{
			name:     "non-string capture",
			captures: map[string]any{"document_name": int64(1)},
			want:     `capture "document_name" is not a string`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveTemplatesValue(map[string]any{
				"name": "{document_name}",
			}, test.captures)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text containing %q", err, test.want)
			}
		})
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

func TestExecuteChecksDeclaredAmberExecutionID(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte("{\"execution_id\":\"exec-1\"}"))
	tests := []struct {
		name           string
		withHeader     bool
		wantRuleStatus string
	}{
		{name: "present", withHeader: true, wantRuleStatus: "pass"},
		{name: "missing", withHeader: false, wantRuleStatus: "fail"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sealed := sealForTest(t, []any{map[string]any{
				"id":       "document.provenance",
				"strength": "must",
				"subject":  "POST /documents",
				"expect": map[string]any{
					"provenance": map[string]any{
						"required": []any{"execution_id"},
					},
				},
			}})
			record, err := Execute(context.Background(), sealed, Config{
				BaseURL: "http://subject.invalid",
				Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					response := responseFor(request, 202, "{\"status\":\"queued\"}")
					if test.withHeader {
						response.Header.Set(amberProvenanceHeader, header)
					}
					return response, nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			rule := record.Rules[0]
			if rule.Status != test.wantRuleStatus {
				t.Fatalf("rule = %+v, want status %s", rule, test.wantRuleStatus)
			}
			if len(rule.Assertions) != 1 || rule.Assertions[0].Path != "provenance.execution_id" {
				t.Fatalf("provenance assertions = %#v, want one execution_id assertion", rule.Assertions)
			}
			if test.withHeader {
				if rule.Observation.Provenance == nil || !rule.Observation.Provenance.ShapeValid || rule.Observation.Provenance.ExecutionID != "exec-1" {
					t.Fatalf("provenance observation = %#v, want valid exec-1", rule.Observation.Provenance)
				}
			} else if rule.Observation.Provenance != nil {
				t.Fatalf("missing header produced provenance observation = %#v", rule.Observation.Provenance)
			}
		})
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

func TestExecuteResetsSubjectStateBeforeEachIsolatedCase(t *testing.T) {
	resetCalls := 0
	created := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/__malcolm/reset":
			resetCalls++
			created = false
			return responseFor(r, http.StatusNoContent, ""), nil
		case r.Method == http.MethodPost && r.URL.Path == "/documents":
			if created {
				return responseFor(r, http.StatusConflict, `{"error":"state leaked"}`), nil
			}
			created = true
			return responseFor(r, http.StatusAccepted, `{"id":"doc-1"}`), nil
		default:
			t.Fatalf("request = %s %s, want reset or create", r.Method, r.URL.Path)
			return nil, nil
		}
	})}

	isolation := map[string]any{
		"scope": "scenario",
		"reset": map[string]any{
			"method": "POST",
			"path":   "/__malcolm/reset",
		},
	}
	rules := []any{
		isolatedStatusRule("first", isolation),
		isolatedStatusRule("second", isolation),
	}
	record, err := Execute(context.Background(), sealForTest(t, rules), Config{
		BaseURL: "http://subject.invalid",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resetCalls != 2 {
		t.Fatalf("reset calls = %d, want one reset per case", resetCalls)
	}
	if record.Summary.Passed != 2 || record.Verdict.Status != "pass" {
		t.Fatalf("summary = %+v, verdict = %+v, want two passing isolated cases", record.Summary, record.Verdict)
	}
	for index, rule := range record.Rules {
		if rule.Isolation == nil || rule.Isolation.Status != "pass" || rule.Isolation.Observation.Status != http.StatusNoContent {
			t.Fatalf("rule %d isolation = %+v, want successful 204 reset evidence", index, rule.Isolation)
		}
		if rule.Isolation.ObservationSHA256 == "" {
			t.Fatalf("rule %d isolation evidence has no observation hash", index)
		}
	}
}

func TestExecuteMakesResetFailureInconclusive(t *testing.T) {
	targetCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.URL.Path == "/__malcolm/reset" {
			return responseFor(r, http.StatusServiceUnavailable, `{"error":"reset unavailable"}`), nil
		}
		targetCalls++
		return responseFor(r, http.StatusAccepted, `{"id":"doc-1"}`), nil
	})}

	record, err := Execute(context.Background(), sealForTest(t, []any{
		isolatedStatusRule("reset-fails", map[string]any{
			"scope": "scenario",
			"reset": map[string]any{
				"method": "POST",
				"path":   "/__malcolm/reset",
			},
		}),
	}), Config{BaseURL: "http://subject.invalid", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if targetCalls != 0 {
		t.Fatalf("target calls = %d, want no target request after reset failure", targetCalls)
	}
	if record.Summary.Inconclusive != 1 || record.Rules[0].Status != "inconclusive" {
		t.Fatalf("record = %+v, want one inconclusive rule", record)
	}
	if record.Rules[0].Isolation == nil || record.Rules[0].Isolation.Status != "inconclusive" {
		t.Fatalf("isolation = %+v, want inconclusive reset evidence", record.Rules[0].Isolation)
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

func isolatedStatusRule(id string, isolation map[string]any) map[string]any {
	return map[string]any{
		"id":       id,
		"strength": "must",
		"subject":  "POST /documents",
		"given": map[string]any{
			"isolation": isolation,
		},
		"expect": map[string]any{"status": int64(http.StatusAccepted)},
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
