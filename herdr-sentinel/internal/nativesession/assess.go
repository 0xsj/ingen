package nativesession

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workspace"
)

const AssessmentSchema = "ingen.sentinel-native-recovery-assessment/v1"

const (
	AssessmentAssessed  = "assessed"
	AssessmentUncertain = "uncertain"
)

// Assessment contains read-only observations. Lease status is only a lock
// observation at AssessedAt; it is not proof that a process is alive or dead.
type Assessment struct {
	Schema              string             `json:"schema"`
	Status              string             `json:"status"`
	AssessedAt          time.Time          `json:"assessed_at"`
	Root                string             `json:"root"`
	JournalPath         string             `json:"journal_path"`
	JournalSHA256       string             `json:"journal_sha256"`
	SnapshotConsistency string             `json:"snapshot_consistency"`
	SessionID           string             `json:"session_id"`
	RunID               string             `json:"run_id"`
	WorkspaceID         string             `json:"workspace_id"`
	WorkspaceVersion    int64              `json:"workspace_version"`
	State               string             `json:"state"`
	ExecutionLease      LeaseObservation   `json:"execution_lease"`
	Receipt             ReceiptObservation `json:"receipt"`
	TerminalEvidence    TerminalEvidence   `json:"terminal_evidence"`
	RecommendedAction   string             `json:"recommended_action"`
	Recommendation      string             `json:"recommendation"`
	Limitations         []string           `json:"limitations"`
}

type LeaseObservation struct {
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type ReceiptObservation struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type TerminalEvidence struct {
	Status               string `json:"status"`
	StdoutPath           string `json:"stdout_path,omitempty"`
	StdoutSHA256         string `json:"stdout_sha256,omitempty"`
	ObservedStdoutSHA256 string `json:"observed_stdout_sha256,omitempty"`
	StderrPath           string `json:"stderr_path,omitempty"`
	StderrSHA256         string `json:"stderr_sha256,omitempty"`
	ObservedStderrSHA256 string `json:"observed_stderr_sha256,omitempty"`
	Detail               string `json:"detail,omitempty"`
}

const (
	assessmentMaxJSON   = 8 << 20
	assessmentMaxOutput = 256 << 20
)

var readOnlyLeaseProbe = inspectLeaseLock

// Assess reads a native journal and its durable evidence without touching
// host APIs or changing project files. The report is a time-bounded snapshot,
// not a recovery decision or a process/host attestation.
func Assess(root, journalPath string, now time.Time) (Assessment, error) {
	canonicalRoot, err := absoluteRoot(root)
	if err != nil {
		return Assessment{}, err
	}
	cleanPath, err := nativeRecordPath(journalPath)
	if err != nil {
		return Assessment{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	result := Assessment{
		Schema: AssessmentSchema, Status: AssessmentAssessed, AssessedAt: now,
		Root: canonicalRoot, JournalPath: cleanPath, SnapshotConsistency: "stable",
		ExecutionLease:   LeaseObservation{Status: "missing"},
		Receipt:          ReceiptObservation{Status: "uncertain"},
		TerminalEvidence: TerminalEvidence{Status: "not-terminal"},
		Limitations: []string{
			"This is a read-only observation; it does not contact Herdr, launch, signal, or recover a child.",
			"A free or missing execution lease does not establish whether a child ran or completed.",
			"For nonterminal journals the lease is never acquired or probed, so assessment cannot interfere with a wrapper claiming execution.",
			"Journal stability covers the journal bytes observed during this command; related files are not an atomic filesystem snapshot.",
			"Lease status is a nonblocking advisory-lock observation at assessment time, not process identity or liveness evidence.",
		},
	}

	firstBytes, firstErr := readRegularRooted(canonicalRoot, cleanPath, assessmentMaxJSON)
	if firstErr != nil {
		return Assessment{}, fmt.Errorf("read native journal snapshot: %w", firstErr)
	}
	firstHash := sha256Hex(firstBytes)
	loaded, loadErr := nativejournal.LoadSnapshot(canonicalRoot, cleanPath, firstBytes)
	if loadErr != nil {
		return Assessment{}, fmt.Errorf("load native journal: %w", loadErr)
	}
	if err := validateRecordLocation(cleanPath, loaded); err != nil {
		return Assessment{}, err
	}
	secondBytes, secondErr := readRegularRooted(canonicalRoot, cleanPath, assessmentMaxJSON)
	if secondErr != nil {
		return Assessment{}, fmt.Errorf("re-read native journal snapshot: %w", secondErr)
	}
	result.JournalSHA256 = firstHash
	result.SessionID = loaded.Intent.SessionID
	result.RunID = loaded.Intent.RunID
	result.WorkspaceID = loaded.Intent.WorkspaceID
	result.WorkspaceVersion = loaded.Intent.WorkspaceVersion
	result.State = loaded.State
	result.Receipt.Path = loaded.Intent.ReceiptPath
	if !bytes.Equal(firstBytes, secondBytes) || sha256Hex(secondBytes) != firstHash {
		result.Status = AssessmentUncertain
		result.SnapshotConsistency = "changed"
		result.RecommendedAction = "reassess"
		result.Recommendation = "The journal changed during assessment; repeat the read-only assessment before choosing a recovery action."
		return result, nil
	}

	result.ExecutionLease = observeLease(canonicalRoot, loaded.Intent.ExecutionLeasePath, terminalState(loaded.State))
	receipt, receiptBytes, receiptErr := readReceiptSnapshot(canonicalRoot, loaded)
	if receiptErr == nil {
		result.Receipt = ReceiptObservation{Status: "matched", Path: loaded.Intent.ReceiptPath, SHA256: sha256Hex(receiptBytes)}
		if err := validateReceiptSnapshot(canonicalRoot, &receipt, loaded); err != nil {
			result.Receipt.Status = "mismatched"
			result.Receipt.Detail = conciseError(err)
		}
	} else {
		status := "uncertain"
		if errors.Is(receiptErr, fs.ErrNotExist) {
			status = "missing"
		}
		result.Receipt = ReceiptObservation{Status: status, Path: loaded.Intent.ReceiptPath, Detail: conciseError(receiptErr)}
	}

	result.TerminalEvidence = inspectTerminalEvidence(canonicalRoot, loaded)
	finalBytes, finalErr := readRegularRooted(canonicalRoot, cleanPath, assessmentMaxJSON)
	if finalErr != nil || !bytes.Equal(firstBytes, finalBytes) || sha256Hex(finalBytes) != firstHash {
		result.Status = AssessmentUncertain
		result.SnapshotConsistency = "changed"
		result.RecommendedAction = "reassess"
		result.Recommendation = "The journal changed or could not be re-read after evidence checks; repeat the read-only assessment before choosing a recovery action."
		return result, nil
	}
	if loaded.State == nativejournal.StateIndeterminate || result.ExecutionLease.Status == "uncertain" || result.ExecutionLease.Status == "unsupported" || result.ExecutionLease.Status == "not-probed" ||
		result.Receipt.Status != "matched" ||
		(result.TerminalEvidence.Status != "matched" && result.TerminalEvidence.Status != "not-terminal" && result.TerminalEvidence.Status != "not-applicable") {
		result.Status = AssessmentUncertain
	}
	result.RecommendedAction, result.Recommendation = recommendationFor(result)
	return result, nil
}

func (assessment Assessment) WriteJSON(writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(assessment)
}

func readReceiptSnapshot(root string, record Record) (run.Receipt, []byte, error) {
	contents, err := readRegularRooted(root, record.Intent.ReceiptPath, assessmentMaxJSON)
	if err != nil {
		return run.Receipt{}, nil, err
	}
	if err := rejectDuplicateJSONKeys(contents); err != nil {
		return run.Receipt{}, nil, fmt.Errorf("parse Sentinel receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var receipt run.Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return run.Receipt{}, nil, fmt.Errorf("parse Sentinel receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return run.Receipt{}, nil, fmt.Errorf("parse Sentinel receipt: multiple JSON values")
		}
		return run.Receipt{}, nil, fmt.Errorf("parse Sentinel receipt trailing data: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return run.Receipt{}, nil, fmt.Errorf("validate Sentinel receipt: %w", err)
	}
	return receipt, contents, nil
}

func rejectDuplicateJSONKeys(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := scanAssessmentJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}

func scanAssessmentJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanAssessmentJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil {
			return err
		}
		if closeToken != json.Delim('}') {
			return fmt.Errorf("malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanAssessmentJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil {
			return err
		}
		if closeToken != json.Delim(']') {
			return fmt.Errorf("malformed JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func validateReceiptSnapshot(root string, receipt *run.Receipt, record Record) error {
	if receipt == nil {
		return fmt.Errorf("Sentinel receipt is required")
	}
	if receipt.RunID != record.Intent.RunID || receipt.Workspace.ID != record.Intent.WorkspaceID ||
		receipt.Workspace.Version != record.Intent.WorkspaceVersion ||
		receipt.Workspace.File.SHA256 != record.Intent.WorkspaceManifestSHA256 {
		return fmt.Errorf("native session does not match the Sentinel receipt run/workspace identity")
	}
	manifestBytes, err := readRegularRooted(root, receipt.Workspace.File.Path, assessmentMaxJSON)
	if err != nil {
		return fmt.Errorf("read receipt workspace manifest: %w", err)
	}
	if sha256Hex(manifestBytes) != record.Intent.WorkspaceManifestSHA256 {
		return fmt.Errorf("workspace manifest bytes do not match the native session intent")
	}
	manifest, err := workspace.LoadBytes(receipt.Workspace.File.Path, manifestBytes)
	if err != nil {
		return fmt.Errorf("validate receipt workspace manifest: %w", err)
	}
	if manifest.ID != record.Intent.WorkspaceID || manifest.Version != record.Intent.WorkspaceVersion {
		return fmt.Errorf("native session does not match the current workspace manifest identity")
	}
	var roleMatch bool
	for _, role := range manifest.Roles {
		if role.ID == record.Intent.RoleID && role.Kind == record.Intent.RoleKind && filepath.Clean(role.Workspace) == filepath.Clean(record.Intent.Workdir) {
			roleMatch = true
			break
		}
	}
	if !roleMatch {
		return fmt.Errorf("native session role identity no longer matches the workspace manifest")
	}
	return nil
}

func inspectTerminalEvidence(root string, record Record) TerminalEvidence {
	evidence := TerminalEvidence{Status: "not-terminal"}
	var terminal *nativejournal.Event
	for i := len(record.Events) - 1; i >= 0; i-- {
		if record.Events[i].Kind == nativejournal.KindWrapperTerminal {
			terminal = &record.Events[i]
			break
		}
	}
	if terminal == nil && !terminalState(record.State) {
		return evidence
	}
	if terminal == nil {
		// Pre-start cancellation and setup failure intentionally have no child
		// output digests. There is no process evidence to hash in those cases.
		if record.State == nativejournal.StateCanceled || record.State == nativejournal.StateFailed {
			evidence.Status = "not-applicable"
			return evidence
		}
		evidence.Status = "missing"
		evidence.Detail = "terminal journal state has no wrapper terminal output evidence"
		return evidence
	}
	evidence = TerminalEvidence{
		Status: "matched", StdoutPath: record.Intent.StdoutPath, StdoutSHA256: terminal.StdoutSHA256,
		StderrPath: record.Intent.StderrPath, StderrSHA256: terminal.StderrSHA256,
	}
	for _, item := range []struct{ path, expected string }{
		{record.Intent.StdoutPath, terminal.StdoutSHA256},
		{record.Intent.StderrPath, terminal.StderrSHA256},
	} {
		actual, err := hashRegularRooted(root, item.path, assessmentMaxOutput)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				evidence.Status = "missing"
			} else {
				evidence.Status = "uncertain"
			}
			evidence.Detail = conciseError(err)
			return evidence
		}
		if actual != item.expected {
			evidence.Status = "drift"
			if item.path == record.Intent.StdoutPath {
				evidence.ObservedStdoutSHA256 = actual
			} else {
				evidence.ObservedStderrSHA256 = actual
			}
			evidence.Detail = "a terminal output file no longer matches its journal digest"
			return evidence
		}
		if item.path == record.Intent.StdoutPath {
			evidence.ObservedStdoutSHA256 = actual
		} else {
			evidence.ObservedStderrSHA256 = actual
		}
	}
	return evidence
}

func observeLease(root, leasePath string, terminal bool) LeaseObservation {
	if leasePath == "" {
		return LeaseObservation{Status: "unsupported", Detail: "journal predates execution-lease tracking"}
	}
	if !terminal {
		exists, err := leasePathExists(root, leasePath)
		if err != nil {
			return LeaseObservation{Status: "uncertain", Path: leasePath, Detail: conciseError(err)}
		}
		if !exists {
			return LeaseObservation{Status: "missing", Path: leasePath, Detail: "no lease file exists; this does not establish child outcome"}
		}
		return LeaseObservation{Status: "not-probed", Path: leasePath, Detail: "nonterminal lease was not locked or probed to avoid interfering with wrapper execution"}
	}
	status, err := readOnlyLeaseProbe(root, leasePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LeaseObservation{Status: "missing", Path: leasePath, Detail: "no lease file exists; this does not establish child outcome"}
		}
		if strings.Contains(err.Error(), "unsupported") {
			return LeaseObservation{Status: "unsupported", Path: leasePath, Detail: conciseError(err)}
		}
		return LeaseObservation{Status: "uncertain", Path: leasePath, Detail: conciseError(err)}
	}
	return LeaseObservation{Status: status, Path: leasePath}
}

func leasePathExists(root, relative string) (bool, error) {
	clean, err := relativePath("execution lease", relative)
	if err != nil {
		return false, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return false, err
	}
	defer handle.Close()
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for i := range parts {
		partial := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		info, lstatErr := handle.Lstat(partial)
		if errors.Is(lstatErr, fs.ErrNotExist) {
			return false, nil
		}
		if lstatErr != nil {
			return false, lstatErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("execution lease path %q contains a symlink component", relative)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return false, fmt.Errorf("execution lease parent %q is not a directory", partial)
		}
	}
	return true, nil
}

func recommendationFor(a Assessment) (string, string) {
	if a.SnapshotConsistency != "stable" || a.Receipt.Status != "matched" || a.TerminalEvidence.Status == "drift" || a.TerminalEvidence.Status == "missing" || a.TerminalEvidence.Status == "uncertain" {
		return "manual-review", "Evidence is missing, changed, or mismatched. Preserve the current files and review them before taking a mutating recovery action."
	}
	if a.State == nativejournal.StateIndeterminate {
		return "manual-review", "The journal is indeterminate. Preserve all evidence and review the wrapper and host history before taking any mutating action."
	}
	if a.ExecutionLease.Status == "held" {
		return "wait-and-reassess", "A process currently holds the execution lease. This does not prove child liveness; wait and repeat the read-only assessment."
	}
	if a.ExecutionLease.Status != "free" {
		return "manual-review", "The execution lease could not be observed as free. Do not infer that no wrapper or child ran."
	}
	if terminalState(a.State) {
		return "collect-or-close", "The journal is terminal and its available output evidence matches. Use the explicit collection or cleanup command only after reviewing the receipt."
	}
	return "native-recover-review", "The journal is nonterminal and the lease is free at this instant. Review host identity and recovery evidence before any mutating recovery; do not relaunch from this assessment."
}

func readRegularRooted(root, relative string, max int64) ([]byte, error) {
	file, err := openRegularAssessmentFile(root, relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > max {
		return nil, fmt.Errorf("rooted file %q exceeds the %d-byte assessment limit", relative, max)
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("rooted file %q exceeds the %d-byte assessment limit", relative, max)
	}
	return data, nil
}

func hashRegularRooted(root, relative string, max int64) (string, error) {
	file, err := openRegularAssessmentFile(root, relative)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() < 0 || info.Size() > max {
		return "", fmt.Errorf("rooted output %q exceeds the %d-byte assessment limit", relative, max)
	}
	hasher := sha256.New()
	n, err := io.CopyN(hasher, file, max+1)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n > max {
		return "", fmt.Errorf("rooted output %q exceeds the %d-byte assessment limit", relative, max)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func openRegularAssessmentFile(root, relative string) (*os.File, error) {
	clean, err := relativePath("assessment input", relative)
	if err != nil {
		return nil, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for i := range parts {
		partial := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		info, lstatErr := handle.Lstat(partial)
		if lstatErr != nil {
			return nil, lstatErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("assessment input %q contains a symlink component", relative)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("assessment input parent %q is not a directory", partial)
		}
	}
	file, err := openAssessmentFile(handle, clean)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("assessment input %q is not a regular file", relative)
	}
	return file, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func conciseError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
