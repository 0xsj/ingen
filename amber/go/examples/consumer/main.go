package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	amber "github.com/0xsj/ingen/amber"
	amberhttp "github.com/0xsj/ingen/amber/adapters/http"
	amberlogging "github.com/0xsj/ingen/amber/adapters/logging"
	ambermessaging "github.com/0xsj/ingen/amber/adapters/messaging"
	amberotel "github.com/0xsj/ingen/amber/adapters/otel"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
	amberpostgres "github.com/0xsj/ingen/amber/adapters/storage/postgres"
	ambertracing "github.com/0xsj/ingen/amber/adapters/tracing"
	ambertransport "github.com/0xsj/ingen/amber/adapters/transport"
)

func main() {
	root, err := amber.Start()
	if err != nil {
		panic(err)
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		panic(err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
	if err != nil {
		panic(err)
	}
	outgoing, err := amberhttp.WithOutgoingRequest(request, child)
	if err != nil {
		panic(err)
	}
	encoded, err := ambertransport.EncodeValue(child)
	if err != nil {
		panic(err)
	}
	decoded, present, err := ambertransport.DecodeValue(encoded, amber.IncomingReject)
	if err != nil || !present || decoded.ExecutionID() != child.ExecutionID() {
		panic("transport public API round trip failed")
	}
	message, err := ambermessaging.WithOutgoingMessage(
		ambermessaging.Message[string]{Body: "order", Metadata: ambermessaging.Metadata{}},
		child,
	)
	if err != nil || message.Metadata[ambermessaging.ProvenanceKey] == "" {
		panic("messaging public API failed")
	}
	fields, err := amberlogging.ToFields(child)
	if err != nil || len(fields) == 0 {
		panic("logging public API failed")
	}
	traceAttributes, err := ambertracing.ToAttributes(child)
	if err != nil || len(traceAttributes) == 0 {
		panic("tracing public API failed")
	}
	otelAttributes, err := amberotel.ToAttributes(child)
	if err != nil || len(otelAttributes) == 0 {
		panic("OpenTelemetry public API failed")
	}
	store := amberstorage.NewMemoryStore()
	if err := store.Put(context.Background(), child); err != nil {
		panic(err)
	}
	stored, err := store.Get(context.Background(), child.ExecutionID())
	if err != nil || stored.ExecutionID() != child.ExecutionID() {
		panic("storage public API failed")
	}
	backend := amberstorage.NewMapKeyValueBackend()
	keyValueStore, err := amberstorage.NewKeyValueStore(backend, "consumer/provenance")
	if err != nil {
		panic(err)
	}
	if err := keyValueStore.Put(context.Background(), child); err != nil {
		panic(err)
	}
	keyValueStored, err := keyValueStore.Get(context.Background(), child.ExecutionID())
	if err != nil || keyValueStored.ExecutionID() != child.ExecutionID() {
		panic("generic storage public API failed")
	}
	if _, err := amberpostgres.NewPostgresStore(nil); !errors.Is(err, amber.ErrInvalidTransition) {
		panic("optional PostgreSQL package public API failed")
	}
	fmt.Printf("consumer header present: %t\n", outgoing.Header.Get(amberhttp.HeaderName) != "")
	fmt.Println("consumer public adapters present: true")
}
