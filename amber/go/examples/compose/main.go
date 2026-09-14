package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	amber "github.com/0xsj/ingen/amber"
	amberhttp "github.com/0xsj/ingen/amber/adapters/http"
	amberlogging "github.com/0xsj/ingen/amber/adapters/logging"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
	ambertracing "github.com/0xsj/ingen/amber/adapters/tracing"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	root, err := amber.Start()
	if err != nil {
		return err
	}

	outbound, err := amberhttp.WithOutgoingRequest(
		httptest.NewRequest(http.MethodGet, "https://example.test/orders/42", nil),
		root,
	)
	if err != nil {
		return err
	}

	storePath := filepath.Join(os.TempDir(), fmt.Sprintf("amber-example-%d.json", os.Getpid()))
	defer os.Remove(storePath)
	store, err := amberstorage.NewFileStore(storePath)
	if err != nil {
		return err
	}

	application := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		incoming, ok := amber.ProvenanceFromContext(request.Context())
		if !ok {
			http.Error(writer, "missing provenance", http.StatusInternalServerError)
			return
		}
		child, childErr := incoming.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
		if childErr != nil {
			http.Error(writer, childErr.Error(), http.StatusInternalServerError)
			return
		}
		if childErr := store.Put(context.Background(), child); childErr != nil {
			http.Error(writer, childErr.Error(), http.StatusInternalServerError)
			return
		}

		logFields, childErr := amberlogging.MergeFields(map[string]any{"component": "example"}, child)
		if childErr != nil {
			http.Error(writer, childErr.Error(), http.StatusInternalServerError)
			return
		}
		traceAttributes, childErr := ambertracing.ToAttributes(child)
		if childErr != nil {
			http.Error(writer, childErr.Error(), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"child_execution_id": child.ExecutionID(),
			"log_fields":         logFields,
			"trace_attributes":   traceAttributes,
		})
	})

	server := amberhttp.Middleware(application, amber.IncomingReject)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, outbound)
	if recorder.Code != http.StatusOK {
		return fmt.Errorf("request returned status %d: %s", recorder.Code, recorder.Body.String())
	}

	fmt.Printf("response status: %d\n", recorder.Code)
	fmt.Printf("response provenance header present: %t\n", recorder.Header().Get(amberhttp.HeaderName) != "")
	fmt.Printf("stored child response: %s", recorder.Body.String())
	return nil
}
