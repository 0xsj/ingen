package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	ambermessaging "github.com/0xsj/ingen/amber/adapters/messaging"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

func TestConsumerHandlerAcceptsTrustedMessageAndStoresChild(t *testing.T) {
	fixture := loadReferenceFixture(t)
	store := amberstorage.NewMemoryStore()
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ambermessaging.WithOutgoingMetadata(nil, root)
	if err != nil {
		t.Fatal(err)
	}
	validator := func(value amber.Provenance) error {
		if value.ExecutionID() != root.ExecutionID() {
			return fmt.Errorf("execution is not trusted")
		}
		return nil
	}

	output, err := newConsumerHandler(store, validator)(context.Background(), ambermessaging.Message[string]{
		Body:     "order",
		Metadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.Body != "order"+fixture.Messaging.ProcessedBodySuffix {
		t.Fatalf("output body = %q", output.Body)
	}
	child, present, err := ambermessaging.DecodeMetadata(output.Metadata, amber.IncomingReject)
	if err != nil || !present {
		t.Fatalf("output metadata missing provenance: present=%v err=%v", present, err)
	}
	if child.Origin() != amber.Origin(fixture.Messaging.ChildOrigin) {
		t.Fatalf("output origin = %q, want %q", child.Origin(), fixture.Messaging.ChildOrigin)
	}
	if child.Depth() != root.Depth()+uint64(fixture.Messaging.DepthIncrement) {
		t.Fatalf("output depth = %d, want parent depth + %d", child.Depth(), fixture.Messaging.DepthIncrement)
	}
	if fixture.Messaging.CorrelationPreserved && child.CorrelationID() != root.CorrelationID() {
		t.Fatalf("output correlation = %s, want %s", child.CorrelationID(), root.CorrelationID())
	}
	causation, ok := child.Causation()
	if !ok || causation.Kind != fixture.Messaging.CausationKind || causation.ID != root.ExecutionID() {
		t.Fatalf("output causation = %+v, want execution %s", causation, root.ExecutionID())
	}
	if fixture.Messaging.ExplicitChildPropagation && child.ExecutionID() != outputExecutionID(t, output.Metadata) {
		t.Fatal("explicit child metadata did not identify the stored child")
	}
	if _, err := store.Get(context.Background(), child.ExecutionID()); err != nil {
		t.Fatal(err)
	}
}

func outputExecutionID(t *testing.T, metadata ambermessaging.Metadata) amber.ID {
	t.Helper()
	decoded, present, err := ambermessaging.DecodeMetadata(metadata, amber.IncomingReject)
	if err != nil || !present {
		t.Fatalf("decode output metadata: present=%v err=%v", present, err)
	}
	return decoded.ExecutionID()
}

type referenceFixture struct {
	Version   int                       `json:"version"`
	HTTP      referenceFlowExpectations `json:"http"`
	Messaging referenceFlowExpectations `json:"messaging"`
}

type referenceFlowExpectations struct {
	ProcessedBodySuffix      string `json:"processed_body_suffix"`
	ChildOrigin              string `json:"child_origin"`
	DepthIncrement           int    `json:"depth_increment"`
	CausationKind            string `json:"causation_kind"`
	CorrelationPreserved     bool   `json:"correlation_preserved"`
	ExplicitChildPropagation bool   `json:"explicit_child_propagation"`
}

func loadReferenceFixture(t *testing.T) referenceFixture {
	t.Helper()
	data, err := os.ReadFile("../../../conformance/reference-v1.json")
	if err != nil {
		t.Fatalf("read reference fixture: %v", err)
	}
	var fixture referenceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode reference fixture: %v", err)
	}
	if fixture.Version != amber.Version {
		t.Fatalf("reference fixture version = %d, want %d", fixture.Version, amber.Version)
	}
	return fixture
}

func TestConsumerHandlerRejectsUntrustedMessageBeforeStorage(t *testing.T) {
	store := amberstorage.NewMemoryStore()
	trusted, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ambermessaging.WithOutgoingMetadata(nil, untrusted)
	if err != nil {
		t.Fatal(err)
	}
	validator := func(value amber.Provenance) error {
		if value.ExecutionID() != trusted.ExecutionID() {
			return fmt.Errorf("execution is not trusted")
		}
		return nil
	}

	_, err = newConsumerHandler(store, validator)(context.Background(), ambermessaging.Message[string]{
		Body:     "order",
		Metadata: metadata,
	})
	if err == nil {
		t.Fatal("untrusted message should be rejected")
	}
	history, err := store.ListByWorkID(context.Background(), untrusted.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("untrusted message stored %d provenance values", len(history))
	}
}
