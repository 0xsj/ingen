package main

import (
	"context"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
	ambermessaging "github.com/0xsj/ingen/amber/adapters/messaging"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	store := amberstorage.NewMemoryStore()
	root, err := amber.Start()
	if err != nil {
		return err
	}
	outgoing, err := ambermessaging.WithOutgoingMessage(
		ambermessaging.Message[string]{Body: "order", Metadata: ambermessaging.Metadata{"queue": "orders"}},
		root,
	)
	if err != nil {
		return err
	}

	validator := func(value amber.Provenance) error {
		if value.ExecutionID() != root.ExecutionID() {
			return fmt.Errorf("execution is not trusted")
		}
		return nil
	}
	handler := newConsumerHandler(store, validator)
	processed, err := handler(context.Background(), outgoing)
	if err != nil {
		return err
	}
	decoded, present, err := ambermessaging.DecodeMetadata(processed.Metadata, amber.IncomingReject)
	if err != nil || !present {
		return fmt.Errorf("decode processed metadata: %w", err)
	}
	if processed.Body != "order-processed" {
		return fmt.Errorf("processed body = %q", processed.Body)
	}
	stored, err := store.Get(context.Background(), decoded.ExecutionID())
	if err != nil {
		return err
	}
	if stored.Origin() != amber.OriginIncoming {
		return fmt.Errorf("stored origin = %q, want %q", stored.Origin(), amber.OriginIncoming)
	}
	causation, ok := stored.Causation()
	if !ok || causation.ID != root.ExecutionID() {
		return fmt.Errorf("stored causation = %+v, want execution %s", causation, root.ExecutionID())
	}

	fmt.Printf("consumer body: %s\n", processed.Body)
	fmt.Printf("consumer response provenance metadata present: %t\n", processed.Metadata[ambermessaging.ProvenanceKey] != "")
	fmt.Printf("stored consumer execution: %s\n", stored.ExecutionID())
	return nil
}

func newConsumerHandler(store amberstorage.Store, validator amber.IncomingValidator) ambermessaging.Handler[string] {
	return ambermessaging.MiddlewareWithValidator(func(ctx context.Context, message ambermessaging.Message[string]) (ambermessaging.Message[string], error) {
		parent, ok := amber.ProvenanceFromContext(ctx)
		if !ok {
			localParent, err := amber.Start()
			if err != nil {
				return ambermessaging.Message[string]{}, err
			}
			parent = localParent
		}
		child, err := parent.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
		if err != nil {
			return ambermessaging.Message[string]{}, err
		}
		if err := store.Put(ctx, child); err != nil {
			return ambermessaging.Message[string]{}, err
		}
		return ambermessaging.WithOutgoingMessage(ambermessaging.Message[string]{
			Body:     message.Body + "-processed",
			Metadata: ambermessaging.Metadata{"reply": "yes"},
		}, child)
	}, amber.IncomingReject, validator)
}
