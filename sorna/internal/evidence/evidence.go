// Package evidence writes and verifies the first Sorna evidence bundle.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

// Bundle describes the files written for one run.
type Bundle struct {
	RootDir               string
	RunPath               string
	ManifestPath          string
	LifecyclePath         string
	SubjectAccessPath     string
	SubjectExecutablePath string
	ChecksumsPath         string
}

// Manifest is the bundle entrypoint. It records identity and hashes, but does
// not turn content integrity into a correctness or isolation claim.
type Manifest struct {
	Schema          string                   `json:"schema"`
	RunID           string                   `json:"run_id"`
	CreatedAt       time.Time                `json:"created_at"`
	Assurance       runner.Assurance         `json:"assurance"`
	Contract        runner.ContractReference `json:"contract"`
	Oracle          *runner.OracleReference  `json:"oracle,omitempty"`
	Subject         runner.SubjectReference  `json:"subject"`
	Policy          *policy.Reference        `json:"policy,omitempty"`
	SubjectPolicy   *policy.Reference        `json:"subject_policy,omitempty"`
	ArtifactsSHA256 map[string]string        `json:"artifacts_sha256"`
}

// LifecycleEvent is the append-only JSONL form of a lifecycle record event.
// The event envelope gives each line run identity and an actor without making
// the lower-level lifecycle package depend on evidence storage.
type LifecycleEvent struct {
	EventID   string         `json:"event_id"`
	RunID     string         `json:"run_id"`
	Sequence  int            `json:"sequence"`
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload"`
}

// WriteBundle writes run.json, an append-only lifecycle JSONL stream, a
// manifest, and checksums for every material file in the bundle.
func WriteBundle(outputDir string, record runner.RunRecord, sealedPolicy *policy.Sealed) (Bundle, error) {
	return WriteBundleWithPolicies(outputDir, record, sealedPolicy, nil)
}

// WriteBundleWithPolicies keeps the oracle-generation policy and the managed
// subject policy as separate evidence inputs. The old WriteBundle API remains
// a compatibility path for runs that only have one policy.
func WriteBundleWithPolicies(outputDir string, record runner.RunRecord, sealedPolicy, sealedSubjectPolicy *policy.Sealed) (Bundle, error) {
	if strings.TrimSpace(outputDir) == "" {
		return Bundle{}, fmt.Errorf("evidence output directory must not be empty")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Bundle{}, err
	}
	runPath, err := runner.Write(outputDir, record)
	if err != nil {
		return Bundle{}, err
	}
	eventsDir := filepath.Join(outputDir, "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return Bundle{}, err
	}
	lifecyclePath := filepath.Join(eventsDir, "lifecycle.jsonl")
	if err := writeLifecycleEvents(lifecyclePath, record); err != nil {
		return Bundle{}, err
	}

	runHash, err := hashFile(runPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("hash run.json: %w", err)
	}
	lifecycleHash, err := hashFile(lifecyclePath)
	if err != nil {
		return Bundle{}, fmt.Errorf("hash events/lifecycle.jsonl: %w", err)
	}
	artifacts := map[string]string{
		"run.json":               runHash,
		"events/lifecycle.jsonl": lifecycleHash,
	}
	var subjectAccessPath string
	var subjectExecutablePath string
	if record.Lifecycle != nil && record.Lifecycle.Access != nil {
		subjectAccessPath = filepath.Join(eventsDir, "subject-access.jsonl")
		if err := writeSubjectAccessEvents(subjectAccessPath, record); err != nil {
			return Bundle{}, err
		}
		subjectAccessHash, err := hashFile(subjectAccessPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash events/subject-access.jsonl: %w", err)
		}
		artifacts["events/subject-access.jsonl"] = subjectAccessHash
		subjectExecutablePath = filepath.Join(eventsDir, "subject-executables.jsonl")
		if err := writeSubjectExecutableObservations(subjectExecutablePath, record); err != nil {
			return Bundle{}, err
		}
		subjectExecutableHash, err := hashFile(subjectExecutablePath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash events/subject-executables.jsonl: %w", err)
		}
		artifacts["events/subject-executables.jsonl"] = subjectExecutableHash
	}
	var policyReference *policy.Reference
	if sealedPolicy != nil {
		policyDir := filepath.Join(outputDir, "policy")
		if err := os.MkdirAll(policyDir, 0o755); err != nil {
			return Bundle{}, err
		}
		policyCanonicalPath := filepath.Join(policyDir, "canonical.json")
		if err := os.WriteFile(policyCanonicalPath, sealedPolicy.CanonicalJSON, 0o644); err != nil {
			return Bundle{}, err
		}
		policyHashPath := filepath.Join(policyDir, "hash.txt")
		if err := os.WriteFile(policyHashPath, []byte(sealedPolicy.SHA256+"\n"), 0o644); err != nil {
			return Bundle{}, err
		}
		policyCanonicalHash, err := hashFile(policyCanonicalPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/canonical.json: %w", err)
		}
		policyHashHash, err := hashFile(policyHashPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/hash.txt: %w", err)
		}
		artifacts["policy/canonical.json"] = policyCanonicalHash
		artifacts["policy/hash.txt"] = policyHashHash
		reference := sealedPolicy.Reference()
		policyReference = &reference
	}
	var subjectPolicyReference *policy.Reference
	if sealedSubjectPolicy != nil {
		policyDir := filepath.Join(outputDir, "policy", "subject")
		if err := os.MkdirAll(policyDir, 0o755); err != nil {
			return Bundle{}, err
		}
		policyCanonicalPath := filepath.Join(policyDir, "canonical.json")
		if err := os.WriteFile(policyCanonicalPath, sealedSubjectPolicy.CanonicalJSON, 0o644); err != nil {
			return Bundle{}, err
		}
		policyHashPath := filepath.Join(policyDir, "hash.txt")
		if err := os.WriteFile(policyHashPath, []byte(sealedSubjectPolicy.SHA256+"\n"), 0o644); err != nil {
			return Bundle{}, err
		}
		policyCanonicalHash, err := hashFile(policyCanonicalPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/subject/canonical.json: %w", err)
		}
		policyHashHash, err := hashFile(policyHashPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/subject/hash.txt: %w", err)
		}
		artifacts["policy/subject/canonical.json"] = policyCanonicalHash
		artifacts["policy/subject/hash.txt"] = policyHashHash
		reference := sealedSubjectPolicy.Reference()
		subjectPolicyReference = &reference
	}
	manifest := Manifest{
		Schema:          "sorna.evidence/v1",
		RunID:           record.RunID,
		CreatedAt:       record.CreatedAt,
		Assurance:       record.Assurance,
		Contract:        record.Contract,
		Oracle:          record.Oracle,
		Subject:         record.Subject,
		Policy:          policyReference,
		SubjectPolicy:   subjectPolicyReference,
		ArtifactsSHA256: artifacts,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Bundle{}, fmt.Errorf("encode evidence manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return Bundle{}, err
	}
	artifacts["manifest.json"] = hashBytes(manifestBytes)

	checksumsPath := filepath.Join(outputDir, "checksums.sha256")
	if err := writeChecksums(checksumsPath, artifacts); err != nil {
		return Bundle{}, err
	}
	return Bundle{
		RootDir:               outputDir,
		RunPath:               runPath,
		ManifestPath:          manifestPath,
		LifecyclePath:         lifecyclePath,
		SubjectAccessPath:     subjectAccessPath,
		SubjectExecutablePath: subjectExecutablePath,
		ChecksumsPath:         checksumsPath,
	}, nil
}

// Verify checks every path listed in checksums.sha256 and reports the first
// missing, malformed, duplicated, or mismatched artifact.
func Verify(outputDir string) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("evidence output directory must not be empty")
	}
	contents, err := os.ReadFile(filepath.Join(outputDir, "checksums.sha256"))
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	for lineNumber, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 || parts[1] == "" {
			return fmt.Errorf("invalid checksum line %d", lineNumber+1)
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			return fmt.Errorf("invalid checksum line %d: %w", lineNumber+1, err)
		}
		relative, err := safeRelativePath(parts[1])
		if err != nil {
			return fmt.Errorf("checksum line %d: %w", lineNumber+1, err)
		}
		if seen[relative] {
			return fmt.Errorf("duplicate checksum path %q", relative)
		}
		seen[relative] = true
		actual, err := hashFile(filepath.Join(outputDir, relative))
		if err != nil {
			return fmt.Errorf("hash %s: %w", relative, err)
		}
		if actual != parts[0] {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", relative, parts[0], actual)
		}
	}
	if len(seen) == 0 {
		return fmt.Errorf("checksum file contains no artifacts")
	}
	return nil
}

func writeLifecycleEvents(path string, record runner.RunRecord) error {
	var buffer bytes.Buffer
	if record.Lifecycle != nil {
		for _, event := range record.Lifecycle.Events {
			line := LifecycleEvent{
				EventID:   fmt.Sprintf("evt-%04d", event.Sequence),
				RunID:     record.RunID,
				Sequence:  event.Sequence,
				Timestamp: event.Timestamp,
				Actor:     "sorna",
				Kind:      event.Kind,
				Payload:   map[string]any{},
			}
			if event.Detail != "" {
				line.Payload["detail"] = event.Detail
			}
			encoded, err := json.Marshal(line)
			if err != nil {
				return fmt.Errorf("encode lifecycle event %d: %w", event.Sequence, err)
			}
			buffer.Write(encoded)
			buffer.WriteByte('\n')
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

type SubjectAccessEvent struct {
	EventID   string    `json:"event_id"`
	RunID     string    `json:"run_id"`
	Sequence  int       `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Process   string    `json:"process"`
	PID       int       `json:"pid"`
	Decision  string    `json:"decision"`
	Operation string    `json:"operation"`
	Resource  string    `json:"resource"`
}

func writeSubjectAccessEvents(path string, record runner.RunRecord) error {
	var buffer bytes.Buffer
	if record.Lifecycle != nil {
		for index, event := range record.Lifecycle.AccessEvents {
			line := SubjectAccessEvent{
				EventID:   fmt.Sprintf("access-%04d", index+1),
				RunID:     record.RunID,
				Sequence:  index + 1,
				Timestamp: event.Timestamp,
				Process:   event.Process,
				PID:       event.PID,
				Decision:  event.Decision,
				Operation: event.Operation,
				Resource:  event.Resource,
			}
			encoded, err := json.Marshal(line)
			if err != nil {
				return fmt.Errorf("encode subject access event %d: %w", index+1, err)
			}
			buffer.Write(encoded)
			buffer.WriteByte('\n')
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

type SubjectExecutableObservation struct {
	EventID   string    `json:"event_id"`
	RunID     string    `json:"run_id"`
	Sequence  int       `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	PID       int       `json:"pid"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
}

func writeSubjectExecutableObservations(path string, record runner.RunRecord) error {
	var buffer bytes.Buffer
	if record.Lifecycle != nil {
		for index, observation := range record.Lifecycle.ExecutableObservations {
			line := SubjectExecutableObservation{
				EventID:   fmt.Sprintf("executable-%04d", index+1),
				RunID:     record.RunID,
				Sequence:  index + 1,
				Timestamp: observation.Timestamp,
				PID:       observation.PID,
				Path:      observation.Path,
				SHA256:    observation.SHA256,
			}
			encoded, err := json.Marshal(line)
			if err != nil {
				return fmt.Errorf("encode subject executable observation %d: %w", index+1, err)
			}
			buffer.Write(encoded)
			buffer.WriteByte('\n')
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func writeChecksums(path string, artifacts map[string]string) error {
	paths := make([]string, 0, len(artifacts))
	for relative := range artifacts {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	var buffer strings.Builder
	for _, relative := range paths {
		if _, err := safeRelativePath(relative); err != nil {
			return err
		}
		fmt.Fprintf(&buffer, "%s  %s\n", artifacts[relative], relative)
	}
	return os.WriteFile(path, []byte(buffer.String()), 0o644)
}

func hashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(contents), nil
}

func hashBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func safeRelativePath(raw string) (string, error) {
	clean := filepath.Clean(raw)
	if clean == "." || filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("checksum path must stay inside the evidence bundle: %q", raw)
	}
	return clean, nil
}
