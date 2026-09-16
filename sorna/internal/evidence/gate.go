package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ingen/core/ciresult"
)

// GatePolicy controls which evidence conditions are blocking. An empty
// MinimumObservationCoverage keeps observation quality report-only.
type GatePolicy struct {
	MinimumObservationCoverage string
}

// GateResult is the language-neutral decision produced by the Sorna CI gate.
// Evidence integrity is checked before this result is calculated.
type GateResult struct {
	Schema              string   `json:"schema"`
	EvidenceSchema      string   `json:"evidence_schema"`
	Status              string   `json:"status"`
	ExitCode            int      `json:"exit_code"`
	ObservationPolicy   string   `json:"observation_policy"`
	ObservationCoverage string   `json:"observation_coverage"`
	ContractStatus      string   `json:"contract_status,omitempty"`
	MutationOutcome     string   `json:"mutation_outcome,omitempty"`
	OracleOutcome       string   `json:"oracle_outcome,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
	Reasons             []string `json:"reasons,omitempty"`
}

// EvaluateGate verifies a bundle and applies the explicit CI policy to its
// behavioral result and observation coverage. Coverage is report-only unless a
// minimum is configured.
func EvaluateGate(outputDir string, policy GatePolicy) (GateResult, error) {
	if err := validateMinimumObservationCoverage(policy.MinimumObservationCoverage); err != nil {
		return GateResult{}, err
	}
	if err := Verify(outputDir); err != nil {
		return GateResult{}, fmt.Errorf("verify evidence before gating: %w", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return GateResult{}, fmt.Errorf("read manifest: %w", err)
	}
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(manifestBytes, &envelope); err != nil {
		return GateResult{}, fmt.Errorf("decode manifest: %w", err)
	}

	result := GateResult{
		Schema:              "ingen.gate/v1",
		EvidenceSchema:      envelope.Schema,
		Status:              "passed",
		ObservationPolicy:   "report-only",
		ObservationCoverage: "not-observed",
	}
	var reasons []string
	switch envelope.Schema {
	case "sorna.oracle-evidence/v1":
		var manifest OracleManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return GateResult{}, fmt.Errorf("decode oracle manifest: %w", err)
		}
		result.ObservationCoverage = manifest.Assurance.ObservationCoverage
		result.OracleOutcome = manifest.Execution.Outcome
		if manifest.Execution.Outcome != "completed" {
			reasons = append(reasons, fmt.Sprintf("oracle outcome is %q", manifest.Execution.Outcome))
		}
	case Schema:
		var manifest Manifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return GateResult{}, fmt.Errorf("decode evidence manifest: %w", err)
		}
		result.ObservationCoverage = manifest.Assurance.ObservationCoverage
		runBytes, err := os.ReadFile(filepath.Join(outputDir, "run.json"))
		if err != nil {
			return GateResult{}, fmt.Errorf("read run: %w", err)
		}
		var record struct {
			Verdict struct {
				Status string `json:"status"`
			} `json:"contract_verdict"`
			Mutation *struct {
				Outcome string `json:"outcome"`
			} `json:"mutation,omitempty"`
		}
		if err := json.Unmarshal(runBytes, &record); err != nil {
			return GateResult{}, fmt.Errorf("decode run: %w", err)
		}
		result.ContractStatus = record.Verdict.Status
		if record.Mutation != nil {
			result.MutationOutcome = record.Mutation.Outcome
			if record.Mutation.Outcome != "killed" {
				reasons = append(reasons, fmt.Sprintf("mutation outcome is %q", record.Mutation.Outcome))
			}
		} else if record.Verdict.Status != "pass" {
			reasons = append(reasons, fmt.Sprintf("contract verdict is %q", record.Verdict.Status))
		}
	default:
		return GateResult{}, fmt.Errorf("unsupported evidence schema %q", envelope.Schema)
	}
	if result.ObservationCoverage == "" {
		result.ObservationCoverage = "not-observed"
	}
	if policy.MinimumObservationCoverage != "" {
		result.ObservationPolicy = "minimum:" + policy.MinimumObservationCoverage
		if observationCoverageRank(result.ObservationCoverage) < observationCoverageRank(policy.MinimumObservationCoverage) {
			reasons = append(reasons, fmt.Sprintf("observation coverage is %q, below required minimum %q", result.ObservationCoverage, policy.MinimumObservationCoverage))
		}
	} else if result.ObservationCoverage != "periodic-best-effort" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("observation coverage is %q and remains report-only", result.ObservationCoverage))
	}
	if len(reasons) > 0 {
		result.Status = "failed"
		result.Reasons = reasons
	}
	result.ExitCode, err = ciresult.ExitCodeForStatus(result.Status)
	if err != nil {
		return GateResult{}, fmt.Errorf("map Sorna gate status: %w", err)
	}
	return result, nil
}

// BuildCIResult adapts a verified Sorna gate decision to the shared InGen CI
// envelope. The gate result remains the producer-owned report; the envelope
// only adds the fields a coordinator can consume without knowing Sorna's
// internal result model.
func BuildCIResult(outputDir string, gate GateResult) (ciresult.Artifact, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("read manifest for CI result: %w", err)
	}
	var manifest struct {
		Schema   string `json:"schema"`
		Contract struct {
			ID string `json:"id"`
		} `json:"contract"`
		ArtifactsSHA256 map[string]string `json:"artifacts_sha256"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("decode manifest for CI result: %w", err)
	}
	if manifest.Schema != gate.EvidenceSchema {
		return ciresult.Artifact{}, fmt.Errorf("gate evidence schema %q does not match manifest schema %q", gate.EvidenceSchema, manifest.Schema)
	}
	report, err := json.Marshal(gate)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode Sorna gate report: %w", err)
	}
	explanation, err := json.Marshal(gateExplanation{
		Schema:            "sorna.gate-explanation/v1",
		Status:            gate.Status,
		ObservationPolicy: gate.ObservationPolicy,
		Warnings:          gate.Warnings,
		Reasons:           gate.Reasons,
	})
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode Sorna gate explanation: %w", err)
	}
	manifestHash, err := hashFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("hash manifest for CI result: %w", err)
	}
	inputs := map[string]ciresult.FileRef{
		"manifest": {Path: "manifest.json", SHA256: manifestHash},
	}
	for name, relative := range map[string]string{
		"run":                 "run.json",
		"oracle":              "oracle.json",
		"lifecycle":           "events/lifecycle.jsonl",
		"oracle_events":       "events/oracle.jsonl",
		"access_events":       "events/access.jsonl",
		"subject_access":      "events/subject-access.jsonl",
		"executable_events":   "events/executables.jsonl",
		"subject_executables": "events/subject-executables.jsonl",
	} {
		if hash, ok := manifest.ArtifactsSHA256[relative]; ok {
			inputs[name] = ciresult.FileRef{Path: relative, SHA256: hash}
		}
	}
	var policyRef *ciresult.FileRef
	if hash, ok := manifest.ArtifactsSHA256["policy/canonical.json"]; ok {
		policyRef = &ciresult.FileRef{Path: "policy/canonical.json", SHA256: hash}
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    gate.Status,
		ExitCode:  gate.ExitCode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source: ciresult.Source{
			Root:       outputDir,
			ModulePath: manifest.Contract.ID,
		},
		Policy:      policyRef,
		Inputs:      inputs,
		Report:      report,
		Explanation: explanation,
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate Sorna CI result: %w", err)
	}
	return artifact, nil
}

// BuildCIErrorResult produces the shared error form when the evidence bundle
// cannot be verified or the gate cannot be evaluated.
func BuildCIErrorResult(outputDir string, err error) (ciresult.Artifact, error) {
	if err == nil {
		return ciresult.Artifact{}, fmt.Errorf("Sorna CI error result requires an error")
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: outputDir},
		Error:     err.Error(),
	}
	if validationErr := artifact.Validate(); validationErr != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate Sorna CI error result: %w", validationErr)
	}
	return artifact, nil
}

type gateExplanation struct {
	Schema            string   `json:"schema"`
	Status            string   `json:"status"`
	ObservationPolicy string   `json:"observation_policy"`
	Warnings          []string `json:"warnings,omitempty"`
	Reasons           []string `json:"reasons,omitempty"`
}

func validateMinimumObservationCoverage(value string) error {
	if value == "" || value == "periodic-best-effort" || value == "periodic-best-effort-with-gaps" {
		return nil
	}
	return fmt.Errorf("minimum observation coverage must be periodic-best-effort or periodic-best-effort-with-gaps, got %q", value)
}

func observationCoverageRank(value string) int {
	switch value {
	case "periodic-best-effort":
		return 2
	case "periodic-best-effort-with-gaps":
		return 1
	default:
		return 0
	}
}
