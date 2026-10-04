package githubchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ingen/core/ciresult"
	"ingen/nublar/internal/delivery"
)

func TestPublishCreatesCompletedSuccessCheck(t *testing.T) {
	var received createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.URL.Path != "/repos/acme/ingen/commits/abc123/check-runs" || r.URL.Query().Get("check_name") != "Nublar / document" || r.URL.Query().Get("filter") != "all" || r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("list request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(0), CheckRuns: []checkRun{}})
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
		writeJSON(w, http.StatusCreated, checkRun{ID: 42, Name: received.Name, ExternalID: received.ExternalID, HeadSHA: "abc123", Status: "completed", Conclusion: received.Conclusion})
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
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(1), CheckRuns: []checkRun{{ID: 77, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123"}}})
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
			writeJSON(w, http.StatusOK, checkRun{ID: 77, Name: received.Name, ExternalID: received.ExternalID, HeadSHA: "abc123", Status: "completed", Conclusion: received.Conclusion})
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

func TestPublishRejectsPatchAcknowledgementForDifferentCheckID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(1), CheckRuns: []checkRun{{ID: 77, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123"}}})
		case http.MethodPatch:
			writeJSON(w, http.StatusOK, checkRun{ID: 78, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123", Status: "completed", Conclusion: "success"})
		default:
			t.Fatalf("unexpected %s request", r.Method)
		}
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("passed", 0)); err == nil || !strings.Contains(err.Error(), "unexpected identity") {
		t.Fatalf("Publish error = %v, want wrong PATCH target ID rejection", err)
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
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(0), CheckRuns: []checkRun{}})
			return
		}
		writeJSON(w, http.StatusCreated, checkRun{ID: 99, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123", Status: "completed", Conclusion: "success"})
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

func TestPublishFollowsBoundedLinkPaginationAndFindsMatchingRun(t *testing.T) {
	var listCount, patchCount int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCount++
			page := r.URL.Query().Get("page")
			if r.URL.Query().Get("filter") != "all" || r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("pagination query = %s", r.URL.RawQuery)
			}
			if page == "1" {
				runs := make([]checkRun, listPageSize)
				for i := range runs {
					runs[i] = checkRun{ID: int64(i + 1), Name: "Nublar / document", ExternalID: fmt.Sprintf("older-run-%d", i), HeadSHA: "abc123"}
				}
				nextQuery := url.Values{"check_name": {"Nublar / document"}, "filter": {"all"}, "per_page": {"100"}, "page": {"2"}}
				w.Header().Set("Link", fmt.Sprintf("<%s?%s>; rel=\"next\"", server.URL+r.URL.Path, nextQuery.Encode()))
				writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(101), CheckRuns: runs})
				return
			}
			if page != "2" {
				t.Fatalf("unexpected list page %q", page)
			}
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(101), CheckRuns: []checkRun{{ID: 101, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123", Status: "completed", Conclusion: "failure"}}})
		case http.MethodPatch:
			patchCount++
			var update updateRequest
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				t.Fatalf("decode update request: %v", err)
			}
			if r.URL.Path != "/repos/acme/ingen/check-runs/101" || update.Conclusion != "success" || update.Status != "completed" {
				t.Fatalf("reconciled update path/body = %s %+v", r.URL.Path, update)
			}
			writeJSON(w, http.StatusOK, checkRun{ID: 101, Name: update.Name, ExternalID: update.ExternalID, HeadSHA: "abc123", Status: "completed", Conclusion: "success"})
		case http.MethodPost:
			t.Fatalf("unexpected POST after finding a matching run")
		default:
			t.Fatalf("unexpected %s request", r.Method)
		}
	}))
	defer server.Close()

	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("passed", 0)); err != nil {
		t.Fatal(err)
	}
	if listCount != 2 || patchCount != 1 {
		t.Fatalf("list=%d patch=%d, want two pages and one update", listCount, patchCount)
	}
}

func TestPublishRejectsDuplicateIdentityAcrossListPage(t *testing.T) {
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			writeJSON(w, http.StatusCreated, checkRun{ID: 9, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123", Status: "completed", Conclusion: "success"})
			return
		}
		writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(2), CheckRuns: []checkRun{
			{ID: 7, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123"},
			{ID: 8, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123"},
		}})
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("passed", 0)); err == nil || !strings.Contains(err.Error(), "multiple GitHub check runs") {
		t.Fatalf("Publish error = %v, want duplicate identity rejection", err)
	}
	if posts != 0 {
		t.Fatalf("POST count = %d, want zero after duplicate identity", posts)
	}
}

func TestPublishReconcilesAmbiguousCreateWithoutRetryingPost(t *testing.T) {
	var created bool
	var posts, lists int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			lists++
			var runs []checkRun
			if created {
				runs = []checkRun{{ID: 44, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123", Status: "completed", Conclusion: "success"}}
			}
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(len(runs)), CheckRuns: runs})
			return
		}
		if r.Method == http.MethodPost {
			posts++
			created = true // The remote side effects before the response fails.
			writeJSON(w, http.StatusBadGateway, map[string]string{"message": "upstream failed after commit"})
			return
		}
		if r.Method == http.MethodPatch {
			var received updateRequest
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode reconciliation update: %v", err)
			}
			writeJSON(w, http.StatusOK, checkRun{ID: 44, Name: received.Name, ExternalID: received.ExternalID, HeadSHA: "abc123", Status: "completed", Conclusion: received.Conclusion})
			return
		}
		t.Fatalf("unexpected %s", r.Method)
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL, MaxAttempts: 4})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := publisher.Publish(context.Background(), testDecision("passed", 0))
	if err != nil {
		t.Fatalf("Publish after successful reconciliation: %v", err)
	}
	if receipt.Status != "accepted" || posts != 1 || lists != 2 {
		t.Fatalf("receipt=%+v posts=%d lists=%d, want reconciled acceptance without POST retry", receipt, posts, lists)
	}
}

func TestPublishDoesNotRetryAmbiguousCreateWhenReconciliationFindsNothing(t *testing.T) {
	var posts, lists int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			lists++
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(0), CheckRuns: []checkRun{}})
			return
		}
		posts++
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "uncertain"})
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL, MaxAttempts: 4})
	if err != nil {
		t.Fatal(err)
	}
	_, err = publisher.Publish(context.Background(), testDecision("passed", 0))
	if err == nil || !strings.Contains(err.Error(), "refusing to retry POST") {
		t.Fatalf("Publish error = %v, want explicit uncertain-create error", err)
	}
	if posts != 1 || lists != 2 {
		t.Fatalf("posts=%d lists=%d, want one POST and initial+reconcile lists", posts, lists)
	}
}

func TestPublishReconcilesLostCreateResponseAndUpdatesDecision(t *testing.T) {
	var created bool
	var posts, patches, lists int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			lists++
			var runs []checkRun
			if created {
				runs = []checkRun{{ID: 45, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "abc123", Status: "in_progress"}}
			}
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(len(runs)), CheckRuns: runs})
		case http.MethodPost:
			posts++
			created = true
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server does not support connection hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack create response: %v", err)
			}
			_ = conn.Close() // Simulate a committed request with a lost response.
		case http.MethodPatch:
			patches++
			var update updateRequest
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				t.Fatalf("decode reconciliation update: %v", err)
			}
			writeJSON(w, http.StatusOK, checkRun{ID: 45, Name: update.Name, ExternalID: update.ExternalID, HeadSHA: "abc123", Status: "completed", Conclusion: update.Conclusion})
		default:
			t.Fatalf("unexpected %s request", r.Method)
		}
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL, MaxAttempts: 5})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := publisher.Publish(context.Background(), testDecision("passed", 0))
	if err != nil {
		t.Fatalf("Publish after lost response reconciliation: %v", err)
	}
	if receipt.Status != "accepted" || posts != 1 || patches != 1 || lists != 2 {
		t.Fatalf("receipt=%+v posts=%d patches=%d lists=%d, want one POST, one update, two lists", receipt, posts, patches, lists)
	}
}

func TestPublishRequiresCommitAndDecisionAcknowledgement(t *testing.T) {
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(0), CheckRuns: []checkRun{}})
			return
		}
		posts++
		writeJSON(w, http.StatusCreated, checkRun{ID: 9, Name: "Nublar / document", ExternalID: "github-run-01", HeadSHA: "different-commit", Status: "completed", Conclusion: "failure"})
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = publisher.Publish(context.Background(), testDecision("passed", 0))
	if err == nil || !strings.Contains(err.Error(), "uncertain") {
		t.Fatalf("Publish error = %v, want acknowledgement mismatch failure", err)
	}
	if posts != 1 {
		t.Fatalf("POST count = %d, want one", posts)
	}
}

func TestPublishDisablesCallerSuppliedRedirects(t *testing.T) {
	var redirectCalls, targetCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(0), CheckRuns: []checkRun{}})
			return
		}
		if r.URL.Path == "/redirect-target" {
			targetCalls++
			t.Fatalf("redirect target called with auth header %q", r.Header.Get("Authorization"))
		}
		http.Redirect(w, r, "/redirect-target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		redirectCalls++
		return nil
	}}
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("passed", 0)); err == nil {
		t.Fatal("Publish succeeded through a redirect")
	}
	if redirectCalls != 0 || targetCalls != 0 {
		t.Fatalf("redirect callback=%d target=%d, want neither called", redirectCalls, targetCalls)
	}
}

func TestHTTPErrorDoesNotExposeResponseStatusText(t *testing.T) {
	const secret = "synthetic-token-in-status"
	publisher, err := New(Config{
		Repository: "acme/ingen",
		HeadSHA:    "abc123",
		CheckName:  "Nublar",
		Token:      "token",
		BaseURL:    "https://api.example.test",
		Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 " + secret,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("{}")),
				Request:    request,
			}, nil
		})},
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err = publisher.doJSONOnce(context.Background(), http.MethodGet, publisher.requestURL("/probe"), nil)
	if err == nil || !strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), secret) {
		t.Fatalf("HTTP error = %v, want numeric status without response text", err)
	}
}

func TestPublishBoundsResponseBody(t *testing.T) {
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			return
		}
		_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBodySize+1))
	}))
	defer server.Close()
	publisher, err := New(Config{Repository: "acme/ingen", HeadSHA: "abc123", CheckName: "Nublar / document", Token: "token", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), testDecision("passed", 0)); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("Publish error = %v, want bounded-body error", err)
	}
	if posts != 0 {
		t.Fatalf("POST count = %d, want zero after oversized list response", posts)
	}
}

func TestPublishMapsCoordinatorErrorToFailingCheck(t *testing.T) {
	var received createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, checkRunList{TotalCount: intValue(0), CheckRuns: []checkRun{}})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode create request: %v", err)
		}
		writeJSON(w, http.StatusCreated, checkRun{ID: 100, Name: received.Name, ExternalID: received.ExternalID, HeadSHA: "abc123", Status: "completed", Conclusion: received.Conclusion})
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

func TestNewBoundsCallerTransportConfiguration(t *testing.T) {
	publisher, err := New(Config{
		Repository:  "acme/ingen",
		HeadSHA:     "abc123",
		CheckName:   "Nublar",
		Client:      &http.Client{Timeout: time.Hour, CheckRedirect: func(*http.Request, []*http.Request) error { return nil }},
		MaxAttempts: 99,
		RetryDelay:  time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if publisher.client.Timeout != maxRequestTimeout || publisher.maxAttempts != maxRetryAttempts || publisher.retryDelay != maxRetryDelay {
		t.Fatalf("timeout=%s attempts=%d delay=%s, want bounded values", publisher.client.Timeout, publisher.maxAttempts, publisher.retryDelay)
	}
}

func TestNextPageURLRejectsUnsafeOrMutatedLinks(t *testing.T) {
	base, err := url.Parse("https://api.example.test")
	if err != nil {
		t.Fatal(err)
	}
	path := "/repos/acme/ingen/commits/abc123/check-runs"
	query := url.Values{"check_name": {"Nublar / document"}, "filter": {"all"}, "per_page": {"100"}, "page": {"1"}}
	validQuery := url.Values{"check_name": {"Nublar / document"}, "filter": {"all"}, "per_page": {"100"}, "page": {"2"}}
	for _, header := range []string{
		"<https://attacker.example" + path + "?" + validQuery.Encode() + ">; rel=\"next\"",
		"<https://api.example.test/other?" + validQuery.Encode() + ">; rel=\"next\"",
		"<https://api.example.test" + path + "?check_name=other&filter=all&per_page=100&page=2>; rel=\"next\"",
		"<https://api.example.test" + path + "?check_name=Nublar&check_name=Nublar&filter=all&per_page=100&page=2>; rel=\"next\"",
	} {
		if _, found, err := nextPageURL(header, base, path, query, 1); err == nil || found {
			t.Errorf("nextPageURL accepted %q (found=%v, err=%v)", header, found, err)
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

func intValue(value int) *int {
	return &value
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
