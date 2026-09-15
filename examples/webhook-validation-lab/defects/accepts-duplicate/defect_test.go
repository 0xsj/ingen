package acceptsduplicate

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	webhookvalidation "ingen/examples/webhook-validation-lab/subject"
)

func TestDefectAcceptsDuplicateAsNewEvent(t *testing.T) {
	handler := NewHandler(webhookvalidation.NewHandler(webhookvalidation.NewStore()))

	first := postEvent(handler, `{"id":"evt-duplicate","type":"invoice.created","payload":{"invoice_id":"inv-003"}}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first delivery status = %d, want %d", first.Code, http.StatusAccepted)
	}

	second := postEvent(handler, `{"id":"evt-duplicate","type":"invoice.created","payload":{"invoice_id":"inv-003"}}`)
	if second.Code != http.StatusAccepted {
		t.Fatalf("duplicate delivery status = %d, want defect status %d", second.Code, http.StatusAccepted)
	}

	var body map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode duplicate response: %v", err)
	}
	if body["accepted"] != true || body["status"] != "accepted" {
		t.Fatalf("duplicate body = %#v, want accepted response", body)
	}
}

func TestDefectLeavesRejectionsUnchanged(t *testing.T) {
	handler := NewHandler(webhookvalidation.NewHandler(webhookvalidation.NewStore()))

	missingID := postEvent(handler, `{"type":"invoice.created","payload":{"invoice_id":"inv-002"}}`)
	if missingID.Code != http.StatusBadRequest {
		t.Fatalf("missing ID status = %d, want %d", missingID.Code, http.StatusBadRequest)
	}

	unsupported := postEvent(handler, `{"id":"evt-unsupported","type":"shipment.delayed","payload":{"shipment_id":"ship-001"}}`)
	if unsupported.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported type status = %d, want %d", unsupported.Code, http.StatusUnprocessableEntity)
	}
}

func postEvent(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/events", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
