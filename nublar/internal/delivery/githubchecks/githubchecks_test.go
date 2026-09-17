package githubchecks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ingen/core/ciresult"
	"ingen/nublar/internal/delivery"
)

func TestPublishCreatesCompletedSuccessCheck(t *testing.T) {
	var received createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.URL.Path != "/repos/acme/ingen/commits/abc123/check-runs" || r.URL.Query().Get("check_name") != "Nublar / document" {
				t.Fatalf("list request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeJSON(w, http.StatusOK, checkRunList{})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/repos/acme/ingen/check-runs" {
			t.Fatalf("create request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Fatalf("request headers missing GitHub authentication/version: %v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode create request: %v", err)
		}
		writeJSON(w, http.StatusCreated, checkRun{ID: 42, Name: received.Name, ExternalID: received.ExternalID})
	}))
	defer server.Close()

	publisher, err := New(Config{
		Repository: "acme/ingen",
		HeadSHA:    "abc123",
		CheckName:  "Nublar / document",
		Token:      "token",
		BaseURL:    server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := publisher.Publish(context.Background(), testDecision("passed", 0))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || receipt.HTTPStatus != http.StatusCreated || receipt.Transport != transport {
		t.Fatalf("receipt = %+v, want accepted GitHub Checks receipt", receipt)
	}
	if received.Status != "completed" || received.Conclusion != "success" || received.ExternalID != "github-run-01" || received.HeadSHA != "abc123" {
		t.Fatalf("create request = %+v, want completed success check", received)
	}
	if received.Output.Title == "" || !strings.Contains(received.Output.Summary, "passed") || !strings.Contains(received.Output.Text, "behavior") {
		t.Fatalf("check output = %+v, want Nublar decision details", received.Output)
	}
}

func TestPublishUpdatesExistingCheckForSameRun(t *testing.T) {
	var patchCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, checkRunList{CheckRuns: []checkRun{{ID: 77, Name: "Nublar / document", ExternalID: "github-run-01"}}})
		case http.MethodPatch:
			patchCount++
			if r.URL.Path != "/repos/acme/ingen/check-runs/77" {
				t.Fatalf("update path = %s, want remote check 77", r.URL.Path)
			}
			var received updateRequest
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode update request: %v", err)
			}
			if received.ExternalID != "github-run-01" || received.Status != "completed" {
				t.Fatalf("update request = %+v, want same-run identity", received)
			}
			writeJSON(w, http.StatusOK, checkRun{ID: 77, Name: received.Name, ExternalID: received.ExternalID})
		default:
			t.Fatalf("unexpected %s request", r.Method)
		}
	}))
	defer server.Close()

	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("failed", 1)); err != nil {
		t.Fatal(err)
	}
	if patchCount != 1 {
		t.Fatalf("patch count = %d, want one update and no create", patchCount)
	}
}

func TestPublishRetriesTransientListFailure(t *testing.T) {
	var listCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if atomic.AddInt32(&listCount, 1) == 1 {
				writeJSON(w, http.StatusBadGateway, map[string]string{"message": "try again"})
				return
			}
			writeJSON(w, http.StatusOK, checkRunList{})
			return
		}
		writeJSON(w, http.StatusCreated, checkRun{ID: 99, Name: "Nublar / document", ExternalID: "github-run-01"})
	}))
	defer server.Close()

	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("passed", 0)); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&listCount); got != 2 {
		t.Fatalf("list requests = %d, want transient retry", got)
	}
}

func TestPublishMapsCoordinatorErrorToFailingCheck(t *testing.T) {
	var received createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, checkRunList{})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode create request: %v", err)
		}
		writeJSON(w, http.StatusCreated, checkRun{ID: 100, Name: received.Name, ExternalID: received.ExternalID})
	}))
	defer server.Close()

	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("error", 2)); err != nil {
		t.Fatal(err)
	}
	if received.Conclusion != "failure" || !strings.Contains(received.Output.Summary, "error") || !strings.Contains(received.Output.Summary, "2") {
		t.Fatalf("error check = %+v, want failing GitHub conclusion with Nublar error details", received)
	}
}

func TestPublishReturnsFailedReceiptWithoutToken(t *testing.T) {
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", BaseURL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := publisher.Publish(context.Background(), testDecision("passed", 0))
	if err == nil || receipt.Status != "failed" || receipt.Error == "" || receipt.Transport != transport {
		t.Fatalf("receipt=%+v err=%v, want failed missing-token receipt", receipt, err)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	for _, config := range []Config{
		{Repository: "acme", HeadSHA: "abc123", CheckName: "Nublar"},
		{Repository: "acme/ingen", CheckName: "Nublar"},
		{Repository: "acme/ingen", HeadSHA: "abc123"},
		{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar", BaseURL: "file:///tmp/github"},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("New(%+v) succeeded, want configuration error", config)
		}
	}
}

func testDecision(status string, exitCode int) delivery.Decision {
	return delivery.Decision{
		Schema: delivery.Schema,
		RunID:  "github-run-01",
		Workflow: delivery.Workflow{
			ID:   "document-pipeline",
			File: ciresult.FileRef{Path: "workflow.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Status:      status,
		ExitCode:    exitCode,
		CreatedAt:   "2026-09-17T10:00:00Z",
		CompletedAt: "2026-09-17T10:00:01Z",
		Checks: []delivery.Check{{
			ID:       "behavior",
			Tool:     "sorna",
			Path:     "sorna.json",
			Required: true,
			Status:   status,
			Result:   &ciresult.FileRef{Path: "sorna.json", SHA256: strings.Repeat("b", 64)},
		}},
	}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(err)
	}
}
