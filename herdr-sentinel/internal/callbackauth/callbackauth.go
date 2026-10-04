// Package callbackauth provides opt-in HMAC authentication for Sentinel's
// offline Herdr callback file ingress. It authenticates possession of a
// shared local key; it does not attest Herdr, a host process, or an application.
package callbackauth

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ingen/herdr-sentinel/internal/adapter"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

const (
	Schema           = "ingen.herdr-signed-event-batch/v1"
	MaxKeyBytes      = 32
	MaxPayloadBytes  = 4 << 20
	MaxEnvelopeBytes = 6 << 20
	MaxEvents        = 1000
	MaxLifetime      = 5 * time.Minute
	ClockSkew        = 30 * time.Second
)

const macDomain = "ingen.sentinel.herdr-event-batch.hmac-sha256.v1"

var envelopeFields = keySet("schema", "sender", "key_id", "run_id", "workspace_id", "workspace_version", "issued_at", "expires_at", "payload_sha256", "payload_base64", "hmac_sha256")
var eventFields = keySet("schema", "event_id", "run_id", "workspace_id", "workspace_version", "type", "at", "role", "workspace", "session_id", "receipt_status", "artifact_ids", "outcome", "reason")

// Envelope is a closed signed batch. Payload is the base64 encoding of the
// exact JSONL bytes whose SHA-256 and MAC are checked before receipt mutation.
type Envelope struct {
	Schema           string `json:"schema"`
	Sender           string `json:"sender"`
	KeyID            string `json:"key_id"`
	RunID            string `json:"run_id"`
	WorkspaceID      string `json:"workspace_id"`
	WorkspaceVersion int64  `json:"workspace_version"`
	IssuedAt         string `json:"issued_at"`
	ExpiresAt        string `json:"expires_at"`
	PayloadSHA256    string `json:"payload_sha256"`
	Payload          string `json:"payload_base64"`
	MAC              string `json:"hmac_sha256"`
}

// Verified holds parsed event values and the authenticated exact payload.
type Verified struct {
	Envelope Envelope
	Payload  []byte
	Events   []adapter.HerdrEvent
}

// ReadKey loads an operator-provisioned 32-byte key from a private regular
// file. The final path component must not be a symlink; group/other access and
// ownership by another effective user are rejected.
func ReadKey(path string) ([]byte, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", fmt.Errorf("callback key path is required")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, "", fmt.Errorf("callback key path must be a clean absolute path")
	}
	if err := rejectSymlinkPath(path); err != nil {
		return nil, "", fmt.Errorf("callback key path: %w", err)
	}
	contents, err := readPrivateKeyFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(contents) != MaxKeyBytes {
		return nil, "", fmt.Errorf("callback key must contain exactly %d raw bytes", MaxKeyBytes)
	}
	return contents, KeyID(contents), nil
}

// KeyID is a stable, nonsecret identifier for a shared key.
func KeyID(key []byte) string {
	digest := sha256.Sum256(key)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// SignBatch validates a JSONL batch against the current receipt before
// authenticating it. This is a local operator signing utility, not a Herdr
// callback signer.
func SignBatch(root string, receipt sentinelrun.Receipt, payload []byte, sender string, key []byte, now time.Time, ttl time.Duration) (Envelope, error) {
	if err := validateSigner(sender, key, ttl); err != nil {
		return Envelope{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if len(payload) == 0 || len(payload) > MaxPayloadBytes {
		return Envelope{}, fmt.Errorf("callback payload size must be between 1 and %d bytes", MaxPayloadBytes)
	}
	events, err := parseEvents(payload)
	if err != nil {
		return Envelope{}, err
	}
	if err := validateBatchAgainstReceipt(root, receipt, events); err != nil {
		return Envelope{}, err
	}
	issued := now.UTC()
	expires := issued.Add(ttl)
	digest := sha256.Sum256(payload)
	envelope := Envelope{
		Schema: Schema, Sender: sender, KeyID: KeyID(key), RunID: receipt.RunID,
		WorkspaceID: receipt.Workspace.ID, WorkspaceVersion: receipt.Workspace.Version,
		IssuedAt: issued.Format(time.RFC3339Nano), ExpiresAt: expires.Format(time.RFC3339Nano),
		PayloadSHA256: hex.EncodeToString(digest[:]), Payload: base64.StdEncoding.EncodeToString(payload),
	}
	envelope.MAC = MAC(key, envelope, payload)
	return envelope, nil
}

// VerifyBatch validates the envelope, key possession, bounded time window,
// receipt identity, JSONL events, and all event transitions/artifact hashes.
func VerifyBatch(root string, raw []byte, key []byte, expectedSender string, receipt sentinelrun.Receipt, now time.Time) (Verified, error) {
	envelope, err := parseEnvelope(raw)
	if err != nil {
		return Verified{}, err
	}
	if len(key) != MaxKeyBytes {
		return Verified{}, fmt.Errorf("callback key must contain exactly %d bytes", MaxKeyBytes)
	}
	if !validSender(expectedSender) || envelope.Sender != expectedSender {
		return Verified{}, fmt.Errorf("callback sender does not match the configured sender")
	}
	if envelope.Schema != Schema {
		return Verified{}, fmt.Errorf("callback envelope schema must be %s", Schema)
	}
	if envelope.KeyID != KeyID(key) {
		return Verified{}, fmt.Errorf("callback key ID does not match the configured key")
	}
	if envelope.RunID != receipt.RunID || envelope.WorkspaceID != receipt.Workspace.ID || envelope.WorkspaceVersion != receipt.Workspace.Version {
		return Verified{}, fmt.Errorf("callback run/workspace identity does not match the receipt")
	}
	if envelope.RunID == "" || envelope.WorkspaceID == "" || envelope.WorkspaceVersion < 1 {
		return Verified{}, fmt.Errorf("callback run/workspace identity is incomplete")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if err := checkWindow(envelope, now.UTC()); err != nil {
		return Verified{}, err
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(envelope.Payload)
	if err != nil || len(payload) == 0 || len(payload) > MaxPayloadBytes {
		return Verified{}, fmt.Errorf("callback payload encoding or size is invalid")
	}
	if base64.StdEncoding.EncodeToString(payload) != envelope.Payload {
		return Verified{}, fmt.Errorf("callback payload encoding is not canonical base64")
	}
	digest := sha256.Sum256(payload)
	if envelope.PayloadSHA256 != hex.EncodeToString(digest[:]) {
		return Verified{}, fmt.Errorf("callback payload digest does not match")
	}
	expectedMAC, err := hex.DecodeString(envelope.MAC)
	if err != nil || len(expectedMAC) != sha256.Size {
		return Verified{}, fmt.Errorf("callback HMAC encoding is invalid")
	}
	actualMAC, _ := hex.DecodeString(MAC(key, envelope, payload))
	if !hmac.Equal(expectedMAC, actualMAC) {
		return Verified{}, fmt.Errorf("callback HMAC verification failed")
	}
	events, err := parseEvents(payload)
	if err != nil {
		return Verified{}, err
	}
	if err := validateBatchAgainstReceipt(root, receipt, events); err != nil {
		return Verified{}, err
	}
	return Verified{Envelope: envelope, Payload: append([]byte(nil), payload...), Events: events}, nil
}

func checkWindow(envelope Envelope, now time.Time) error {
	issued, err := time.Parse(time.RFC3339Nano, envelope.IssuedAt)
	if err != nil {
		return fmt.Errorf("callback issued_at must be RFC3339: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, envelope.ExpiresAt)
	if err != nil {
		return fmt.Errorf("callback expires_at must be RFC3339: %w", err)
	}
	now = now.UTC()
	if issued.After(now.Add(ClockSkew)) || !expires.After(issued) || expires.Sub(issued) > MaxLifetime || !now.Before(expires) {
		return fmt.Errorf("callback issuance window is invalid or expired")
	}
	return nil
}

// IngestFile verifies the entire signed file before entering the receipt's
// single locked update. The locked callback rechecks live receipt identity and
// applies every event atomically, retaining legacy idempotent replay behavior.
func IngestFile(receiptPath, envelopePath, root, keyPath, expectedSender string, now time.Time) (int, error) {
	return ingestFileWithClock(receiptPath, envelopePath, root, keyPath, expectedSender, func() time.Time {
		if now.IsZero() {
			return time.Now().UTC()
		}
		return now
	}, time.Now)
}

func ingestFileWithClock(receiptPath, envelopePath, root, keyPath, expectedSender string, verifyNow, lockedNow func() time.Time) (int, error) {
	if verifyNow == nil || lockedNow == nil {
		return 0, fmt.Errorf("callback clock is unavailable")
	}
	if err := ensureKeyOutsideRoot(keyPath, root); err != nil {
		return 0, err
	}
	key, _, err := ReadKey(keyPath)
	if err != nil {
		return 0, err
	}
	raw, err := readRegularBounded(envelopePath, MaxEnvelopeBytes)
	if err != nil {
		return 0, fmt.Errorf("read signed callback: %w", err)
	}
	receipt, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		return 0, fmt.Errorf("load callback receipt: %w", err)
	}
	verified, err := VerifyBatch(root, raw, key, expectedSender, receipt, verifyNow())
	if err != nil {
		return 0, err
	}
	appended := 0
	_, err = sentinelrun.UpdateFile(receiptPath, func(live *sentinelrun.Receipt) (bool, error) {
		if err := checkWindow(verified.Envelope, lockedNow().UTC()); err != nil {
			return false, err
		}
		if live.RunID != verified.Envelope.RunID || live.Workspace.ID != verified.Envelope.WorkspaceID || live.Workspace.Version != verified.Envelope.WorkspaceVersion {
			return false, fmt.Errorf("callback identity no longer matches the live receipt")
		}
		count, err := adapter.ApplyHerdrEventsWithRoot(live, verified.Events, root)
		if err != nil {
			return false, err
		}
		appended = count
		return count > 0, nil
	})
	if err != nil {
		return 0, fmt.Errorf("ingest authenticated callback: %w", err)
	}
	return appended, nil
}

// SignFile reads and validates a bounded event stream and receipt snapshot,
// signs the exact event bytes, then creates an exclusive 0600 output file.
func SignFile(root, receiptPath, eventsPath, keyPath, sender, outputPath string, now time.Time, ttl time.Duration) (Envelope, error) {
	if err := ensureKeyOutsideRoot(keyPath, root); err != nil {
		return Envelope{}, err
	}
	key, _, err := ReadKey(keyPath)
	if err != nil {
		return Envelope{}, err
	}
	payload, err := readRegularBounded(eventsPath, MaxPayloadBytes)
	if err != nil {
		return Envelope{}, fmt.Errorf("read event stream: %w", err)
	}
	receipt, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		return Envelope{}, fmt.Errorf("load callback receipt: %w", err)
	}
	envelope, err := SignBatch(root, receipt, payload, sender, key, now, ttl)
	if err != nil {
		return Envelope{}, err
	}
	contents, err := MarshalEnvelope(envelope)
	if err != nil {
		return Envelope{}, err
	}
	if err := WriteExclusive(outputPath, contents); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// MarshalEnvelope returns the closed envelope as indented JSON with a final
// newline. It does not write files or alter the authenticated payload bytes.
func MarshalEnvelope(envelope Envelope) ([]byte, error) {
	contents, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode signed callback: %w", err)
	}
	return append(contents, '\n'), nil
}

// WriteExclusive publishes already validated bytes without replacing an
// existing path. The caller owns cleanup of any path it explicitly created.
func WriteExclusive(path string, contents []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create callback output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create callback output exclusively: %w", err)
	}
	ownedInfo, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return fmt.Errorf("inspect callback output: %w", statErr)
	}
	removeOwned := func() {
		current, err := os.Lstat(path)
		if err == nil && os.SameFile(ownedInfo, current) {
			_ = os.Remove(path)
		}
	}
	if written, err := file.Write(contents); err != nil || written != len(contents) {
		_ = file.Close()
		removeOwned()
		if err != nil {
			return fmt.Errorf("write signed callback: %w", err)
		}
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		removeOwned()
		return fmt.Errorf("sync signed callback: %w", err)
	}
	if err := file.Close(); err != nil {
		removeOwned()
		return fmt.Errorf("close signed callback: %w", err)
	}
	return nil
}

// MAC computes the HMAC over a domain-separated, length-prefixed field tuple
// and the exact payload bytes. Callers should normally use SignBatch.
func MAC(key []byte, envelope Envelope, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(macDomain))
	_, _ = mac.Write([]byte{0})
	fields := []string{
		envelope.Schema, envelope.Sender, envelope.KeyID, envelope.RunID, envelope.WorkspaceID,
		strconv.FormatInt(envelope.WorkspaceVersion, 10), envelope.IssuedAt, envelope.ExpiresAt,
		envelope.PayloadSHA256,
	}
	for _, field := range fields {
		writeFrame(mac, []byte(field))
	}
	writeFrame(mac, payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func writeFrame(writer hash.Hash, field []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(field)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(field)
}

func validateSigner(sender string, key []byte, ttl time.Duration) error {
	if !validSender(sender) {
		return fmt.Errorf("sender must be 1-128 ASCII letters, digits, or ._:@/- and start with a letter or digit")
	}
	if len(key) != MaxKeyBytes {
		return fmt.Errorf("callback key must contain exactly %d bytes", MaxKeyBytes)
	}
	if ttl <= 0 || ttl > MaxLifetime {
		return fmt.Errorf("callback lifetime must be between 1ns and %s", MaxLifetime)
	}
	return nil
}

func validSender(sender string) bool {
	if len(sender) == 0 || len(sender) > 128 || !asciiAlphaNum(sender[0]) {
		return false
	}
	for i := 0; i < len(sender); i++ {
		c := sender[i]
		if !asciiAlphaNum(c) && !strings.ContainsRune("._:@/-", rune(c)) {
			return false
		}
	}
	return true
}

func asciiAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func parseEnvelope(raw []byte) (Envelope, error) {
	if len(raw) == 0 || len(raw) > MaxEnvelopeBytes {
		return Envelope{}, fmt.Errorf("signed callback size must be between 1 and %d bytes", MaxEnvelopeBytes)
	}
	if err := rejectDuplicateKeysAndNull(raw); err != nil {
		return Envelope{}, fmt.Errorf("invalid signed callback JSON: %w", err)
	}
	if err := rejectObjectKeys(raw, envelopeFields); err != nil {
		return Envelope{}, fmt.Errorf("invalid signed callback JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope Envelope
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, fmt.Errorf("parse signed callback: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Envelope{}, fmt.Errorf("signed callback has trailing JSON")
		}
		return Envelope{}, fmt.Errorf("parse signed callback trailing data: %w", err)
	}
	if strings.TrimSpace(envelope.Sender) == "" || strings.TrimSpace(envelope.KeyID) == "" || strings.TrimSpace(envelope.RunID) == "" || strings.TrimSpace(envelope.WorkspaceID) == "" || envelope.WorkspaceVersion < 1 || envelope.IssuedAt == "" || envelope.ExpiresAt == "" || envelope.PayloadSHA256 == "" || envelope.Payload == "" || envelope.MAC == "" {
		return Envelope{}, fmt.Errorf("signed callback is missing a required field")
	}
	if !validSender(envelope.Sender) || len(envelope.KeyID) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(envelope.KeyID, "sha256:") || !isHex(envelope.KeyID[len("sha256:"):]) || !isSHA256(envelope.PayloadSHA256) || len(envelope.MAC) != sha256.Size*2 || !isHex(envelope.MAC) {
		return Envelope{}, fmt.Errorf("signed callback contains a malformed identifier or digest")
	}
	return envelope, nil
}

func parseEvents(payload []byte) ([]adapter.HerdrEvent, error) {
	if len(payload) == 0 || len(payload) > MaxPayloadBytes {
		return nil, fmt.Errorf("callback payload size must be between 1 and %d bytes", MaxPayloadBytes)
	}
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 64*1024), MaxPayloadBytes)
	events := make([]adapter.HerdrEvent, 0)
	for line := 1; scanner.Scan(); line++ {
		contents := bytes.TrimSpace(scanner.Bytes())
		if len(contents) == 0 {
			continue
		}
		if err := rejectDuplicateKeysAndNull(contents); err != nil {
			return nil, fmt.Errorf("callback event line %d has invalid JSON: %w", line, err)
		}
		if err := rejectObjectKeys(contents, eventFields); err != nil {
			return nil, fmt.Errorf("callback event line %d has invalid JSON: %w", line, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(contents))
		decoder.DisallowUnknownFields()
		var event adapter.HerdrEvent
		if err := decoder.Decode(&event); err != nil {
			return nil, fmt.Errorf("parse callback event line %d: %w", line, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("callback event line %d must contain exactly one JSON object", line)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("validate callback event line %d: %w", line, err)
		}
		events = append(events, event)
		if len(events) > MaxEvents {
			return nil, fmt.Errorf("callback event count exceeds %d", MaxEvents)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read callback JSONL: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("callback JSONL has no events")
	}
	return events, nil
}

func validateBatchAgainstReceipt(root string, receipt sentinelrun.Receipt, events []adapter.HerdrEvent) error {
	if receipt.RunID == "" || receipt.Workspace.ID == "" || receipt.Workspace.Version < 1 {
		return fmt.Errorf("receipt identity is incomplete")
	}
	for index, event := range events {
		if event.RunID != receipt.RunID || event.WorkspaceID != receipt.Workspace.ID || event.WorkspaceVersion != receipt.Workspace.Version {
			return fmt.Errorf("callback event %d run/workspace identity does not match the receipt", index+1)
		}
	}
	candidate := receipt
	if _, err := adapter.ApplyHerdrEventsWithRoot(&candidate, events, root); err != nil {
		return fmt.Errorf("validate callback event batch: %w", err)
	}
	return nil
}

func rejectDuplicateKeysAndNull(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := scanValue(decoder, first); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectObjectKeys(raw []byte, allowed map[string]struct{}) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	if object == nil {
		return fmt.Errorf("JSON value must be an object")
	}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown or noncanonical field %q", key)
		}
	}
	return nil
}

func keySet(fields ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		set[field] = struct{}{}
	}
	return set
}

func scanValue(decoder *json.Decoder, token json.Token) error {
	if token == nil {
		return fmt.Errorf("null values are not allowed")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = true
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := scanValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := scanValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func readRegularBounded(path string, maximum int64) ([]byte, error) {
	return readRegularFile(path, maximum)
}

func isSHA256(value string) bool { return len(value) == sha256.Size*2 && isHex(value) }

func isHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func rejectSymlinkPath(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	relative := strings.TrimPrefix(path, current)
	if volume != "" {
		relative = strings.TrimPrefix(path, volume+string(filepath.Separator))
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component %q is not allowed", current)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("path component %q is not a directory", current)
		}
	}
	return nil
}

func ensureKeyOutsideRoot(keyPath, root string) error {
	if !filepath.IsAbs(keyPath) || filepath.Clean(keyPath) != keyPath {
		return fmt.Errorf("callback key path must be a clean absolute path outside the project root")
	}
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve project root for callback key check: %w", err)
	}
	canonicalRoot, err = filepath.EvalSymlinks(canonicalRoot)
	if err != nil {
		return fmt.Errorf("resolve project root for callback key check: %w", err)
	}
	relative, err := filepath.Rel(canonicalRoot, keyPath)
	if err != nil {
		return fmt.Errorf("compare callback key to project root: %w", err)
	}
	if relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("callback key must be stored outside the selected project root")
	}
	return nil
}
