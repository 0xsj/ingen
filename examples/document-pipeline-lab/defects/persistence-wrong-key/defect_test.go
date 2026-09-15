package persistencewrongkey

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func TestDefectMakesReturnedDocumentIDUnreadable(t *testing.T) {
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
	if created.ID != "mutation-discarded" {
		t.Fatalf("created ID = %q, want defect key", created.ID)
	}

	request = httptest.NewRequest(http.MethodGet, "/documents/"+created.ID, nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("lookup status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
