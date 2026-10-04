package agentprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"
)

const (
	MatrixSchema             = "ingen.sentinel-codex-compatibility-matrix/v1"
	MatrixScope              = "local-synthetic-codex-compatibility-matrix"
	maxMatrixCandidates      = 4
	maxMatrixExecutableBytes = 512 << 20
)

type MatrixRequest struct {
	ExecutablePaths []string
	Timeout         time.Duration
}

type MatrixCandidate struct {
	ExecutablePath         string  `json:"executable_path"`
	ExecutableSHA256Before string  `json:"executable_sha256_before"`
	ExecutableSHA256After  string  `json:"executable_sha256_after,omitempty"`
	Status                 string  `json:"status"`
	Diagnostic             *Report `json:"diagnostic,omitempty"`
	ReasonCode             string  `json:"reason_code,omitempty"`
	Reason                 string  `json:"reason,omitempty"`
}

type MatrixReport struct {
	Schema         string            `json:"schema"`
	Scope          string            `json:"scope"`
	Status         string            `json:"status"`
	StartedAt      string            `json:"started_at"`
	FinishedAt     string            `json:"finished_at"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Synthetic      bool              `json:"synthetic"`
	Assurance      string            `json:"assurance"`
	NetworkMode    string            `json:"network_mode"`
	Candidates     []MatrixCandidate `json:"candidates"`
	Limitations    []string          `json:"limitations"`
}

type matrixCandidateInput struct {
	path   string
	sha256 string
}

type diagnoseFunc func(context.Context, Request) (Report, error)

// Compare runs the existing synthetic diagnostic sequentially for each
// explicitly selected candidate. It performs all path checks and before hashes
// before launching any candidate process.
func Compare(ctx context.Context, request MatrixRequest) (MatrixReport, error) {
	return compare(ctx, request, Diagnose, time.Now)
}

func compare(ctx context.Context, request MatrixRequest, diagnoseCandidate diagnoseFunc, now func() time.Time) (MatrixReport, error) {
	if ctx == nil {
		return MatrixReport{}, errors.New("matrix context is required")
	}
	if request.Timeout < time.Second || request.Timeout > maxTimeout || request.Timeout%time.Second != 0 {
		return MatrixReport{}, fmt.Errorf("matrix candidate timeout must be between 1 and %s in whole seconds", maxTimeout)
	}
	if len(request.ExecutablePaths) < 1 || len(request.ExecutablePaths) > maxMatrixCandidates {
		return MatrixReport{}, fmt.Errorf("matrix requires between 1 and %d Codex executable candidates", maxMatrixCandidates)
	}
	if diagnoseCandidate == nil {
		diagnoseCandidate = Diagnose
	}
	if now == nil {
		now = time.Now
	}

	inputs := make([]matrixCandidateInput, 0, len(request.ExecutablePaths))
	seen := make(map[string]struct{}, len(request.ExecutablePaths))
	for _, selectedPath := range request.ExecutablePaths {
		if selectedPath == "" || !filepath.IsAbs(selectedPath) || filepath.Clean(selectedPath) != selectedPath {
			return MatrixReport{}, errors.New("each Codex candidate must be a clean absolute path")
		}
		canonical, err := filepath.EvalSymlinks(selectedPath)
		if err != nil {
			return MatrixReport{}, errors.New("a Codex candidate path could not be resolved")
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil || filepath.Clean(canonical) != canonical {
			return MatrixReport{}, errors.New("a Codex candidate did not resolve to a canonical path")
		}
		if _, ok := seen[canonical]; ok {
			return MatrixReport{}, errors.New("Codex candidate paths must resolve to distinct canonical binaries")
		}
		seen[canonical] = struct{}{}
		sha, err := hashMatrixExecutable(canonical)
		if err != nil {
			return MatrixReport{}, fmt.Errorf("invalid Codex candidate %q: %w", canonical, err)
		}
		inputs = append(inputs, matrixCandidateInput{path: canonical, sha256: sha})
	}

	started := now().UTC()
	report := MatrixReport{
		Schema: MatrixSchema, Scope: MatrixScope, Status: "indeterminate",
		StartedAt: started.Format(time.RFC3339Nano), TimeoutSeconds: int(request.Timeout / time.Second),
		Synthetic: true, Assurance: "unverified",
		NetworkMode: "allowlist", Candidates: make([]MatrixCandidate, 0, len(inputs)),
		Limitations: []string{
			"each supported result means only that the selected CLI completed one local synthetic Responses round trip under the recorded Sorna backend",
			"the in-process mock transport makes no provider request and provides no evidence of real inference, provider behavior, retention, or credential acceptance",
			"candidate SHA-256 values bind executable bytes before and after each probe; they are not an observed running-image identity or host attestation",
			"candidates run sequentially with the selected per-candidate timeout; total diagnostic execution is bounded by candidate count multiplied by that timeout",
			"the matrix reports compatibility only and never selects a runtime, replaces a binary, or falls back to another candidate",
			"this probe uses private temporary state and does not mutate a Sentinel, native Herdr, or project workspace",
			"read the backend and enforcement fields in each diagnostic; the matrix does not claim every candidate reached host policy preparation",
			"network_mode records intended synthetic broker scope; each candidate report records its observed Sorna preparation outcome",
		},
	}

	for _, input := range inputs {
		candidate := MatrixCandidate{
			ExecutablePath: input.path, ExecutableSHA256Before: input.sha256,
			Status: "indeterminate",
		}
		candidateContext, cancel := context.WithTimeout(ctx, request.Timeout)
		diagnostic, diagnoseErr := diagnoseCandidate(candidateContext, Request{ExecutablePath: input.path, Timeout: request.Timeout})
		cancel()
		if diagnoseErr == nil {
			candidate.Diagnostic = &diagnostic
		}
		shaAfter, hashErr := hashMatrixExecutable(input.path)
		if hashErr == nil {
			candidate.ExecutableSHA256After = shaAfter
		}
		switch {
		case hashErr != nil:
			candidate.ReasonCode = "executable-unavailable-after-diagnostic"
			candidate.Reason = "candidate executable could not be hashed after its diagnostic"
		case shaAfter != input.sha256:
			candidate.ReasonCode = "executable-changed-during-diagnostic"
			candidate.Reason = "candidate executable bytes changed during its diagnostic"
		case diagnoseErr != nil:
			candidate.ReasonCode = "diagnostic-failed"
			candidate.Reason = "synthetic diagnostic could not produce a report"
		case diagnostic.ExecutablePath != input.path || diagnostic.ExecutableSHA256 != input.sha256:
			candidate.ReasonCode = "diagnostic-identity-mismatch"
			candidate.Reason = "synthetic diagnostic did not bind the selected candidate bytes"
		case diagnostic.Status != "supported" && diagnostic.Status != "unsupported" && diagnostic.Status != "indeterminate":
			candidate.ReasonCode = "diagnostic-status-invalid"
			candidate.Reason = "synthetic diagnostic returned an unknown status"
		default:
			candidate.Status = diagnostic.Status
		}
		report.Candidates = append(report.Candidates, candidate)
	}

	report.Status = matrixOutcome(report.Candidates)
	for _, candidate := range report.Candidates {
		if candidate.Status == "indeterminate" {
			report.Limitations = append(report.Limitations, "one or more selected candidates remain indeterminate; inspect every candidate result even when another candidate is supported")
			break
		}
	}
	report.FinishedAt = now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func matrixOutcome(candidates []MatrixCandidate) string {
	allUnsupported := len(candidates) > 0
	for _, candidate := range candidates {
		if candidate.Status == "supported" {
			return "supported"
		}
		if candidate.Status != "unsupported" {
			allUnsupported = false
		}
	}
	if allUnsupported {
		return "unsupported"
	}
	return "indeterminate"
}

func hashMatrixExecutable(path string) (string, error) {
	file, err := openExecutable(path)
	if err != nil {
		return "", errors.New("executable cannot be opened")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("candidate must be a regular executable file")
	}
	if info.Size() < 1 || info.Size() > maxMatrixExecutableBytes {
		return "", fmt.Errorf("candidate size must be between 1 byte and %d bytes", maxMatrixExecutableBytes)
	}
	hash := sha256.New()
	read, err := io.Copy(hash, io.LimitReader(file, maxMatrixExecutableBytes+1))
	if err != nil {
		return "", errors.New("candidate executable could not be hashed")
	}
	if read != info.Size() || read > maxMatrixExecutableBytes {
		return "", errors.New("candidate executable changed or exceeded the hash limit")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
