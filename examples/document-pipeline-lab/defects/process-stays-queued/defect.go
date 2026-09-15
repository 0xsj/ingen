// Package processstaysqueued is a controlled defect variant of the document
// pipeline subject.
package processstaysqueued

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

// NewHandler wraps a subject and changes the public process response to leave
// the document queued instead of reporting completed. All other behavior is
// delegated unchanged.
func NewHandler(next http.Handler) http.Handler {
	if next == nil {
		next = documentpipeline.NewHandler(documentpipeline.NewStore())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffer := newResponseBuffer()
		next.ServeHTTP(buffer, r)
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/documents/") && strings.HasSuffix(r.URL.Path, "/process") && buffer.status == http.StatusOK {
			var payload map[string]any
			if err := json.Unmarshal(buffer.body.Bytes(), &payload); err == nil && payload["status"] == "completed" {
				payload["status"] = "queued"
				buffer.body.Reset()
				_ = json.NewEncoder(&buffer.body).Encode(payload)
			}
		}
		buffer.copyTo(w)
	})
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
