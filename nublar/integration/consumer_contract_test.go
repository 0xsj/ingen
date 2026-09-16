package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"ingen/nublar/internal/delivery"
	"ingen/nublar/internal/delivery/webhook"
	nublarrun "ingen/nublar/internal/run"
	nublarstore "ingen/nublar/internal/storage/filesystem"
)

func TestConsumerContractCollectsStoresProjectsAndDelivers(t *testing.T) {
	workflowPath := filepath.Join("..", "testdata", "workflows", "mixed-producers.yaml")
	artifactRoot := filepath.Join("..", "testdata", "ci-results")
	correlation := &nublarrun.Correlation{System: "github-actions", ID: "build-42", Attempt: 3}
	record, err := nublarrun.CollectWorkflowFileWithCorrelation(workflowPath, artifactRoot, "consumer-run-01", correlation)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || record.ExitCode != 1 || record.Correlation == nil || *record.Correlation != *correlation {
		t.Fatalf("collected run = %+v, want failed correlated run", record)
	}

	store, err := nublarstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.RunID)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := delivery.Project(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "failed" || decision.ExitCode != 1 || decision.Correlation == nil || *decision.Correlation != *correlation {
		t.Fatalf("projected decision = %+v, want failed correlated decision", decision)
	}

	var received delivery.Decision
	var receivedKey string
	var receivedKeys []string
	requests := 0
	transport := integrationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		receivedKey = request.Header.Get("Idempotency-Key")
		receivedKeys = append(receivedKeys, receivedKey)
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s, want JSON POST", request.Method, request.URL)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode webhook decision: %v", err)
		}
		statusCode := http.StatusNoContent
		status := "204 No Content"
		if requests == 2 {
			statusCode = http.StatusBadGateway
			status = "502 Bad Gateway"
		}
		return &http.Response{
			StatusCode: statusCode,
			Status:     status,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})
	publisher, err := webhook.New("https://example.test/nublar", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := publisher.Publish(context.Background(), decision)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || receipt.HTTPStatus != http.StatusNoContent || receivedKey != record.RunID {
		t.Fatalf("receipt = %+v, idempotency key = %q, want accepted delivery for %q", receipt, receivedKey, record.RunID)
	}
	if received.RunID != record.RunID || received.Status != decision.Status || received.Correlation == nil || *received.Correlation != *correlation {
		t.Fatalf("received decision = %+v, want correlated delivery decision", received)
	}
	receiptStore, err := nublarstore.NewReceiptStore(filepath.Join(t.TempDir(), "receipts"))
	if err != nil {
		t.Fatal(err)
	}
	if err := receiptStore.Save(receipt); err != nil {
		t.Fatal(err)
	}
	storedReceipts, err := receiptStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(storedReceipts) != 1 || storedReceipts[0].Status != "accepted" || storedReceipts[0].RunID != record.RunID {
		t.Fatalf("stored receipts = %+v, want accepted receipt for %q", storedReceipts, record.RunID)
	}
	failedReceipt, publishErr := publisher.Publish(context.Background(), decision)
	if publishErr == nil {
		t.Fatal("second delivery succeeded, want controlled webhook failure")
	}
	if failedReceipt.Status != "failed" || failedReceipt.HTTPStatus != http.StatusBadGateway || failedReceipt.Error == "" || receivedKey != record.RunID {
		t.Fatalf("failed receipt = %+v, idempotency key = %q, want failed delivery for %q", failedReceipt, receivedKey, record.RunID)
	}
	if err := receiptStore.Save(failedReceipt); err != nil {
		t.Fatal(err)
	}
	storedReceipts, err = receiptStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(storedReceipts) != 2 {
		t.Fatalf("stored receipts after retry = %+v, want two delivery attempts", storedReceipts)
	}
	statuses := map[string]bool{}
	for _, storedReceipt := range storedReceipts {
		if storedReceipt.RunID != record.RunID {
			t.Fatalf("stored receipt = %+v, want receipt for %q", storedReceipt, record.RunID)
		}
		statuses[storedReceipt.Status] = true
	}
	if !statuses["accepted"] || !statuses["failed"] || len(receivedKeys) != 2 || receivedKeys[0] != record.RunID || receivedKeys[1] != record.RunID {
		t.Fatalf("retry evidence statuses=%+v keys=%v, want accepted/failed and repeated run ID", statuses, receivedKeys)
	}
	storedRun, err := store.Load(record.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != "failed" || storedRun.ExitCode != 1 {
		t.Fatalf("stored run after delivery retry = %+v, want unchanged failed run", storedRun)
	}
	encoded, err := json.Marshal(received)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "test.sorna-report") || strings.Contains(string(encoded), "test.paddock-report") {
		t.Fatalf("webhook payload contains producer-owned report: %s", encoded)
	}
}

type integrationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f integrationRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
