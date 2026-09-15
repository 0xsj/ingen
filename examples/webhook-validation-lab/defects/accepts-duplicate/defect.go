// Package acceptsduplicate is a controlled webhook defect variant.
package acceptsduplicate

import (
	"bytes"
	"encoding/json"
	"net/http"

	webhookvalidation "ingen/examples/webhook-validation-lab/subject"
)

// NewHandler wraps a clean webhook subject and changes only the duplicate
// response. A repeated event is incorrectly reported as newly accepted.
func NewHandler(next http.Handler) http.Handler {
	if next == nil {
		next = webhookvalidation.NewHandler(webhookvalidation.NewStore())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffer := newResponseBuffer()
		next.ServeHTTP(buffer, r)
		if r.Method == http.MethodPost && r.URL.Path == "/webhooks/events" {
			buffer.rewriteDuplicate()
		}
		buffer.copyTo(w)
	})
}

func (buffer *responseBuffer) rewriteDuplicate() {
	if buffer.status != http.StatusOK {
		return
	}

	var duplicate struct {
		Duplicate bool   `json:"duplicate"`
		EventID   string `json:"event_id"`
	}
	if err := json.Unmarshal(buffer.body.Bytes(), &duplicate); err != nil || !duplicate.Duplicate {
		return
	}

	buffer.body.Reset()
	_ = json.NewEncoder(&buffer.body).Encode(map[string]any{
		"accepted": true,
		"event_id": duplicate.EventID,
		"status":   "accepted",
	})
	buffer.status = http.StatusAccepted
}

type responseBuffer struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponseBuffer() *responseBuffer {
	return &responseBuffer{header: make(http.Header)}
}

func (buffer *responseBuffer) Header() http.Header {
	return buffer.header
}

func (buffer *responseBuffer) WriteHeader(status int) {
	if buffer.status == 0 {
		buffer.status = status
	}
}

func (buffer *responseBuffer) Write(contents []byte) (int, error) {
	if buffer.status == 0 {
		buffer.status = http.StatusOK
	}
	return buffer.body.Write(contents)
}

func (buffer *responseBuffer) copyTo(w http.ResponseWriter) {
	for key, values := range buffer.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	status := buffer.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(buffer.body.Bytes())
}
