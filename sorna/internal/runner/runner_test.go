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

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
	"ingen/sorna/internal/contract"
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
				"type":     "object",
				"required": []any{"id", "name", "status"},
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
