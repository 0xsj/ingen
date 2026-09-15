package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	amberhttp "github.com/0xsj/ingen/amber/adapters/http"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

func TestServiceHandlerAcceptsInboundProvenanceAndStoresChild(t *testing.T) {
	fixture := loadReferenceFixture(t)
	store := amberstorage.NewMemoryStore()
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/orders/42", nil)
	if err := amberhttp.SetOutgoingHeader(request.Header, root); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()

	newServiceHandler(store).ServeHTTP(recorder, request)

	if recorder.Code != fixture.HTTP.AcceptedStatus {
		t.Fatalf("service returned status %d, want %d", recorder.Code, fixture.HTTP.AcceptedStatus)
	}
	if recorder.Header().Get(amberhttp.HeaderName) == "" {
		t.Fatal("service response omitted Amber provenance header")
	}
	var body responseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Stored || body.ExecutionID == "" {
		t.Fatalf("service response = %+v, want stored execution", body)
	}
	stored, err := store.Get(request.Context(), amber.ID(body.ExecutionID))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Origin() != amber.Origin(fixture.HTTP.ChildOrigin) {
		t.Fatalf("stored origin = %q, want %q", stored.Origin(), fixture.HTTP.ChildOrigin)
	}
	if stored.Depth() != root.Depth()+uint64(fixture.HTTP.DepthIncrement) {
		t.Fatalf("stored depth = %d, want parent depth + %d", stored.Depth(), fixture.HTTP.DepthIncrement)
	}
	if fixture.HTTP.CorrelationPreserved && stored.CorrelationID() != root.CorrelationID() {
		t.Fatalf("stored correlation = %s, want %s", stored.CorrelationID(), root.CorrelationID())
	}
	causation, ok := stored.Causation()
	if !ok || causation.Kind != fixture.HTTP.CausationKind || causation.ID != root.ExecutionID() {
		t.Fatalf("stored causation = %+v, want execution %s", causation, root.ExecutionID())
	}
	responseProvenance, present, err := amberhttp.DecodeHeader(recorder.Header().Get(amberhttp.HeaderName), amber.IncomingReject)
	if err != nil || !present {
		t.Fatalf("service response provenance was not decodable: present=%v err=%v", present, err)
	}
	if fixture.HTTP.ExplicitChildPropagation && responseProvenance.ExecutionID() != stored.ExecutionID() {
		t.Fatalf("response execution = %s, want stored child %s", responseProvenance.ExecutionID(), stored.ExecutionID())
	}
}

func TestServiceHandlerRejectsMalformedInboundProvenance(t *testing.T) {
	fixture := loadReferenceFixture(t)
	store := amberstorage.NewMemoryStore()
	request := httptest.NewRequest(http.MethodGet, "http://example.test/orders/42", nil)
	request.Header.Set(amberhttp.HeaderName, "not-base64")
	recorder := httptest.NewRecorder()

	newServiceHandler(store).ServeHTTP(recorder, request)

	if recorder.Code != fixture.HTTP.RejectedStatus {
		t.Fatalf("service returned status %d, want %d", recorder.Code, fixture.HTTP.RejectedStatus)
	}
	if recorder.Header().Get(amberhttp.HeaderName) != "" {
		t.Fatal("rejected request returned an Amber provenance header")
	}
	if recorder.Body.String() != "invalid Amber provenance\n" {
		t.Fatalf("rejected request body = %q", recorder.Body.String())
	}
}

type referenceFixture struct {
	Version   int                       `json:"version"`
	HTTP      referenceFlowExpectations `json:"http"`
	Messaging referenceFlowExpectations `json:"messaging"`
}

type referenceFlowExpectations struct {
	AcceptedStatus           int    `json:"accepted_status"`
	RejectedStatus           int    `json:"rejected_status"`
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

func TestServiceHandlerAppliesTrustValidatorBeforeStorage(t *testing.T) {
	store := amberstorage.NewMemoryStore()
	trusted, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	validator := func(value amber.Provenance) error {
		if value.ExecutionID() != trusted.ExecutionID() {
			return fmt.Errorf("execution is not trusted")
		}
		return nil
	}
	handler := newServiceHandlerWithValidator(store, validator)

	trustedRequest := httptest.NewRequest(http.MethodGet, "http://example.test/orders/trusted", nil)
	if err := amberhttp.SetOutgoingHeader(trustedRequest.Header, trusted); err != nil {
		t.Fatal(err)
	}
	trustedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(trustedRecorder, trustedRequest)
	if trustedRecorder.Code != http.StatusOK {
		t.Fatalf("trusted request returned status %d, want %d", trustedRecorder.Code, http.StatusOK)
	}

	untrustedRequest := httptest.NewRequest(http.MethodGet, "http://example.test/orders/untrusted", nil)
	if err := amberhttp.SetOutgoingHeader(untrustedRequest.Header, untrusted); err != nil {
		t.Fatal(err)
	}
	untrustedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(untrustedRecorder, untrustedRequest)
	if untrustedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("untrusted request returned status %d, want %d", untrustedRecorder.Code, http.StatusBadRequest)
	}
	history, err := store.ListByWorkID(context.Background(), untrusted.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("untrusted request stored %d provenance values", len(history))
	}
}
