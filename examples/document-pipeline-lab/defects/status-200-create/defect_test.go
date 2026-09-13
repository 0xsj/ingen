package status200create

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectChangesOnlySuccessfulCreateStatus(t *testing.T) {
	handler := NewHandler(documentpipeline.NewHandler(documentpipeline.NewStore()))
	request := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"welcome.md","content":"Read the contract."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("defect create status = %d, want %d", response.Code, http.StatusOK)
	}

	request = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"image.png","content":"not accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("defect rejection status = %d, want unchanged %d", response.Code, http.StatusBadRequest)
	}
}
