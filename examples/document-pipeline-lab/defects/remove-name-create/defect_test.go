package removenamecreate

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectRemovesOnlySuccessfulCreateName(t *testing.T) {
	handler := NewHandler(documentpipeline.NewHandler(documentpipeline.NewStore()))
	request := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"welcome.md","content":"Read the contract."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("defect create status = %d, want unchanged %d", response.Code, http.StatusAccepted)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["name"]; ok {
		t.Fatalf("defect response = %v, want name removed", payload)
	}
	if payload["status"] != "queued" {
		t.Fatalf("defect response status field = %v, want queued", payload["status"])
	}

	request = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"image.png","content":"not accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("defect rejection status = %d, want unchanged %d", response.Code, http.StatusBadRequest)
	}
}
