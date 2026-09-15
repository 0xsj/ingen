package unsupportedtype500

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectChangesOnlyUnsupportedTypeStatus(t *testing.T) {
	handler := NewHandler(documentpipeline.NewHandler(documentpipeline.NewStore()))

	request := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"image.png","content":"not accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("defect unsupported type status = %d, want %d", response.Code, http.StatusInternalServerError)
	}

	request = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"welcome.md","content":"accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("valid create status = %d, want unchanged %d", response.Code, http.StatusAccepted)
	}

	request = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"broken.md"`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON status = %d, want unchanged %d", response.Code, http.StatusBadRequest)
	}
}
