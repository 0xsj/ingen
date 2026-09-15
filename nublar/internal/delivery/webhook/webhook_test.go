package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/nublar/internal/delivery"
)

func TestPublishPostsDecisionWithIdempotencyKey(t *testing.T) {
	var received delivery.Decision
	var receivedKey string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s, want JSON POST", r.Method, r.Header.Get("Content-Type"))
		}
		if r.URL.String() != "https://example.test/nublar" {
			t.Errorf("request URL = %s, want webhook endpoint", r.URL)
		}
		receivedKey = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Status:     "204 No Content",
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	publisher, err := New("https://example.test/nublar", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	decision := testDecision()
	if err := publisher.Publish(context.Background(), decision); err != nil {
		t.Fatal(err)
	}
	if receivedKey != decision.RunID || received.Schema != delivery.Schema || received.Status != decision.Status || received.Checks[0].Result == nil {
		t.Fatalf("received decision = %+v with key %q, want provider-neutral decision", received, receivedKey)
	}
	encoded, err := json.Marshal(received)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "artifact") {
		t.Fatalf("webhook payload contains producer artifact: %s", encoded)
	}
}

func TestPublishRejectsNonSuccessResponse(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})
	publisher, err := New("https://example.test/nublar", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(context.Background(), testDecision()); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("Publish() = %v, want HTTP 502 error", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewRejectsUnsafeEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "file:///tmp/nublar", "https://user:pass@example.test/hook", "https://example.test/hook#fragment"} {
		if _, err := New(endpoint, nil); err == nil {
			t.Fatalf("New(%q) succeeded, want endpoint validation error", endpoint)
		}
	}
}

func testDecision() delivery.Decision {
	return delivery.Decision{
		Schema:      delivery.Schema,
		RunID:       "run-webhook-01",
		Workflow:    delivery.Workflow{ID: "webhook-workflow", File: ciresult.FileRef{Path: "workflow.yaml", SHA256: strings.Repeat("a", 64)}},
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		CompletedAt: "2026-09-15T12:00:01Z",
		Checks: []delivery.Check{{
			ID:       "behavior",
			Tool:     "sorna",
			Path:     "sorna.json",
			Required: true,
			Status:   "passed",
			Result:   &ciresult.FileRef{Path: "sorna.json", SHA256: strings.Repeat("b", 64)},
		}},
	}
}
