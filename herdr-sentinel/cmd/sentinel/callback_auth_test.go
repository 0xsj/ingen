package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/callbackauth"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestAuthenticatedHerdrFileIngressSignsAppliesAndReplays(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	keyDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keyDir, "operator.key")
	key := bytes.Repeat([]byte{0x5d}, callbackauth.MaxKeyBytes)
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema, RunID: "run-cli-auth", Status: "created",
		Workspace: sentinelrun.WorkspaceRef{ID: "workspace-cli-auth", Version: 3, File: ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)}},
		CreatedAt: "2026-10-04T09:10:00Z", UpdatedAt: "2026-10-04T09:10:00Z",
		Events: []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-10-04T09:10:00Z"}},
	}
	receiptPath := filepath.Join(root, "receipt.json")
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(root, "events.jsonl")
	eventBytes, err := json.Marshal(map[string]any{
		"schema": "ingen.herdr-event/v1", "event_id": "event-cli-auth", "run_id": receipt.RunID,
		"workspace_id": receipt.Workspace.ID, "workspace_version": receipt.Workspace.Version,
		"type": "role-launched", "at": "2026-10-04T09:10:01Z", "role": "backend-implementer",
		"workspace": ".sentinel/backend", "session_id": "session-cli-auth", "receipt_status": "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventsPath, append(eventBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	envelopePath := filepath.Join(root, "events.signed.json")
	stdout, stderr, code := captureSentinelStreams(t, func() int {
		return run([]string{"adapter", "herdr-sign-events", "--root", root, "--receipt", receiptPath, "--events", eventsPath, "--key", keyPath, "--sender", "local-adapter", "--output", envelopePath, "--lifetime-seconds", "300"})
	})
	if code != 0 || !strings.Contains(stdout, "local-adapter") || containsKeyMaterial(stdout+stderr, key) {
		t.Fatalf("sign command stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = captureSentinelStreams(t, func() int {
		return run([]string{"adapter", "herdr-auth-events", "--root", root, "--receipt", receiptPath, "--envelope", envelopePath, "--key", keyPath, "--expected-sender", "local-adapter"})
	})
	if code != 0 || !strings.Contains(stdout, "1 new events") || containsKeyMaterial(stdout+stderr, key) {
		t.Fatalf("ingest command stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	afterFirst, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = captureSentinelStreams(t, func() int {
		return run([]string{"adapter", "herdr-auth-events", "--root", root, "--receipt", receiptPath, "--envelope", envelopePath, "--key", keyPath, "--expected-sender", "local-adapter"})
	})
	if code != 0 || !strings.Contains(stdout, "0 new events") || containsKeyMaterial(stdout+stderr, key) {
		t.Fatalf("replay command stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	afterReplay, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterFirst, afterReplay) {
		t.Fatal("authenticated duplicate replay changed receipt bytes")
	}
	loaded, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "running" || len(loaded.Events) != 2 || loaded.Events[1].SourceID != "event-cli-auth" {
		t.Fatalf("authenticated receipt result = %#v", loaded)
	}
}

func containsKeyMaterial(output string, key []byte) bool {
	return strings.Contains(output, string(key)) || strings.Contains(output, base64.StdEncoding.EncodeToString(key)) || strings.Contains(output, hex.EncodeToString(key))
}

func TestAuthenticatedHerdrIngressRejectsWrongSenderWithoutReceiptMutation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	keyDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keyDir, "operator.key")
	key := bytes.Repeat([]byte{0x26}, callbackauth.MaxKeyBytes)
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema, RunID: "run-cli-auth", Status: "created",
		Workspace: sentinelrun.WorkspaceRef{ID: "workspace-cli-auth", Version: 3, File: ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)}},
		CreatedAt: "2026-10-04T09:10:00Z", UpdatedAt: "2026-10-04T09:10:00Z",
		Events: []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-10-04T09:10:00Z"}},
	}
	receiptPath := filepath.Join(root, "receipt.json")
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(root, "events.jsonl")
	eventBytes, _ := json.Marshal(map[string]any{
		"schema": "ingen.herdr-event/v1", "event_id": "event-cli-auth", "run_id": receipt.RunID,
		"workspace_id": receipt.Workspace.ID, "workspace_version": receipt.Workspace.Version,
		"type": "role-launched", "at": "2026-10-04T09:10:01Z",
	})
	if err := os.WriteFile(eventsPath, append(eventBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	envelopePath := filepath.Join(root, "events.signed.json")
	if _, _, code := captureSentinelStreams(t, func() int {
		return run([]string{"adapter", "herdr-sign-events", "--root", root, "--receipt", receiptPath, "--events", eventsPath, "--key", keyPath, "--sender", "local-adapter", "--output", envelopePath})
	}); code != 0 {
		t.Fatalf("sign command exit=%d", code)
	}
	before, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := captureSentinelStreams(t, func() int {
		return run([]string{"adapter", "herdr-auth-events", "--root", root, "--receipt", receiptPath, "--envelope", envelopePath, "--key", keyPath, "--expected-sender", "wrong-adapter"})
	})
	if code != 1 || stdout != "" || !strings.Contains(stderr, "sender") {
		t.Fatalf("wrong-sender result stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	after, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("wrong sender changed receipt bytes")
	}
}
