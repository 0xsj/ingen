package omitacceptedevent

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectRemovesOnlyAcceptedEventSignal(t *testing.T) {
	handler := NewHandler(documentpipeline.NewHandler(documentpipeline.NewStore()))
	request := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(`{"name":"welcome.md","content":"Read the contract."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("defect create status = %d, want %d", response.Code, http.StatusAccepted)
	}
	if got := strings.Join(response.Header().Values(eventHeader), ","); got != "document.queued" {
		t.Fatalf("defect event headers = %q, want document.queued", got)
	}
}
