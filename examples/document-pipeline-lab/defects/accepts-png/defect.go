// Package acceptspng is a controlled defect variant of the document pipeline
// subject.
package acceptspng

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

// NewHandler wraps a subject and accepts the unsupported PNG case at the
// public boundary. All other behavior is delegated unchanged.
func NewHandler(next http.Handler) http.Handler {
	if next == nil {
		next = documentpipeline.NewHandler(documentpipeline.NewStore())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptPNG := false
		if r.Method == http.MethodPost && r.URL.Path == "/documents" && r.Body != nil {
			contents, err := io.ReadAll(r.Body)
			if err == nil {
				r.Body = io.NopCloser(bytes.NewReader(contents))
				var input struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(contents, &input) == nil {
					acceptPNG = strings.HasSuffix(strings.ToLower(strings.TrimSpace(input.Name)), ".png")
				}
			}
		}
		buffer := newResponseBuffer()
		next.ServeHTTP(buffer, r)
		if acceptPNG && buffer.status == http.StatusBadRequest && isUnsupportedDocumentType(buffer.body.Bytes()) {
			buffer.status = http.StatusAccepted
			buffer.body.Reset()
			_ = json.NewEncoder(&buffer.body).Encode(map[string]string{
				"id":     "mutation-png",
				"name":   "image.png",
				"status": "queued",
			})
		}
		buffer.copyTo(w)
	})
}

func isUnsupportedDocumentType(body []byte) bool {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.Error.Code == "unsupported_document_type"
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
