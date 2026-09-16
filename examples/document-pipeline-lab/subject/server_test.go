package documentpipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentWorkflowExposesContractedStatesAndResult(t *testing.T) {
	handler := NewHandler(NewStore())

	create := requestJSON(t, handler, http.MethodPost, "/documents", map[string]string{
		"name":    "welcome.md",
		"content": "Read the contract.",
	})
	if create.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want %d", create.Code, http.StatusAccepted)
	}
	events := create.Header().Values("X-InGen-Event")
	if len(events) != 2 || events[0] != "document.accepted" || events[1] != "document.queued" {
		t.Fatalf("create event headers = %#v, want accepted then queued", events)
	}

	var accepted struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	decodeJSON(t, create, &accepted)
	if accepted.ID == "" || accepted.Name != "welcome.md" || accepted.Status != "queued" {
		t.Fatalf("create body = %+v, want a queued document", accepted)
	}

	status := requestJSON(t, handler, http.MethodGet, "/documents/"+accepted.ID, nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status status = %d, want %d", status.Code, http.StatusOK)
	}
	var queued struct {
		Status string `json:"status"`
	}
	decodeJSON(t, status, &queued)
	if queued.Status != "queued" {
		t.Fatalf("status body = %+v, want queued", queued)
	}

	process := requestJSON(t, handler, http.MethodPost, "/documents/"+accepted.ID+"/process", nil)
	if process.Code != http.StatusOK {
		t.Fatalf("process status = %d, want %d", process.Code, http.StatusOK)
	}

	result := requestJSON(t, handler, http.MethodGet, "/documents/"+accepted.ID+"/result", nil)
	if result.Code != http.StatusOK {
		t.Fatalf("result status = %d, want %d", result.Code, http.StatusOK)
	}
	var got Result
	decodeJSON(t, result, &got)
	digest := sha256.Sum256([]byte("Read the contract."))
	wantDigest := hex.EncodeToString(digest[:])
	if got.DocumentID != accepted.ID || got.ContentSHA256 != wantDigest || got.WordCount != 3 {
		t.Fatalf("result = %+v, want document_id=%q digest=%q word_count=3", got, accepted.ID, wantDigest)
	}
}

func TestHealthEndpointSupportsManagedSubjectReadiness(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodGet, "/healthz", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
	var body struct {
		Status string `json:"status"`
	}
	decodeJSON(t, response, &body)
	if body.Status != "ok" {
		t.Fatalf("health body = %+v, want status=ok", body)
	}
}

func TestCreateRejectsUnsupportedType(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodPost, "/documents", map[string]string{
		"name":    "image.png",
		"content": "not an accepted document",
	})

	assertErrorCode(t, response, http.StatusBadRequest, "unsupported_document_type")
}

func TestCreateRejectsContentOverTheContractLimit(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodPost, "/documents", map[string]string{
		"name":    "too-large.txt",
		"content": strings.Repeat("a", MaxContentBytes+1),
	})

	assertErrorCode(t, response, http.StatusRequestEntityTooLarge, "document_too_large")
}

func TestProcessMissingDocumentIsNotSuccess(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodPost, "/documents/does-not-exist/process", nil)

	assertErrorCode(t, response, http.StatusNotFound, "document_not_found")
}

func TestResultBeforeProcessingIsNotReportedAsCompleted(t *testing.T) {
	handler := NewHandler(NewStore())
	create := requestJSON(t, handler, http.MethodPost, "/documents", map[string]string{
		"name":    "pending.txt",
		"content": "still queued",
	})
	var accepted struct {
		ID string `json:"id"`
	}
	decodeJSON(t, create, &accepted)

	response := requestJSON(t, handler, http.MethodGet, "/documents/"+accepted.ID+"/result", nil)
	assertErrorCode(t, response, http.StatusConflict, "document_not_completed")
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, payload)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response JSON: %v; body=%q", err, response.Body.String())
	}
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, status, response.Body.String())
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSON(t, response, &envelope)
	if envelope.Error.Code != code {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, code)
	}
}
