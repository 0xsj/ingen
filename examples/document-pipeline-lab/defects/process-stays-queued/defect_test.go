package processstaysqueued

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectChangesOnlyProcessCompletionResponse(t *testing.T) {
	handler := NewHandler(documentpipeline.NewHandler(documentpipeline.NewStore()))

	request := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"welcome.md","content":"accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want unchanged %d", response.Code, http.StatusAccepted)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%s/process", created.ID), nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("process status = %d, want unchanged %d", response.Code, http.StatusOK)
	}
	var processed map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &processed); err != nil {
		t.Fatal(err)
	}
	if processed["status"] != "queued" {
		t.Fatalf("process response = %v, want queued defect state", processed)
	}

	request = httptest.NewRequest(http.MethodPost, "/documents/does-not-exist/process", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing process status = %d, want unchanged %d", response.Code, http.StatusNotFound)
	}
}
