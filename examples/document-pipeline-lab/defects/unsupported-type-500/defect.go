// Package unsupportedtype500 is a controlled defect variant of the document
// pipeline subject.
package unsupportedtype500

import (
	"bytes"
	"encoding/json"
	"net/http"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

// NewHandler wraps a subject and changes only the unsupported document type
// response from 400 Bad Request to 500 Internal Server Error. All other
// behavior is delegated unchanged.
func NewHandler(next http.Handler) http.Handler {
	if next == nil {
		next = documentpipeline.NewHandler(documentpipeline.NewStore())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffer := newResponseBuffer()
		next.ServeHTTP(buffer, r)
		if r.Method == http.MethodPost && r.URL.Path == "/documents" && buffer.status == http.StatusBadRequest && isUnsupportedType(buffer.body.Bytes()) {
			buffer.status = http.StatusInternalServerError
		}
		buffer.copyTo(w)
	})
}

func isUnsupportedType(body []byte) bool {
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
