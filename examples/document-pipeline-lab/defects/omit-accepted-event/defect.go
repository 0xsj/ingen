// Package omitacceptedevent is a controlled defect variant of the document
// pipeline subject.
package omitacceptedevent

import (
	"net/http"
	"strings"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

const eventHeader = "X-InGen-Event"

// NewHandler removes the public accepted-event signal from successful document
// creation while delegating all response bodies and statuses unchanged.
func NewHandler(next http.Handler) http.Handler {
	if next == nil {
		next = documentpipeline.NewHandler(documentpipeline.NewStore())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/documents" {
			next.ServeHTTP(&eventRemovingWriter{ResponseWriter: w}, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type eventRemovingWriter struct {
	http.ResponseWriter
}

func (w *eventRemovingWriter) WriteHeader(status int) {
	removeAcceptedEvent(w.Header())
	w.ResponseWriter.WriteHeader(status)
}

func (w *eventRemovingWriter) Write(contents []byte) (int, error) {
	removeAcceptedEvent(w.Header())
	return w.ResponseWriter.Write(contents)
}

func removeAcceptedEvent(headers http.Header) {
	values := headers.Values(eventHeader)
	headers.Del(eventHeader)
	for _, value := range values {
		for _, rawEvent := range strings.Split(value, ",") {
			event := strings.TrimSpace(rawEvent)
			if event == "" || event == "document.accepted" {
				continue
			}
			headers.Add(eventHeader, event)
		}
	}
}
