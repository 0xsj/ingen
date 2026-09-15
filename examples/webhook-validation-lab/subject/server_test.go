package webhookvalidation

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAcceptsSupportedEvent(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodPost, "/webhooks/events", map[string]any{
		"id":      "evt-001",
		"type":    "invoice.created",
		"payload": map[string]any{"invoice_id": "inv-001", "amount": 4200},
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}
	var body struct {
		Accepted bool   `json:"accepted"`
		EventID  string `json:"event_id"`
		Status   string `json:"status"`
	}
	decodeJSON(t, response, &body)
	if !body.Accepted || body.EventID != "evt-001" || body.Status != "accepted" {
		t.Fatalf("body = %+v, want accepted event", body)
	}
}

func TestDuplicateEventIsIdempotent(t *testing.T) {
	handler := NewHandler(NewStore())
	body := map[string]any{
		"id":      "evt-duplicate",
		"type":    "invoice.created",
		"payload": map[string]any{"invoice_id": "inv-003"},
	}
	first := requestJSON(t, handler, http.MethodPost, "/webhooks/events", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusAccepted)
	}
	second := requestJSON(t, handler, http.MethodPost, "/webhooks/events", body)
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, want %d", second.Code, http.StatusOK)
	}
	var response struct {
		Duplicate bool   `json:"duplicate"`
		EventID   string `json:"event_id"`
		Status    string `json:"status"`
	}
	decodeJSON(t, second, &response)
	if !response.Duplicate || response.EventID != "evt-duplicate" || response.Status != "duplicate" {
		t.Fatalf("duplicate body = %+v, want idempotent duplicate", response)
	}
}

func TestRejectsMissingEventID(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodPost, "/webhooks/events", map[string]any{
		"type":    "invoice.created",
		"payload": map[string]any{"invoice_id": "inv-002"},
	})
	assertErrorCode(t, response, http.StatusBadRequest, "invalid_event")
}

func TestRejectsUnsupportedEventType(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodPost, "/webhooks/events", map[string]any{
		"id":      "evt-unsupported",
		"type":    "shipment.delayed",
		"payload": map[string]any{"shipment_id": "ship-001"},
	})
	assertErrorCode(t, response, http.StatusUnprocessableEntity, "unsupported_event_type")
}

func TestHealthEndpointSupportsManagedSubjectReadiness(t *testing.T) {
	response := requestJSON(t, NewHandler(NewStore()), http.MethodGet, "/healthz", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
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
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSON(t, response, &body)
	if body.Error.Code != code {
		t.Fatalf("error code = %q, want %q", body.Error.Code, code)
	}
}
