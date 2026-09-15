// Package webhookvalidation is the deliberately small HTTP subject used by
// the webhook-validation lab.
package webhookvalidation

import (
	"encoding/json"
	"net/http"
	"sync"
)

type event struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// Store retains the IDs accepted by the subject. It is intentionally smaller
// than a real event store because the lab tests the public idempotency
// behavior, not persistence infrastructure.
type Store struct {
	mu       sync.Mutex
	accepted map[string]struct{}
}

// NewStore returns an empty webhook event store.
func NewStore() *Store {
	return &Store{accepted: make(map[string]struct{})}
}

type handler struct {
	store *Store
}

// NewHandler returns the HTTP/JSON boundary for a webhook-validation subject.
func NewHandler(store *Store) http.Handler {
	if store == nil {
		store = NewStore()
	}
	return &handler{store: store}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.URL.Path == "/webhooks/events":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.receiveEvent(w, r)
	default:
		writeError(w, http.StatusNotFound, "route_not_found")
	}
}

func (h *handler) receiveEvent(w http.ResponseWriter, r *http.Request) {
	var input event
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil || input.ID == "" || input.Type == "" || input.Payload == nil {
		writeError(w, http.StatusBadRequest, "invalid_event")
		return
	}
	if input.Type != "invoice.created" && input.Type != "customer.updated" {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_event_type")
		return
	}

	h.store.mu.Lock()
	if _, exists := h.store.accepted[input.ID]; exists {
		h.store.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"duplicate": true,
			"event_id":  input.ID,
			"status":    "duplicate",
		})
		return
	}
	h.store.accepted[input.ID] = struct{}{}
	h.store.mu.Unlock()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"event_id": input.ID,
		"status":   "accepted",
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]map[string]string{
		"error": {"code": code},
	})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}
