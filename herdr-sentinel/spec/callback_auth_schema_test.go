package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/adapter"
	"ingen/herdr-sentinel/internal/callbackauth"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestProducedSignedCallbackMatchesSchema(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema, RunID: "schema-callback-run", Status: "created",
		Workspace: sentinelrun.WorkspaceRef{ID: "schema-workspace", Version: 1, File: ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)}},
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
		Events: []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: now.Format(time.RFC3339Nano)}},
	}
	event := adapter.HerdrEvent{Schema: adapter.HerdrEventSchema, EventID: "schema-event", RunID: receipt.RunID,
		WorkspaceID: receipt.Workspace.ID, WorkspaceVersion: 1, Type: "role-launched", At: now.Format(time.RFC3339Nano),
		Role: "implementation", Workspace: "role", SessionID: "schema-session", ReceiptStatus: "running"}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := callbackauth.SignBatch(t.TempDir(), receipt, append(payload, '\n'), "operator-fixture", bytes.Repeat([]byte{0x1a}, 32), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	schema := compileNativeBoundarySchema(t, "herdr-signed-event-batch-v1.schema.json")
	if err := schema.Validate(boundaryDocument(t, envelope)); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"hmac_sha256", "key_id", "sender", "payload_base64", "expires_at"} {
		invalid := boundaryDocument(t, envelope)
		delete(invalid, field)
		if err := schema.Validate(invalid); err == nil {
			t.Fatalf("accepted absent %s", field)
		}
	}
	invalid := boundaryDocument(t, envelope)
	invalid["credential"] = "undeclared"
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("accepted an undeclared credential")
	}
}

func TestPublishedSignedCallbackBatch(t *testing.T) {
	path := os.Getenv("INGEN_SIGNED_CALLBACK_BATCH")
	if path == "" {
		t.Skip("set INGEN_SIGNED_CALLBACK_BATCH to validate a produced envelope")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	if err := compileNativeBoundarySchema(t, "herdr-signed-event-batch-v1.schema.json").Validate(document); err != nil {
		t.Fatal(err)
	}
}
