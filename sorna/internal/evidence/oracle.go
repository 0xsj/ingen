package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/policy"
)

// OracleBundle describes the files written for one frozen oracle execution.
type OracleBundle struct {
	RootDir       string
	OraclePath    string
	ManifestPath  string
	LifecyclePath string
	ChecksumsPath string
}

// OracleExecution records the enforcement context around the child process.
// The path lists are resolved capability declarations, not kernel audit logs.
type OracleExecution struct {
	ExecutionID   string                 `json:"execution_id"`
	Mode          string                 `json:"mode"`
	Command       []string               `json:"command"`
	WorkingDir    string                 `json:"working_dir"`
	Backend       string                 `json:"backend"`
	Enforcement   string                 `json:"enforcement"`
	PolicySHA256  string                 `json:"policy_sha256"`
	AllowedReads  []string               `json:"allowed_reads"`
	AllowedWrites []string               `json:"allowed_writes"`
	DeniedPaths   []string               `json:"denied_paths"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   time.Time              `json:"completed_at"`
	Outcome       string                 `json:"outcome"`
	Events        []OracleExecutionEvent `json:"-"`
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
	if problems := oracle.Validate(artifact); len(problems) > 0 {
		return OracleBundle{}, fmt.Errorf("invalid oracle: %s", strings.Join(problems, "; "))
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
	for _, relative := range []string{"oracle.json", "events/oracle.jsonl", "policy/canonical.json", "policy/hash.txt"} {
		path := filepath.Join(outputDir, relative)
		artifactHash, err := hashFile(path)
		if err != nil {
			return OracleBundle{}, fmt.Errorf("hash %s: %w", relative, err)
		}
		artifacts[relative] = artifactHash
	}
	manifest := OracleManifest{
		Schema:          "sorna.oracle-evidence/v1",
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
		RootDir:       outputDir,
		OraclePath:    oraclePath,
		ManifestPath:  manifestPath,
		LifecyclePath: lifecyclePath,
		ChecksumsPath: checksumsPath,
	}, nil
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
