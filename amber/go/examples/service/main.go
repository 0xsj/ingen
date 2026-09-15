package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	amber "github.com/0xsj/ingen/amber"
	amberhttp "github.com/0xsj/ingen/amber/adapters/http"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

type responseBody struct {
	ExecutionID string `json:"execution_id"`
	Stored      bool   `json:"stored"`
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	storePath := filepath.Join(os.TempDir(), fmt.Sprintf("amber-service-%d.json", os.Getpid()))
	defer os.Remove(storePath)
	store, err := amberstorage.NewFileStore(storePath)
	if err != nil {
		return err
	}

	server := &http.Server{
		Handler:           newServiceHandler(store),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	var shutdownOnce sync.Once
	var shutdownErr error
	shutdown := func() error {
		shutdownOnce.Do(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				shutdownErr = err
				return
			}
			if err := <-serveErr; err != nil && err != http.ErrServerClosed {
				shutdownErr = err
			}
		})
		return shutdownErr
	}
	defer func() {
		_ = shutdown()
	}()

	root, err := amber.Start()
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/orders/42", nil)
	if err != nil {
		return err
	}
	outgoing, err := amberhttp.WithOutgoingRequest(request, root)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(outgoing)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("service returned status %d", response.StatusCode)
	}
	var body responseBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return err
	}
	if !body.Stored || body.ExecutionID == "" {
		return fmt.Errorf("service response did not confirm stored provenance")
	}
	stored, err := store.Get(context.Background(), amber.ID(body.ExecutionID))
	if err != nil {
		return err
	}
	if stored.Origin() != amber.OriginIncoming {
		return fmt.Errorf("stored origin = %q, want %q", stored.Origin(), amber.OriginIncoming)
	}
	if response.Header.Get(amberhttp.HeaderName) == "" {
		return fmt.Errorf("service response omitted Amber provenance header")
	}

	if err := shutdown(); err != nil {
		return err
	}
	fmt.Printf("service status: %d\n", response.StatusCode)
	fmt.Printf("service response provenance header present: %t\n", response.Header.Get(amberhttp.HeaderName) != "")
	fmt.Printf("stored service execution: %s\n", stored.ExecutionID())
	return nil
}

func newServiceHandler(store amberstorage.Store) http.Handler {
	return newServiceHandlerWithValidator(store, nil)
}

func newServiceHandlerWithValidator(store amberstorage.Store, validator amber.IncomingValidator) http.Handler {
	application := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		parent, ok := amber.ProvenanceFromContext(request.Context())
		if !ok {
			localParent, startErr := amber.Start()
			if startErr != nil {
				http.Error(writer, "create local provenance", http.StatusInternalServerError)
				return
			}
			parent = localParent
		}
		child, childErr := parent.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
		if childErr != nil {
			http.Error(writer, "derive request provenance", http.StatusInternalServerError)
			return
		}
		if storeErr := store.Put(request.Context(), child); storeErr != nil {
			http.Error(writer, "store request provenance", http.StatusInternalServerError)
			return
		}
		if headerErr := amberhttp.SetOutgoingHeader(writer.Header(), child); headerErr != nil {
			http.Error(writer, "set response provenance", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(writer).Encode(responseBody{
			ExecutionID: child.ExecutionID().String(),
			Stored:      true,
		})
	})
	return amberhttp.MiddlewareWithValidator(application, amber.IncomingReject, validator)
}
