package evidence

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

// OracleBundle describes the files written for one frozen oracle execution.
type OracleBundle struct {
	RootDir        string
	OraclePath     string
	ManifestPath   string
	LifecyclePath  string
	AccessPath     string
	ExecutablePath string
	ChecksumsPath  string
}

// OracleExecution records the enforcement context around the child process.
// The path lists are resolved capability declarations; AccessEvents are the
// separate host observations written to events/access.jsonl.
type OracleExecution struct {
	ExecutionID              string                        `json:"execution_id"`
	Mode                     string                        `json:"mode"`
	Command                  []string                      `json:"command"`
	WorkingDir               string                        `json:"working_dir"`
	Backend                  string                        `json:"backend"`
	Enforcement              string                        `json:"enforcement"`
	PolicySHA256             string                        `json:"policy_sha256"`
	SubjectID                string                        `json:"subject_id"`
	ExecutablePath           string                        `json:"executable_path"`
	ExecutableSHA256         string                        `json:"executable_sha256"`
	ObservedExecutablePath   string                        `json:"observed_executable_path"`
	ObservedExecutableSHA256 string                        `json:"observed_executable_sha256"`
	ExecutableObservedAt     time.Time                     `json:"executable_observed_at"`
	CanInvokeSubject         bool                          `json:"can_invoke_subject"`
	AllowedTools             []string                      `json:"allowed_tools,omitempty"`
	AllowedReads             []string                      `json:"allowed_reads"`
	AllowedWrites            []string                      `json:"allowed_writes"`
	DeniedPaths              []string                      `json:"denied_paths"`
	StartedAt                time.Time                     `json:"started_at"`
	CompletedAt              time.Time                     `json:"completed_at"`
	Outcome                  string                        `json:"outcome"`
	Access                   AccessTelemetry               `json:"access_telemetry"`
	Events                   []OracleExecutionEvent        `json:"-"`
	AccessEvents             []OracleAccessEvent           `json:"-"`
	ExecutableObservations   []OracleExecutableObservation `json:"-"`
}

// AccessTelemetry describes the quality of the host access observation. A
// captured stream may still contain zero events; that means only that no
// matching event was observed during the collection window.
type AccessTelemetry struct {
	Status                       string    `json:"status"`
	Source                       string    `json:"source"`
	ProcessID                    int       `json:"process_id,omitempty"`
	ProcessIDs                   []int     `json:"process_ids,omitempty"`
	EventCount                   int       `json:"event_count"`
	ParseErrors                  int       `json:"parse_errors"`
	ProcessTreeErrors            int       `json:"process_tree_errors"`
	ExecutableSampleCount        int       `json:"executable_sample_count"`
	ExecutableSamplingIntervalMS int       `json:"executable_sampling_interval_ms"`
	ExecutableSamplingStartedAt  time.Time `json:"executable_sampling_started_at,omitempty"`
	ExecutableSamplingStoppedAt  time.Time `json:"executable_sampling_stopped_at,omitempty"`
	ExecutableObservationCount   int       `json:"executable_observation_count"`
	ExecutableObservationErrors  int       `json:"executable_observation_errors"`
	Reason                       string    `json:"reason,omitempty"`
}

// OracleAccessEvent is the append-only JSONL representation of a normalized
// host sandbox decision.
type OracleAccessEvent struct {
	EventID     string    `json:"event_id"`
	ExecutionID string    `json:"execution_id"`
	Sequence    int       `json:"sequence"`
	Timestamp   time.Time `json:"timestamp"`
	Process     string    `json:"process"`
	PID         int       `json:"pid"`
	Decision    string    `json:"decision"`
	Operation   string    `json:"operation"`
	Resource    string    `json:"resource"`
}

// OracleExecutableObservation is the append-only JSONL representation of a
// host process identity observation.
type OracleExecutableObservation struct {
	EventID     string    `json:"event_id"`
	ExecutionID string    `json:"execution_id"`
	Sequence    int       `json:"sequence"`
	Timestamp   time.Time `json:"timestamp"`
	PID         int       `json:"pid"`
	Path        string    `json:"path"`
	SHA256      string    `json:"sha256"`
}

// OracleExecutionEvent is the append-only JSONL representation of an oracle
// process event.
type OracleExecutionEvent struct {
	EventID     string         `json:"event_id"`
	ExecutionID string         `json:"execution_id"`
	Sequence    int            `json:"sequence"`
	Timestamp   time.Time      `json:"timestamp"`
	Actor       string         `json:"actor"`
	Kind        string         `json:"kind"`
	Payload     map[string]any `json:"payload"`
}

// OracleReference identifies the exact bytes of a frozen oracle.
type OracleReference struct {
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

// OracleManifest is the evidence entrypoint for a frozen oracle. It is
// separate from a subject run because it has no subject observations.
type OracleManifest struct {
	Schema          string                   `json:"schema"`
	Assurance       runner.Assurance         `json:"assurance"`
	Oracle          OracleReference          `json:"oracle"`
	Contract        oracle.ContractReference `json:"contract"`
	Policy          policy.Reference         `json:"policy"`
	Execution       OracleExecution          `json:"execution"`
	ArtifactsSHA256 map[string]string        `json:"artifacts_sha256"`
}

// WriteOracleBundle verifies the child-written canonical oracle and writes
// its manifest, execution events, policy copy, and checksums.
func WriteOracleBundle(outputDir string, artifact oracle.Artifact, execution OracleExecution, sealedPolicy policy.Sealed) (OracleBundle, error) {
	if strings.TrimSpace(outputDir) == "" {
		return OracleBundle{}, fmt.Errorf("oracle evidence output directory must not be empty")
	}
	if execution.ExecutionID == "" || execution.Mode != "sandboxed-process" {
		return OracleBundle{}, fmt.Errorf("oracle execution must be a sandboxed process with an execution ID")
	}
	if execution.PolicySHA256 != sealedPolicy.SHA256 || artifact.PolicySHA256 != sealedPolicy.SHA256 {
		return OracleBundle{}, fmt.Errorf("oracle execution, artifact, and policy hashes do not match")
	}
	policySubjectID, err := policy.SubjectID(sealedPolicy.Document)
	if err != nil {
		return OracleBundle{}, err
	}
	if execution.SubjectID != policySubjectID {
		return OracleBundle{}, fmt.Errorf("oracle execution subject ID %q does not match policy subject ID %q", execution.SubjectID, policySubjectID)
	}
	if execution.ExecutablePath == "" || execution.ExecutableSHA256 == "" {
		return OracleBundle{}, fmt.Errorf("oracle execution executable identity is required")
	}
	if len(execution.ExecutableSHA256) != 64 {
		return OracleBundle{}, fmt.Errorf("oracle execution executable SHA-256 must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(execution.ExecutableSHA256); err != nil {
		return OracleBundle{}, fmt.Errorf("oracle execution executable SHA-256 is invalid: %w", err)
	}
	if execution.ObservedExecutablePath == "" || execution.ObservedExecutableSHA256 == "" || execution.ExecutableObservedAt.IsZero() {
		return OracleBundle{}, fmt.Errorf("oracle execution observed executable identity is required")
	}
	if execution.ObservedExecutablePath != execution.ExecutablePath || execution.ObservedExecutableSHA256 != execution.ExecutableSHA256 {
		return OracleBundle{}, fmt.Errorf("oracle execution observed executable identity does not match prepared identity")
	}
	if problems := oracle.Validate(artifact); len(problems) > 0 {
		return OracleBundle{}, fmt.Errorf("invalid oracle: %s", strings.Join(problems, "; "))
	}
	if execution.Access.Status == "" {
		execution.Access = AccessTelemetry{
			Status: "unavailable",
			Source: "unknown",
			Reason: "access telemetry was not supplied",
		}
	}
	if execution.Access.EventCount != len(execution.AccessEvents) {
		return OracleBundle{}, fmt.Errorf("oracle access event count is %d, but %d events were supplied", execution.Access.EventCount, len(execution.AccessEvents))
	}
	if execution.Access.ExecutableObservationCount != len(execution.ExecutableObservations) {
		return OracleBundle{}, fmt.Errorf("oracle executable observation count is %d, but %d observations were supplied", execution.Access.ExecutableObservationCount, len(execution.ExecutableObservations))
	}
	if err := validateExecutableSampling(
		execution.Access.ExecutableSampleCount,
		execution.Access.ExecutableSamplingIntervalMS,
		execution.Access.ExecutableSamplingStartedAt,
		execution.Access.ExecutableSamplingStoppedAt,
		execution.Access.ExecutableObservationCount,
		execution.Access.ExecutableObservationErrors,
	); err != nil {
		return OracleBundle{}, fmt.Errorf("invalid executable sampling metadata: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return OracleBundle{}, err
	}

	oraclePath := filepath.Join(outputDir, "oracle.json")
	contents, err := os.ReadFile(oraclePath)
	if err != nil {
		return OracleBundle{}, fmt.Errorf("read child-written oracle: %w", err)
	}
	loaded, err := oracle.LoadFile(oraclePath)
	if err != nil {
		return OracleBundle{}, err
	}
	canonical, err := oracle.CanonicalJSON(loaded)
	if err != nil {
		return OracleBundle{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return OracleBundle{}, fmt.Errorf("oracle artifact is not canonical JSON")
	}
	if loaded.Contract != artifact.Contract || loaded.PolicySHA256 != artifact.PolicySHA256 {
		return OracleBundle{}, fmt.Errorf("child-written oracle identity differs from the expected artifact")
	}
	oracleHash := hashBytes(contents)

	eventsDir := filepath.Join(outputDir, "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return OracleBundle{}, err
	}
	lifecyclePath := filepath.Join(eventsDir, "oracle.jsonl")
	if err := writeOracleEvents(lifecyclePath, execution); err != nil {
		return OracleBundle{}, err
	}
	accessPath := filepath.Join(eventsDir, "access.jsonl")
	if err := writeOracleAccessEvents(accessPath, execution); err != nil {
		return OracleBundle{}, err
	}
	executablePath := filepath.Join(eventsDir, "executables.jsonl")
	if err := writeOracleExecutableObservations(executablePath, execution); err != nil {
		return OracleBundle{}, err
	}

	policyDir := filepath.Join(outputDir, "policy")
	if err := os.MkdirAll(policyDir, 0o755); err != nil {
		return OracleBundle{}, err
	}
	policyCanonicalPath := filepath.Join(policyDir, "canonical.json")
	if err := os.WriteFile(policyCanonicalPath, sealedPolicy.CanonicalJSON, 0o644); err != nil {
		return OracleBundle{}, err
	}
	policyHashPath := filepath.Join(policyDir, "hash.txt")
	if err := os.WriteFile(policyHashPath, []byte(sealedPolicy.SHA256+"\n"), 0o644); err != nil {
		return OracleBundle{}, err
	}

	oracleReference := OracleReference{Schema: artifact.Schema, SHA256: oracleHash}
	artifacts := map[string]string{}
	for _, relative := range []string{"oracle.json", "events/oracle.jsonl", "events/access.jsonl", "events/executables.jsonl", "policy/canonical.json", "policy/hash.txt"} {
		path := filepath.Join(outputDir, relative)
		artifactHash, err := hashFile(path)
		if err != nil {
			return OracleBundle{}, fmt.Errorf("hash %s: %w", relative, err)
		}
		artifacts[relative] = artifactHash
	}
	manifest := OracleManifest{
		Schema:          "sorna.oracle-evidence/v1",
		Assurance:       oracleAssurance(execution),
		Oracle:          oracleReference,
		Contract:        artifact.Contract,
		Policy:          sealedPolicy.Reference(),
		Execution:       execution,
		ArtifactsSHA256: artifacts,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return OracleBundle{}, fmt.Errorf("encode oracle evidence manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return OracleBundle{}, err
	}
	artifacts["manifest.json"] = hashBytes(manifestBytes)

	checksumsPath := filepath.Join(outputDir, "checksums.sha256")
	if err := writeChecksums(checksumsPath, artifacts); err != nil {
		return OracleBundle{}, err
	}
	return OracleBundle{
		RootDir:        outputDir,
		OraclePath:     oraclePath,
		ManifestPath:   manifestPath,
		LifecyclePath:  lifecyclePath,
		AccessPath:     accessPath,
		ExecutablePath: executablePath,
		ChecksumsPath:  checksumsPath,
	}, nil
}

func writeOracleAccessEvents(path string, execution OracleExecution) error {
	var buffer bytes.Buffer
	for index, event := range execution.AccessEvents {
		if event.EventID == "" {
			event.EventID = fmt.Sprintf("access-%04d", index+1)
		}
		if event.ExecutionID == "" {
			event.ExecutionID = execution.ExecutionID
		}
		if event.Sequence == 0 {
			event.Sequence = index + 1
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode oracle access event %d: %w", event.Sequence, err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func writeOracleExecutableObservations(path string, execution OracleExecution) error {
	var buffer bytes.Buffer
	for index, observation := range execution.ExecutableObservations {
		if observation.EventID == "" {
			observation.EventID = fmt.Sprintf("executable-%04d", index+1)
		}
		if observation.ExecutionID == "" {
			observation.ExecutionID = execution.ExecutionID
		}
		if observation.Sequence == 0 {
			observation.Sequence = index + 1
		}
		encoded, err := json.Marshal(observation)
		if err != nil {
			return fmt.Errorf("encode oracle executable observation %d: %w", observation.Sequence, err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func oracleAssurance(execution OracleExecution) runner.Assurance {
	limitations := []string{
		"access events are host log observations and do not prove the absence of unobserved actions",
		"this assurance describes oracle generation only; subject-run isolation remains separate",
	}
	if execution.Access.Status != "captured" {
		limitations = append([]string{"host access telemetry was unavailable"}, limitations...)
		return runner.Assurance{
			Level:               0,
			Status:              "telemetry-unavailable",
			ObservationCoverage: "unavailable",
			Limitations:         limitations,
		}
	}
	if execution.Access.ParseErrors > 0 || execution.Access.ProcessTreeErrors > 0 || execution.Access.ExecutableObservationErrors > 0 || execution.Access.ExecutableObservationCount == 0 {
		gaps := make([]string, 0, 4)
		if execution.Access.ParseErrors > 0 {
			gaps = append(gaps, fmt.Sprintf("%d host access records could not be normalized", execution.Access.ParseErrors))
		}
		if execution.Access.ProcessTreeErrors > 0 {
			gaps = append(gaps, fmt.Sprintf("%d process-tree samples failed", execution.Access.ProcessTreeErrors))
		}
		if execution.Access.ExecutableObservationErrors > 0 {
			gaps = append(gaps, fmt.Sprintf("%d executable identity observations failed", execution.Access.ExecutableObservationErrors))
		}
		if execution.Access.ExecutableObservationCount == 0 {
			gaps = append(gaps, "no executable identity samples were captured")
		}
		limitations = append(gaps, executableObservationLimitation(execution.Access))
		limitations = append(limitations, "access events are host log observations and do not prove the absence of unobserved actions", "this assurance describes oracle generation only; subject-run isolation remains separate")
		return runner.Assurance{
			Level:               0,
			Status:              "host-enforced-observed-with-gaps",
			ObservationCoverage: executableObservationCoverage(execution.Access),
			Limitations:         limitations,
		}
	}
	limitations = append([]string{executableObservationLimitation(execution.Access)}, limitations...)
	return runner.Assurance{
		Level:               0,
		Status:              "host-enforced-observed",
		ObservationCoverage: executableObservationCoverage(execution.Access),
		Limitations:         limitations,
	}
}

func executableObservationCoverage(access AccessTelemetry) string {
	if access.Status != "captured" {
		return "unavailable"
	}
	if access.ExecutableSampleCount == 0 || access.ExecutableObservationCount == 0 || access.ProcessTreeErrors > 0 || access.ExecutableObservationErrors > 0 {
		return "periodic-best-effort-with-gaps"
	}
	return "periodic-best-effort"
}

func executableObservationLimitation(access AccessTelemetry) string {
	if access.ExecutableSamplingIntervalMS > 0 {
		return fmt.Sprintf("executable identity was sampled every %d ms; transitions between samples may be unobserved", access.ExecutableSamplingIntervalMS)
	}
	return "executable identity coverage is periodic; transitions between samples may be unobserved"
}

func writeOracleEvents(path string, execution OracleExecution) error {
	var buffer bytes.Buffer
	for _, event := range execution.Events {
		if event.ExecutionID == "" {
			event.ExecutionID = execution.ExecutionID
		}
		if event.Actor == "" {
			event.Actor = "sorna"
		}
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode oracle event %d: %w", event.Sequence, err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}
