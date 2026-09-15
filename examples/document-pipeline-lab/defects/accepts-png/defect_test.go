package acceptspng

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectAcceptsOnlyPNGAmongUnsupportedTypes(t *testing.T) {
	handler := NewHandler(documentpipeline.NewHandler(documentpipeline.NewStore()))

	request := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"image.png","content":"not accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("PNG status = %d, want defect status %d", response.Code, http.StatusAccepted)
	}

	request = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"image.pdf","content":"not accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("PDF status = %d, want unchanged %d", response.Code, http.StatusBadRequest)
	}

	request = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"welcome.md","content":"accepted"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("Markdown status = %d, want unchanged %d", response.Code, http.StatusAccepted)
	}
}
