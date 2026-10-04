package callbackauth

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestSignAndVerifyBatchBindsPayloadSenderIdentityAndWindow(t *testing.T) {
	root := t.TempDir()
	key := bytes.Repeat([]byte{0x5a}, MaxKeyBytes)
	now := time.Date(2026, time.July, 8, 9, 10, 11, 0, time.UTC)
	payload := fixturePayload(t, "event-1", "2026-07-08T09:10:12Z")
	receipt := callbackReceipt()
	envelope, err := SignBatch(root, receipt, payload, "local-signer", key, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyBatch(root, raw, key, "local-signer", receipt, now.Add(time.Second))
	if err != nil {
		t.Fatalf("verify signed batch: %v", err)
	}
	if verified.Envelope.KeyID != KeyID(key) || !bytes.Equal(verified.Payload, payload) || len(verified.Events) != 1 || verified.Events[0].EventID != "event-1" {
		t.Fatalf("verified batch lost authenticated fields: %#v", verified)
	}
	if _, err := VerifyBatch(root, raw, key, "other-sender", receipt, now.Add(time.Second)); err == nil {
		t.Fatal("verification accepted an unexpected configured sender")
	}
	wrongKey := bytes.Repeat([]byte{0x4b}, MaxKeyBytes)
	if _, err := VerifyBatch(root, raw, wrongKey, "local-signer", receipt, now.Add(time.Second)); err == nil {
		t.Fatal("verification accepted a different key")
	}
	if _, err := VerifyBatch(root, raw, key, "local-signer", receipt, now.Add(2*time.Minute)); err == nil {
		t.Fatal("verification accepted an expired batch")
	}
	wrongReceipt := receipt
	wrongReceipt.RunID = "another-run"
	if _, err := VerifyBatch(root, raw, key, "local-signer", wrongReceipt, now.Add(time.Second)); err == nil {
		t.Fatal("verification accepted a receipt with another run identity")
	}
	wrongReceipt = receipt
	wrongReceipt.Workspace.ID = "another-workspace"
	if _, err := VerifyBatch(root, raw, key, "local-signer", wrongReceipt, now.Add(time.Second)); err == nil {
		t.Fatal("verification accepted a receipt with another workspace identity")
	}
	wrongReceipt = receipt
	wrongReceipt.Workspace.Version++
	if _, err := VerifyBatch(root, raw, key, "local-signer", wrongReceipt, now.Add(time.Second)); err == nil {
		t.Fatal("verification accepted a receipt with another workspace version")
	}
}

func TestVerifyBatchRejectsDigestSignatureExtraDuplicateAndNullFields(t *testing.T) {
	root := t.TempDir()
	key := bytes.Repeat([]byte{0x31}, MaxKeyBytes)
	now := time.Date(2026, time.July, 8, 9, 10, 11, 0, time.UTC)
	envelope, err := SignBatch(root, callbackReceipt(), fixturePayload(t, "event-1", "2026-07-08T09:10:12Z"), "source-a", key, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("payload digest mismatch", func(t *testing.T) {
		bad := envelope
		bad.PayloadSHA256 = strings.Repeat("0", 64)
		verifyEnvelopeFails(t, root, bad, key, callbackReceipt(), now)
	})
	t.Run("signature mismatch", func(t *testing.T) {
		bad := envelope
		bad.MAC = strings.Repeat("0", 64)
		verifyEnvelopeFails(t, root, bad, key, callbackReceipt(), now)
	})
	t.Run("noncanonical base64 whitespace", func(t *testing.T) {
		bad := envelope
		bad.Payload = bad.Payload[:3] + "\n" + bad.Payload[3:]
		raw, err := json.Marshal(bad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyBatch(root, raw, key, "source-a", callbackReceipt(), now); err == nil {
			t.Fatal("verification accepted base64 with ignored newline whitespace")
		}
	})
	t.Run("unknown field", func(t *testing.T) {
		raw, _ := MarshalEnvelope(envelope)
		raw = bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"extra\": true,\n"), 1)
		if _, err := VerifyBatch(root, raw, key, "source-a", callbackReceipt(), now); err == nil {
			t.Fatal("verification accepted an unknown envelope field")
		}
	})
	t.Run("duplicate envelope field", func(t *testing.T) {
		raw, _ := MarshalEnvelope(envelope)
		raw = bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"sender\": \"source-a\",\n"), 1)
		if _, err := VerifyBatch(root, raw, key, "source-a", callbackReceipt(), now); err == nil {
			t.Fatal("verification accepted a duplicate envelope field")
		}
	})
	t.Run("case alias envelope field", func(t *testing.T) {
		raw, _ := MarshalEnvelope(envelope)
		raw = bytes.Replace(raw, []byte(`"sender": "source-a"`), []byte(`"Sender": "source-a"`), 1)
		if _, err := VerifyBatch(root, raw, key, "source-a", callbackReceipt(), now); err == nil {
			t.Fatal("verification accepted case-insensitive Sender alias")
		}
	})
	t.Run("case collision envelope fields", func(t *testing.T) {
		raw, _ := MarshalEnvelope(envelope)
		raw = bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"Sender\": \"source-a\",\n"), 1)
		if _, err := VerifyBatch(root, raw, key, "source-a", callbackReceipt(), now); err == nil {
			t.Fatal("verification accepted sender and Sender aliases together")
		}
	})
	t.Run("duplicate event field", func(t *testing.T) {
		payload := []byte(`{"schema":"ingen.herdr-event/v1","event_id":"event-1","run_id":"run-auth","workspace_id":"ws-auth","workspace_version":1,"type":"role-launched","at":"2026-07-08T09:10:12Z","event_id":"event-evil"}`)
		bad, err := SignBatch(root, callbackReceipt(), payload, "source-a", key, now, time.Minute)
		if err == nil {
			t.Fatalf("signing accepted duplicate event key: %#v", bad)
		}
	})
	t.Run("case alias event field", func(t *testing.T) {
		payload := []byte(`{"SCHEMA":"ingen.herdr-event/v1","event_id":"event-1","run_id":"run-auth","workspace_id":"ws-auth","workspace_version":1,"type":"role-launched","at":"2026-07-08T09:10:12Z"}`)
		if _, err := SignBatch(root, callbackReceipt(), payload, "source-a", key, now, time.Minute); err == nil {
			t.Fatal("signing accepted case-insensitive SCHEMA alias")
		}
	})
	t.Run("case collision event fields", func(t *testing.T) {
		payload := []byte(`{"schema":"ingen.herdr-event/v1","SCHEMA":"ingen.herdr-event/v1","event_id":"event-1","run_id":"run-auth","workspace_id":"ws-auth","workspace_version":1,"type":"role-launched","at":"2026-07-08T09:10:12Z"}`)
		if _, err := SignBatch(root, callbackReceipt(), payload, "source-a", key, now, time.Minute); err == nil {
			t.Fatal("signing accepted schema and SCHEMA aliases together")
		}
	})
	t.Run("null envelope field", func(t *testing.T) {
		raw, _ := MarshalEnvelope(envelope)
		raw = bytes.Replace(raw, []byte(`"sender": "source-a"`), []byte(`"sender": null`), 1)
		if _, err := VerifyBatch(root, raw, key, "source-a", callbackReceipt(), now); err == nil {
			t.Fatal("verification accepted null in a closed envelope")
		}
	})
}

func TestIngestFileAppliesWholeAuthenticatedBatchOnceAndPreservesReplay(t *testing.T) {
	root := t.TempDir()
	receiptPath := filepath.Join(root, "receipt.json")
	keyDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keyDir, "callback.key")
	envelopePath := filepath.Join(t.TempDir(), "events.signed.json")
	key := bytes.Repeat([]byte{0xa7}, MaxKeyBytes)
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sentinelrun.SaveFile(receiptPath, callbackReceipt()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := fixturePayload(t, "event-1", "2026-07-08T09:10:12Z")
	envelope, err := SignBatch(root, callbackReceipt(), payload, "test-sender", key, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	contents, _ := MarshalEnvelope(envelope)
	if err := os.WriteFile(envelopePath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	appended, err := IngestFile(receiptPath, envelopePath, root, keyPath, "test-sender", now.Add(time.Second))
	if err != nil || appended != 1 {
		t.Fatalf("first ingest appended=%d err=%v", appended, err)
	}
	afterFirst, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	appended, err = IngestFile(receiptPath, envelopePath, root, keyPath, "test-sender", now.Add(2*time.Second))
	if err != nil || appended != 0 {
		t.Fatalf("replay appended=%d err=%v", appended, err)
	}
	afterReplay, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterFirst, afterReplay) {
		t.Fatalf("idempotent replay changed receipt bytes")
	}
	loaded, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 || loaded.Events[1].SourceID != "event-1" || loaded.Status != "running" {
		t.Fatalf("receipt after signed batch = %#v", loaded)
	}
}

func TestIngestRechecksExpiryInsideLockedReceiptUpdate(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keyDir, "callback.key")
	key := bytes.Repeat([]byte{0x78}, MaxKeyBytes)
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, "receipt.json")
	if err := sentinelrun.SaveFile(receiptPath, callbackReceipt()); err != nil {
		t.Fatal(err)
	}
	issued := time.Now().UTC()
	envelope, err := SignBatch(root, callbackReceipt(), fixturePayload(t, "event-expiring", "2026-07-08T09:10:12Z"), "local-signer", key, issued, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelopePath := filepath.Join(root, "signed.json")
	if err := os.WriteFile(envelopePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	appended, err := ingestFileWithClock(receiptPath, envelopePath, root, keyPath, "local-signer", func() time.Time {
		return issued
	}, func() time.Time {
		return issued.Add(2 * time.Second)
	})
	if err == nil || appended != 0 || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired-at-lock batch result appended=%d err=%v", appended, err)
	}
	after, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("batch that expired while waiting for the receipt lock changed receipt bytes")
	}
}

func TestReadKeyRequiresPrivateOwnedRegularNonsymlinkFile(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x13}, MaxKeyBytes)
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, key, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadKey(path); err == nil {
		t.Fatal("ReadKey accepted a key accessible by group/other")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, keyID, err := ReadKey(path)
	if err != nil || !bytes.Equal(loaded, key) || keyID != KeyID(key) {
		t.Fatalf("ReadKey valid owner-only key: id=%q err=%v", keyID, err)
	}
	symlink := filepath.Join(dir, "key-link")
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadKey(symlink); err == nil {
		t.Fatal("ReadKey accepted a symlink")
	}
}

func TestSigningAndIngestRejectKeyInsideProjectAndNoncanonicalPaths(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "key")
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{1}, MaxKeyBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureKeyOutsideRoot(keyPath, root); err == nil {
		t.Fatal("accepted a key stored inside the selected project root")
	}
	if _, _, err := ReadKey(root + string(filepath.Separator) + "subdir" + string(filepath.Separator) + ".." + string(filepath.Separator) + "key"); err == nil {
		t.Fatal("accepted a noncanonical key path")
	}
	alias := filepath.Join(filepath.Dir(root), "key-root-alias")
	if err := os.Symlink(root, alias); err == nil {
		defer os.Remove(alias)
		if _, _, err := ReadKey(filepath.Join(alias, filepath.Base(keyPath))); err == nil {
			t.Fatal("accepted a key path with a symlinked parent component")
		}
	}
}

func TestSignFileDoesNotOverwriteExistingOutput(t *testing.T) {
	root := t.TempDir()
	receiptPath := filepath.Join(root, "receipt.json")
	keyDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keyDir, "key")
	eventsPath := filepath.Join(root, "events.jsonl")
	outputPath := filepath.Join(root, "signed.json")
	key := bytes.Repeat([]byte{0x21}, MaxKeyBytes)
	if err := sentinelrun.SaveFile(receiptPath, callbackReceipt()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	payload := fixturePayload(t, "event-1", "2026-07-08T09:10:12Z")
	if err := os.WriteFile(eventsPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = SignFile(root, receiptPath, eventsPath, keyPath, "source-a", outputPath, time.Now().UTC(), time.Minute)
	if err == nil {
		t.Fatal("SignFile overwrote an existing output")
	}
	contents, _ := os.ReadFile(outputPath)
	if string(contents) != "keep me" {
		t.Fatalf("existing output changed: %q", contents)
	}
}

func verifyEnvelopeFails(t *testing.T, root string, envelope Envelope, key []byte, receipt sentinelrun.Receipt, now time.Time) {
	t.Helper()
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBatch(root, raw, key, envelope.Sender, receipt, now); err == nil {
		t.Fatal("invalid signed envelope passed verification")
	}
}

func callbackReceipt() sentinelrun.Receipt {
	return sentinelrun.Receipt{
		Schema: sentinelrun.Schema, RunID: "run-auth", Status: "created",
		Workspace: sentinelrun.WorkspaceRef{ID: "ws-auth", Version: 1, File: ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)}},
		CreatedAt: "2026-07-08T09:10:00Z", UpdatedAt: "2026-07-08T09:10:00Z",
		Events: []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-07-08T09:10:00Z"}},
	}
}

func fixturePayload(t *testing.T, eventID, at string) []byte {
	t.Helper()
	contents, err := json.Marshal(map[string]any{
		"schema": "ingen.herdr-event/v1", "event_id": eventID, "run_id": "run-auth",
		"workspace_id": "ws-auth", "workspace_version": 1, "type": "role-launched", "at": at,
		"role": "backend-implementer", "workspace": ".sentinel/backend", "session_id": "session-auth",
		"receipt_status": "running", "outcome": "started",
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(contents, '\n')
}
