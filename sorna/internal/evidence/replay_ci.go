package evidence

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

const replayExplanationSchema = "sorna.replay-explanation/v1"

// ReplayCIExplanation is the compact coordinator-facing interpretation of a
// replay report. The complete replay report remains in ciresult.Artifact.Report.
type ReplayCIExplanation struct {
	Schema             string   `json:"schema"`
	Status             string   `json:"status"`
	ReplayStatus       string   `json:"replay_status"`
	IntegrityStatus    string   `json:"integrity_status"`
	BehaviorStatus     string   `json:"behavior_status"`
	ObservationStatus  string   `json:"observation_status"`
	ObservationChanges int      `json:"observation_changes"`
	RequestStatus      string   `json:"request_status"`
	RequestChanges     int      `json:"request_changes"`
	Differences        []string `json:"differences,omitempty"`
}

// BuildReplayCIResult runs the frozen-oracle replay and adapts its producer
// report to the shared CI envelope. The supplied source root only resolves
// relative paths; it is not an additional contract or execution input.
func BuildReplayCIResult(evidenceDir, oraclePath, baseURL, sourceRoot string) (ciresult.Artifact, error) {
	return buildReplayCIResult(evidenceDir, oraclePath, sourceRoot, runner.Config{BaseURL: baseURL})
}

func buildReplayCIResult(evidenceDir, oraclePath, sourceRoot string, config runner.Config) (ciresult.Artifact, error) {
	if strings.TrimSpace(evidenceDir) == "" || strings.TrimSpace(oraclePath) == "" || strings.TrimSpace(config.BaseURL) == "" {
		return ciresult.Artifact{}, fmt.Errorf("replay evidence directory, oracle path, and base URL must not be empty")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	evidenceResolved := resolveReplayPath(evidenceDir, sourceRoot)
	oracleResolved := resolveReplayPath(oraclePath, sourceRoot)
	artifact, err := oracle.LoadFile(oracleResolved)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("load replay oracle: %w", err)
	}
	result, err := Replay(context.Background(), evidenceResolved, artifact, config)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	if err := ValidateReplayResult(result); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate replay result for CI: %w", err)
	}
	reportBytes, err := json.Marshal(result)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode replay report: %w", err)
	}
	ciStatus := replayCIStatus(result.Status)
	explanationBytes, err := json.Marshal(ReplayCIExplanation{
		Schema:             replayExplanationSchema,
		Status:             ciStatus,
		ReplayStatus:       result.Status,
		IntegrityStatus:    result.Integrity.Status,
		BehaviorStatus:     result.Behavior.Status,
		ObservationStatus:  result.Behavior.ObservationStatus,
		ObservationChanges: result.ObservationChanges,
		RequestStatus:      result.Behavior.RequestStatus,
		RequestChanges:     result.RequestChanges,
		Differences:        result.Differences,
	})
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode replay explanation: %w", err)
	}
	inputs, err := replayInputRefs(evidenceResolved, evidenceDir, oracleResolved, oraclePath)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	exitCode, err := ciresult.ExitCodeForStatus(ciStatus)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("map replay status: %w", err)
	}
	artifactResult := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "behavioral-replay",
		Status:    ciStatus,
		ExitCode:  exitCode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source: ciresult.Source{
			Root:       sourceRoot,
			ModulePath: result.Contract.ID,
		},
		Inputs:      inputs,
		Report:      reportBytes,
		Explanation: explanationBytes,
	}
	if ciStatus == "error" {
		artifactResult.Error = replayCIError(result)
	}
	if err := artifactResult.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate replay CI result: %w", err)
	}
	return artifactResult, nil
}

func replayCIError(result ReplayResult) string {
	if len(result.Differences) > 0 {
		return result.Differences[0]
	}
	return "replay could not be evaluated completely"
}

// BuildReplayCIErrorResult produces a collector-friendly error envelope when
// replay inputs cannot be loaded or verified before a replay report exists.
func BuildReplayCIErrorResult(evidenceDir, oraclePath, sourceRoot string, cause error) (ciresult.Artifact, error) {
	if cause == nil {
		return ciresult.Artifact{}, fmt.Errorf("replay CI error result requires an error")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	inputs := make(map[string]ciresult.FileRef)
	if strings.TrimSpace(evidenceDir) != "" {
		resolvedEvidence := resolveReplayPath(evidenceDir, sourceRoot)
		addReplayInputIfPresent(inputs, "evidence_manifest", filepath.Join(evidenceDir, "manifest.json"), filepath.Join(resolvedEvidence, "manifest.json"))
		addReplayInputIfPresent(inputs, "evidence_checksums", filepath.Join(evidenceDir, "checksums.sha256"), filepath.Join(resolvedEvidence, "checksums.sha256"))
	}
	if strings.TrimSpace(oraclePath) != "" {
		addReplayInputIfPresent(inputs, "oracle", oraclePath, resolveReplayPath(oraclePath, sourceRoot))
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "behavioral-replay",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Inputs:    inputs,
		Error:     cause.Error(),
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate replay CI error result: %w", err)
	}
	return artifact, nil
}

func replayCIStatus(status string) string {
	switch status {
	case "matched":
		return "passed"
	case "drifted":
		return "failed"
	default:
		return "error"
	}
}

func replayInputRefs(evidenceResolved, evidenceInput, oracleResolved, oracleInput string) (map[string]ciresult.FileRef, error) {
	inputs := map[string]ciresult.FileRef{
		"oracle": {
			Path:   oracleInput,
			SHA256: "",
		},
	}
	oracleHash, err := hashFile(oracleResolved)
	if err != nil {
		return nil, fmt.Errorf("hash replay oracle for CI result: %w", err)
	}
	inputs["oracle"] = ciresult.FileRef{Path: oracleInput, SHA256: oracleHash}
	manifestInput := filepath.Join(evidenceInput, "manifest.json")
	checksumsInput := filepath.Join(evidenceInput, "checksums.sha256")
	manifestResolved := filepath.Join(evidenceResolved, "manifest.json")
	checksumsResolved := filepath.Join(evidenceResolved, "checksums.sha256")
	for name, inputPath := range map[string]string{
		"evidence_manifest":  manifestInput,
		"evidence_checksums": checksumsInput,
	} {
		resolved := manifestResolved
		if name == "evidence_checksums" {
			resolved = checksumsResolved
		}
		hash, err := hashFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("hash %s for CI result: %w", inputPath, err)
		}
		inputs[name] = ciresult.FileRef{Path: inputPath, SHA256: hash}
	}
	paths, err := replayChecksumPaths(checksumsResolved)
	if err != nil {
		return nil, err
	}
	for _, relative := range paths {
		resolved := filepath.Join(evidenceResolved, relative)
		hash, err := hashFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("hash evidence artifact %s for CI result: %w", relative, err)
		}
		inputs["evidence:"+relative] = ciresult.FileRef{Path: filepath.Join(evidenceInput, relative), SHA256: hash}
	}
	return inputs, nil
}

func replayChecksumPaths(path string) ([]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read replay checksums: %w", err)
	}
	seen := make(map[string]bool)
	paths := make([]string, 0)
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 || parts[1] == "" {
			return nil, fmt.Errorf("invalid replay checksum line %d", lineNumber+1)
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			return nil, fmt.Errorf("invalid replay checksum line %d: %w", lineNumber+1, err)
		}
		relative, err := safeRelativePath(parts[1])
		if err != nil {
			return nil, fmt.Errorf("replay checksum line %d: %w", lineNumber+1, err)
		}
		if seen[relative] {
			return nil, fmt.Errorf("duplicate replay checksum path %q", relative)
		}
		seen[relative] = true
		paths = append(paths, relative)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("replay checksum file contains no artifacts")
	}
	return paths, nil
}

func addReplayInputIfPresent(inputs map[string]ciresult.FileRef, name, inputPath, resolvedPath string) {
	if hash, err := hashFile(resolvedPath); err == nil {
		inputs[name] = ciresult.FileRef{Path: inputPath, SHA256: hash}
	}
}

func resolveReplayPath(path, sourceRoot string) string {
	if filepath.IsAbs(path) || sourceRoot == "." {
		return path
	}
	return filepath.Join(sourceRoot, path)
}
