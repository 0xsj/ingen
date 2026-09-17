// Package documentpipeline is the deliberately small HTTP subject used by
// the document-pipeline lab.
package documentpipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// MaxContentBytes is the document size limit stated by the lab contract.
const MaxContentBytes = 4096

type createRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type document struct {
	ID      string
	Name    string
	Content string
	Status  string
	Result  *Result
}

// Result contains metadata derived by the processing operation.
type Result struct {
	DocumentID    string `json:"document_id"`
	ContentSHA256 string `json:"content_sha256"`
	WordCount     int    `json:"word_count"`
}

// Store is the subject's in-memory persistence layer.
//
// It is intentionally not part of the public HTTP contract. The mutex keeps
// the example safe when a real HTTP server handles concurrent requests.
type Store struct {
	mu     sync.RWMutex
	docs   map[string]*document
	nextID uint64
}

// NewStore returns an empty document store.
func NewStore() *Store {
	return &Store{docs: make(map[string]*document)}
}

type handler struct {
	store *Store
}

// NewHandler returns the HTTP/JSON boundary for a document-pipeline subject.
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
	case r.URL.Path == "/__malcolm/reset":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.resetStore(w)
	case r.URL.Path == "/documents":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.createDocument(w, r)
	case strings.HasPrefix(r.URL.Path, "/documents/"):
		h.documentRoute(w, r)
	default:
		writeError(w, http.StatusNotFound, "route_not_found")
	}
}

// resetStore is a subject-owned fixture hook. It is intentionally outside the
// document API and exists only so Sorna can establish a fresh state boundary
// between independently executable cases.
func (h *handler) resetStore(w http.ResponseWriter) {
	h.store.mu.Lock()
	h.store.docs = make(map[string]*document)
	h.store.nextID = 0
	h.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) createDocument(w http.ResponseWriter, r *http.Request) {
	var input createRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	name := strings.TrimSpace(input.Name)
	extension := strings.ToLower(filepath.Ext(name))
	if name == "" || (extension != ".md" && extension != ".txt") {
		writeError(w, http.StatusBadRequest, "unsupported_document_type")
		return
	}
	if len([]byte(input.Content)) > MaxContentBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "document_too_large")
		return
	}

	h.store.mu.Lock()
	h.store.nextID++
	id := "doc-" + strconv.FormatUint(h.store.nextID, 10)
	h.store.docs[id] = &document{
		ID:      id,
		Name:    name,
		Content: input.Content,
		Status:  "queued",
	}
	h.store.mu.Unlock()

	writeJSONWithEvents(w, http.StatusAccepted, map[string]string{
		"id":     id,
		"name":   name,
		"status": "queued",
	}, "document.accepted", "document.queued")
}

func (h *handler) documentRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/documents/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.getDocument(w, parts[0])
		return
	}
	if len(parts) == 2 && parts[0] != "" {
		switch parts[1] {
		case "process":
			if r.Method != http.MethodPost {
				methodNotAllowed(w, http.MethodPost)
				return
			}
			h.processDocument(w, parts[0])
		case "result":
			if r.Method != http.MethodGet {
				methodNotAllowed(w, http.MethodGet)
				return
			}
			h.getResult(w, parts[0])
		default:
			writeError(w, http.StatusNotFound, "route_not_found")
		}
		return
	}
	writeError(w, http.StatusNotFound, "route_not_found")
}

func (h *handler) getDocument(w http.ResponseWriter, id string) {
	h.store.mu.RLock()
	doc, ok := h.store.docs[id]
	if ok {
		doc = &document{ID: doc.ID, Name: doc.Name, Status: doc.Status}
	}
	h.store.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "document_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     doc.ID,
		"name":   doc.Name,
		"status": doc.Status,
	})
}

func (h *handler) processDocument(w http.ResponseWriter, id string) {
	h.store.mu.Lock()

	doc, ok := h.store.docs[id]
	if !ok {
		h.store.mu.Unlock()
		writeError(w, http.StatusNotFound, "document_not_found")
		return
	}
	if doc.Status != "queued" {
		h.store.mu.Unlock()
		writeError(w, http.StatusConflict, "document_not_queued")
		return
	}

	digest := sha256.Sum256([]byte(doc.Content))
	doc.Result = &Result{
		DocumentID:    doc.ID,
		ContentSHA256: hex.EncodeToString(digest[:]),
		WordCount:     len(strings.Fields(doc.Content)),
	}
	doc.Status = "completed"
	response := map[string]string{
		"id":     doc.ID,
		"status": doc.Status,
	}
	h.store.mu.Unlock()
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) getResult(w http.ResponseWriter, id string) {
	h.store.mu.RLock()
	doc, ok := h.store.docs[id]
	if !ok {
		h.store.mu.RUnlock()
		writeError(w, http.StatusNotFound, "document_not_found")
		return
	}
	if doc.Result == nil {
		h.store.mu.RUnlock()
		writeError(w, http.StatusConflict, "document_not_completed")
		return
	}
	result := *doc.Result
	h.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONWithEvents(w http.ResponseWriter, status int, value any, events ...string) {
	for _, event := range events {
		w.Header().Add("X-InGen-Event", event)
	}
	writeJSON(w, status, value)
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
