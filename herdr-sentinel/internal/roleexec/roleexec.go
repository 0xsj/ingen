// Package roleexec runs noninteractive Sentinel role commands under Sorna's
// host policy. Reports describe this particular host execution and do not
// attest the host or establish an independent behavioral verdict.
package roleexec

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/capability"
	sentinelrun "ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workflowgate"
	"ingen/herdr-sentinel/internal/workspace"
	"ingen/sorna/execution"
	"ingen/sorna/policy"
)

const ReportSchema = "ingen.sentinel-role-execution/v1"

type Request struct {
	Root, WorkspacePath, RoleID, ExecutionID                     string
	ReceiptPath                                                  string
	PolicyPath                                                   string
	Command                                                      []string
	AllowedTools                                                 []string
	ExpectedToolSHA256                                           []string
	ProtectedPaths                                               []string
	ExpectedManifestSHA256                                       string
	ExpectedPolicySHA256                                         string
	ExpectedExecutableSHA256                                     string
	Governed                                                     bool
	ApprovalPath, ReviewPolicyPath, OraclePath                   string
	ExpectedApprovalSHA256, ExpectedReviewPolicySHA256           string
	ExpectedContractSHA256, ExpectedOraclePolicySHA256           string
	ExpectedContractSourceSHA256, ExpectedOraclePolicyFileSHA256 string
	ExpectedOracleSHA256                                         string
}

type Prepared struct {
	Policy                              policy.Sealed
	Command                             execution.Prepared
	Plan                                capability.Plan
	Role                                capability.Role
	ExecutionID                         string
	Root, RoleWorkspace, ScratchRoot    string
	ExecutableSHA256, ManifestSHA256    string
	AllowedToolSHA256                   []string
	DerivedReadRoots, DerivedWriteRoots []string
}

type Report struct {
	Schema                string               `json:"schema"`
	ExecutionID           string               `json:"execution_id"`
	WorkspaceID           string               `json:"workspace_id"`
	RoleID                string               `json:"role_id"`
	RoleKind              string               `json:"role_kind"`
	ManifestSHA256        string               `json:"manifest_sha256"`
	PolicySHA256          string               `json:"policy_sha256"`
	WorkspaceManifestPath string               `json:"workspace_manifest_path"`
	PolicyPath            string               `json:"policy_path"`
	ExecutablePath        string               `json:"executable_path"`
	ExecutableSHA256      string               `json:"executable_sha256"`
	AllowedTools          []string             `json:"allowed_tools"`
	AllowedToolSHA256     []string             `json:"allowed_tool_sha256"`
	Backend               string               `json:"backend"`
	Enforcement           string               `json:"enforcement"`
	Assurance             string               `json:"assurance"`
	EnforcementScope      string               `json:"enforcement_scope"`
	Limitations           []string             `json:"limitations"`
	NetworkMode           string               `json:"network_mode"`
	DeclaredReadRoots     []string             `json:"declared_read_roots"`
	DeclaredWriteRoots    []string             `json:"declared_write_roots"`
	DeclaredDenyRoots     []string             `json:"declared_deny_roots"`
	DerivedReadRoots      []string             `json:"derived_read_roots"`
	DerivedWriteRoots     []string             `json:"derived_write_roots"`
	StdoutPath            string               `json:"stdout_path"`
	StdoutSHA256          string               `json:"stdout_sha256,omitempty"`
	StderrPath            string               `json:"stderr_path"`
	StderrSHA256          string               `json:"stderr_sha256,omitempty"`
	StartedAt             string               `json:"started_at"`
	FinishedAt            string               `json:"finished_at"`
	Status                string               `json:"status"`
	ExitCode              *int                 `json:"exit_code,omitempty"`
	Reason                string               `json:"reason,omitempty"`
	Governance            *workflowgate.Result `json:"governance,omitempty"`
}

// Prepare compiles the exact manifest capabilities for one role and asks
// Sorna for a host-enforced command. Declared write roots are never copied to
// read roots. A unique subtree within the role workspace is the only derived
// read/write capability.
func Prepare(request Request) (Prepared, error) {
	if strings.TrimSpace(request.Root) == "" || strings.TrimSpace(request.WorkspacePath) == "" || strings.TrimSpace(request.RoleID) == "" || strings.TrimSpace(request.ExecutionID) == "" {
		return Prepared{}, fmt.Errorf("role execution requires root, workspace, role, and execution ID")
	}
	if len(request.Command) == 0 || strings.TrimSpace(request.Command[0]) == "" {
		return Prepared{}, fmt.Errorf("role execution command is required")
	}
	root, err := filepath.Abs(request.Root)
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve project root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Prepared{}, fmt.Errorf("canonicalize project root: %w", err)
	}
	workspacePath := filepath.Clean(request.WorkspacePath)
	if filepath.IsAbs(request.WorkspacePath) || workspacePath != request.WorkspacePath || workspacePath == "." || workspacePath == ".." || strings.HasPrefix(workspacePath, ".."+string(filepath.Separator)) {
		return Prepared{}, fmt.Errorf("workspace path must be normalized and project-relative")
	}
	if err := rejectSymlinkComponents(root, workspacePath); err != nil {
		return Prepared{}, fmt.Errorf("workspace path contains a symlink: %w", err)
	}
	plan, err := capability.FromFileUnderRoot(root, workspacePath)
	if err != nil {
		return Prepared{}, err
	}
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return Prepared{}, err
	}
	manifestFile, err := rooted.Open(filepath.ToSlash(workspacePath))
	if err != nil {
		rooted.Close()
		return Prepared{}, fmt.Errorf("open workspace manifest beneath project root: %w", err)
	}
	manifestInfo, err := manifestFile.Stat()
	if err != nil || !manifestInfo.Mode().IsRegular() {
		manifestFile.Close()
		rooted.Close()
		return Prepared{}, fmt.Errorf("workspace manifest is not a regular file")
	}
	manifestBytes, err := io.ReadAll(manifestFile)
	closeManifestErr := manifestFile.Close()
	if err == nil {
		err = closeManifestErr
	}
	if err != nil {
		rooted.Close()
		return Prepared{}, fmt.Errorf("read workspace manifest beneath project root: %w", err)
	}
	if hashBytes(manifestBytes) != plan.Workspace.Manifest.SHA256 {
		rooted.Close()
		return Prepared{}, fmt.Errorf("workspace manifest changed during role preparation")
	}
	if request.ExpectedManifestSHA256 != "" && plan.Workspace.Manifest.SHA256 != request.ExpectedManifestSHA256 {
		rooted.Close()
		return Prepared{}, fmt.Errorf("workspace manifest digest changed")
	}
	if request.ReceiptPath != "" {
		if err := rejectSymlinkComponents(root, request.ReceiptPath); err != nil {
			rooted.Close()
			return Prepared{}, fmt.Errorf("receipt path contains a symlink: %w", err)
		}
		receipt, err := loadReceiptRooted(rooted, request.ReceiptPath)
		if err != nil {
			rooted.Close()
			return Prepared{}, err
		}
		if receipt.Workspace.ID != plan.Workspace.ID || receipt.Workspace.Version != plan.Workspace.Version || receipt.Workspace.File != plan.Workspace.Manifest {
			rooted.Close()
			return Prepared{}, fmt.Errorf("Sentinel receipt does not match the current workspace manifest")
		}
	}
	if err := rooted.Close(); err != nil {
		return Prepared{}, err
	}
	var role *capability.Role
	for index := range plan.Roles {
		if plan.Roles[index].ID == request.RoleID {
			role = &plan.Roles[index]
			break
		}
	}
	if role == nil {
		return Prepared{}, fmt.Errorf("role %q is not declared in the workspace", request.RoleID)
	}
	for _, capability := range []struct {
		name  string
		paths []string
	}{{"read", role.ReadRoots}, {"write", role.WriteRoots}} {
		for _, path := range capability.paths {
			if filepath.Clean(filepath.FromSlash(path)) == "." {
				return Prepared{}, fmt.Errorf("isolated role %s capability cannot grant the project root", capability.name)
			}
		}
	}
	manifest, err := workspace.LoadBytes(workspacePath, manifestBytes)
	if err != nil {
		return Prepared{}, fmt.Errorf("load workspace controls: %w", err)
	}
	roleWorkspace := filepath.Join(root, role.Workspace)
	if err := sentinelrun.ValidateDirectoryPathUnderRoot(root, role.Workspace); err != nil {
		return Prepared{}, fmt.Errorf("validate role workspace: %w", err)
	}
	if info, statErr := os.Stat(roleWorkspace); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = fmt.Errorf("not a directory")
		}
		return Prepared{}, fmt.Errorf("inspect role workspace: %w", statErr)
	}
	allRoots := append(append(append([]string{}, role.ReadRoots...), role.WriteRoots...), role.DenyRoots...)
	if err := validateRootsUnderRoot(root, allRoots); err != nil {
		return Prepared{}, err
	}
	if err := rejectManySymlinkComponents(root, allRoots); err != nil {
		return Prepared{}, fmt.Errorf("role capability includes a symlink path: %w", err)
	}
	if err := validateTools(request.AllowedTools); err != nil {
		return Prepared{}, err
	}
	toolHashes, err := validateToolPins(request.AllowedTools, request.ExpectedToolSHA256)
	if err != nil {
		return Prepared{}, err
	}
	allowedTools := canonicalTools(request.AllowedTools)
	commandPath, err := resolveExecutable(request.Command[0])
	if err != nil {
		return Prepared{}, err
	}
	denyPaths := absoluteRoots(root, role.DenyRoots)
	for _, denied := range denyPaths {
		if execution.PathCovered([]string{denied}, commandPath) {
			return Prepared{}, fmt.Errorf("role command executable is beneath denied root %q", denied)
		}
	}
	for _, tool := range canonicalTools(request.AllowedTools) {
		for _, denied := range denyPaths {
			if execution.PathCovered([]string{denied}, tool) {
				return Prepared{}, fmt.Errorf("allowed tool %q is beneath denied root %q", tool, denied)
			}
		}
	}
	executableHash, err := hashFile(commandPath)
	if err != nil {
		return Prepared{}, fmt.Errorf("hash role command executable: %w", err)
	}
	if request.ExpectedExecutableSHA256 != "" && executableHash != request.ExpectedExecutableSHA256 {
		return Prepared{}, fmt.Errorf("role command executable digest changed")
	}
	command := append([]string(nil), request.Command...)
	command[0] = commandPath
	scratchRel := filepath.Join(role.Workspace, ".sentinel-roleexec", request.ExecutionID)
	if err := validID(request.ExecutionID); err != nil {
		return Prepared{}, err
	}
	scratchRoot := filepath.Join(root, scratchRel)
	if err := sentinelrun.ValidatePathUnderRoot(root, scratchRel); err != nil {
		return Prepared{}, fmt.Errorf("validate private scratch path: %w", err)
	}
	if err := rejectSymlinkComponents(root, scratchRel); err != nil {
		return Prepared{}, fmt.Errorf("private scratch path contains a symlink: %w", err)
	}
	if _, err := os.Lstat(scratchRoot); err == nil {
		return Prepared{}, fmt.Errorf("private scratch execution ID already exists")
	} else if !os.IsNotExist(err) {
		return Prepared{}, fmt.Errorf("inspect private scratch: %w", err)
	}
	for _, capabilityRoot := range append(append([]string{}, role.ReadRoots...), role.WriteRoots...) {
		for _, protected := range []string{".ingen/receipt.json", ".ingen/workspace.yaml", ".ingen/artifacts/native-sessions", ".ingen/artifacts/role-executions", ".ingen/artifacts/sessions"} {
			if pathOverlaps(capabilityRoot, protected) {
				return Prepared{}, fmt.Errorf("role capability %q exposes protected Sentinel state %q", capabilityRoot, protected)
			}
		}
	}
	readRoots := absoluteRoots(root, role.ReadRoots)
	derivedReads := []string{scratchRoot}
	readRoots = append(readRoots, derivedReads...)
	writeRoots := absoluteRoots(root, role.WriteRoots)
	scratchWritable := filepath.Join(scratchRoot, "rw")
	writeRoots = append(writeRoots, scratchWritable)
	denyRoots := denyPaths
	for _, denied := range denyRoots {
		if execution.PathCovered([]string{denied}, scratchRoot) {
			return Prepared{}, fmt.Errorf("role deny capability overlaps private scratch")
		}
	}
	policyDocument := makePolicy(plan, *role, request.ExecutionID, readRoots, writeRoots, denyRoots, allowedTools)
	sealed, err := policy.Seal(policyDocument)
	if err != nil {
		return Prepared{}, fmt.Errorf("seal role execution policy: %w", err)
	}
	if request.ExpectedPolicySHA256 != "" && sealed.SHA256 != request.ExpectedPolicySHA256 {
		return Prepared{}, fmt.Errorf("compiled role policy digest changed")
	}
	prepared, err := execution.Prepare(command, root, sealed)
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare host-enforced role command: %w", err)
	}
	if prepared.Enforcement != "host-enforced" || prepared.Backend != "macos-seatbelt" || prepared.NetworkMode != "disabled" {
		return Prepared{}, fmt.Errorf("role execution requires macOS Seatbelt host enforcement with network disabled (got backend=%q enforcement=%q network=%q)", prepared.Backend, prepared.Enforcement, prepared.NetworkMode)
	}
	if prepared.ExecutableSHA256 != executableHash {
		return Prepared{}, fmt.Errorf("role executable changed during policy preparation")
	}
	if !equalStrings(prepared.AllowedTools, allowedTools) {
		return Prepared{}, fmt.Errorf("resolved allowed tool identity changed during policy preparation")
	}
	protectedPaths := append([]string{workspacePath, request.ReceiptPath, request.ApprovalPath, request.ReviewPolicyPath, manifest.Contract.Path, plan.Workspace.OraclePolicy.Path, plan.Workspace.SubjectPolicy.Path}, request.ProtectedPaths...)
	for _, protected := range protectedPaths {
		if protected == "" {
			continue
		}
		if filepath.IsAbs(protected) || filepath.Clean(protected) != protected {
			return Prepared{}, fmt.Errorf("protected control path %q is not normalized and project-relative", protected)
		}
		if err := rejectSymlinkComponents(root, protected); err != nil {
			return Prepared{}, fmt.Errorf("protected input path contains a symlink: %w", err)
		}
		if overlapsAny(role.WriteRoots, filepath.Clean(protected)) {
			return Prepared{}, fmt.Errorf("role write capability overlaps protected input %q", protected)
		}
	}
	if request.Governed && role.Kind != "oracle-writer" && request.OraclePath != "" {
		for _, protected := range []string{request.OraclePath, filepath.ToSlash(filepath.Join(filepath.Dir(request.OraclePath), "hash.txt"))} {
			if overlapsAny(role.WriteRoots, filepath.Clean(protected)) {
				return Prepared{}, fmt.Errorf("governed role write capability overlaps frozen oracle input %q", protected)
			}
		}
	}
	return Prepared{Policy: sealed, Command: prepared, Plan: plan, Role: *role, ExecutionID: request.ExecutionID, Root: root, RoleWorkspace: roleWorkspace, ScratchRoot: scratchRoot, ExecutableSHA256: executableHash, ManifestSHA256: plan.Workspace.Manifest.SHA256, DerivedReadRoots: derivedReads, DerivedWriteRoots: []string{scratchWritable}, AllowedToolSHA256: toolHashes}, nil
}

// PersistPolicy writes a canonical sealed execution policy without replacing
// any existing file. The expected digest belongs in the immutable wrapper argv.
func PersistPolicy(root, relativePath string, sealed policy.Sealed) error {
	if err := validateIDFreeRelativePath(relativePath); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(root, relativePath); err != nil {
		return fmt.Errorf("policy output path contains a symlink: %w", err)
	}
	if err := sentinelrun.ValidatePathUnderRoot(root, relativePath); err != nil {
		return err
	}
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rooted.Close()
	relativePath = filepath.ToSlash(relativePath)
	if err := rooted.MkdirAll(filepath.ToSlash(filepath.Dir(filepath.FromSlash(relativePath))), 0o700); err != nil {
		return err
	}
	file, err := rooted.OpenFile(relativePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create sealed role policy: %w", err)
	}
	if _, err := file.Write(sealed.CanonicalJSON); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncRootDirectory(rooted, filepath.ToSlash(filepath.Dir(filepath.FromSlash(relativePath))))
}

// Execute re-derives every hash-bound input immediately before launching the
// noninteractive child. It writes captures and a role execution report outside
// every allowed role write root.
func Execute(ctx context.Context, request Request) (Report, string, error) {
	if request.ExpectedManifestSHA256 == "" || request.ExpectedPolicySHA256 == "" || request.ExpectedExecutableSHA256 == "" {
		return Report{}, "", fmt.Errorf("role execution requires immutable manifest, policy, and executable digests")
	}
	prepared, err := Prepare(request)
	if err != nil {
		return Report{}, "", err
	}
	if request.PolicyPath == "" || request.ExpectedPolicySHA256 == "" {
		return Report{}, "", fmt.Errorf("role execution requires a sealed policy reference and digest")
	}
	wantPolicyPath := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".policy.json"))
	if filepath.ToSlash(filepath.Clean(request.PolicyPath)) != wantPolicyPath {
		return Report{}, "", fmt.Errorf("sealed role policy path does not match the execution ID")
	}
	if err := rejectSymlinkComponents(prepared.Root, request.PolicyPath); err != nil {
		return Report{}, "", fmt.Errorf("sealed policy path contains a symlink: %w", err)
	}
	rooted, err := os.OpenRoot(prepared.Root)
	if err != nil {
		return Report{}, "", err
	}
	defer rooted.Close()
	policyHandle, err := rooted.Open(filepath.ToSlash(request.PolicyPath))
	if err != nil {
		return Report{}, "", fmt.Errorf("read sealed role policy: %w", err)
	}
	policyInfo, err := policyHandle.Stat()
	if err != nil || !policyInfo.Mode().IsRegular() {
		policyHandle.Close()
		return Report{}, "", fmt.Errorf("sealed role policy is not a regular file")
	}
	policyBytes, err := io.ReadAll(policyHandle)
	closeErr := policyHandle.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Report{}, "", fmt.Errorf("read sealed role policy: %w", err)
	}
	if hashBytes(policyBytes) != request.ExpectedPolicySHA256 || !bytes.Equal(policyBytes, prepared.Policy.CanonicalJSON) {
		return Report{}, "", fmt.Errorf("sealed role policy bytes changed")
	}
	if request.Governed {
		result, gateErr := checkGate(request, prepared)
		if gateErr != nil {
			return Report{}, "", gateErr
		}
		if err := compareGate(request, result); err != nil {
			return Report{}, "", err
		}
		if err := validateGateProtectedPaths(request, prepared, result); err != nil {
			return Report{}, "", err
		}
		return executePrepared(ctx, request, prepared, &result)
	}
	return executePrepared(ctx, request, prepared, nil)
}

func executePrepared(ctx context.Context, request Request, prepared Prepared, gate *workflowgate.Result) (Report, string, error) {
	artifactRelative := filepath.Join(".ingen", "artifacts", "role-executions")
	if err := rejectSymlinkComponents(prepared.Root, artifactRelative); err != nil {
		return Report{}, "", fmt.Errorf("role report path contains a symlink: %w", err)
	}
	if err := sentinelrun.ValidatePathUnderRoot(prepared.Root, artifactRelative); err != nil {
		return Report{}, "", fmt.Errorf("validate role report directory: %w", err)
	}
	rooted, err := os.OpenRoot(prepared.Root)
	if err != nil {
		return Report{}, "", err
	}
	defer rooted.Close()
	artifactDir := filepath.ToSlash(artifactRelative)
	if err := rooted.MkdirAll(artifactDir, 0o700); err != nil {
		return Report{}, "", err
	}
	claimPath := filepath.ToSlash(filepath.Join(artifactDir, request.ExecutionID+".claim"))
	claim, err := rooted.OpenFile(claimPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Report{}, "", fmt.Errorf("claim role execution ID exactly once: %w", err)
	}
	if _, err := io.WriteString(claim, prepared.Policy.SHA256+"\n"+prepared.ExecutableSHA256+"\n"); err != nil {
		claim.Close()
		return Report{}, "", err
	}
	if err := claim.Sync(); err != nil {
		claim.Close()
		return Report{}, "", err
	}
	if err := claim.Close(); err != nil {
		return Report{}, "", err
	}
	if err := syncRootDirectory(rooted, artifactDir); err != nil {
		return Report{}, "", fmt.Errorf("sync role execution claim: %w", err)
	}
	for _, path := range []string{prepared.ScratchRoot, filepath.Join(prepared.ScratchRoot, "rw", "home"), filepath.Join(prepared.ScratchRoot, "rw", "tmp"), filepath.Join(prepared.ScratchRoot, "rw", "cache")} {
		relativeScratch, _ := filepath.Rel(prepared.Root, path)
		if err := rooted.MkdirAll(filepath.ToSlash(relativeScratch), 0o700); err != nil {
			return Report{}, "", fmt.Errorf("create private role scratch: %w", err)
		}
		if err := rooted.Chmod(filepath.ToSlash(relativeScratch), 0o700); err != nil {
			return Report{}, "", fmt.Errorf("secure private role scratch: %w", err)
		}
	}
	stdoutRel, stderrRel, reportRel := filepath.ToSlash(filepath.Join(artifactDir, request.ExecutionID+".stdout")), filepath.ToSlash(filepath.Join(artifactDir, request.ExecutionID+".stderr")), filepath.ToSlash(filepath.Join(artifactDir, request.ExecutionID+".json"))
	stdout, err := rooted.OpenFile(stdoutRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Report{}, "", fmt.Errorf("create role stdout capture: %w", err)
	}
	stderr, err := rooted.OpenFile(stderrRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		stdout.Close()
		return Report{}, "", fmt.Errorf("create role stderr capture: %w", err)
	}
	started := time.Now().UTC()
	command := exec.Command(prepared.Command.Command[0], prepared.Command.Command[1:]...)
	command.Dir = prepared.ScratchRoot
	command.Env = privateEnvironment(prepared.ScratchRoot)
	command.Stdout, command.Stderr = stdout, stderr
	latestGate, identityErr := verifyLaunchInputs(rooted, request, prepared)
	if identityErr != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return Report{}, "", identityErr
	}
	if gate != nil {
		gate = latestGate
	}
	runErr, startErr, canceled, cancelReason := runWithCancellation(ctx, command)
	stdoutSyncErr := stdout.Sync()
	stderrSyncErr := stderr.Sync()
	stdoutCloseErr := stdout.Close()
	stderrCloseErr := stderr.Close()
	stdoutHash, stdoutHashErr := hashRootFile(rooted, stdoutRel)
	stderrHash, stderrHashErr := hashRootFile(rooted, stderrRel)
	finished := time.Now().UTC()
	report := Report{Schema: ReportSchema, ExecutionID: request.ExecutionID, WorkspaceID: prepared.Plan.Workspace.ID, RoleID: prepared.Role.ID, RoleKind: prepared.Role.Kind, ManifestSHA256: prepared.ManifestSHA256, PolicySHA256: prepared.Policy.SHA256, WorkspaceManifestPath: filepath.ToSlash(request.WorkspacePath), PolicyPath: filepath.ToSlash(request.PolicyPath), ExecutablePath: prepared.Command.ExecutablePath, ExecutableSHA256: prepared.ExecutableSHA256, AllowedTools: canonicalTools(request.AllowedTools), AllowedToolSHA256: prepared.AllowedToolSHA256, Backend: prepared.Command.Backend, Enforcement: prepared.Command.Enforcement, Assurance: "unverified", EnforcementScope: "filesystem, tool, and network rules", Limitations: []string{"executable digest is a prelaunch byte check, not running-image identity", "Darwin runtime bootstrap and ancestor metadata are available", "no process namespace", "no control over prior role context", "no host attestation", "noninteractive execution only"}, NetworkMode: prepared.Command.NetworkMode, DeclaredReadRoots: absoluteRoots(prepared.Root, prepared.Role.ReadRoots), DeclaredWriteRoots: absoluteRoots(prepared.Root, prepared.Role.WriteRoots), DeclaredDenyRoots: absoluteRoots(prepared.Root, prepared.Role.DenyRoots), DerivedReadRoots: prepared.DerivedReadRoots, DerivedWriteRoots: prepared.DerivedWriteRoots, StdoutPath: filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".stdout")), StderrPath: filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".stderr")), StartedAt: started.Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano), Status: "completed"}
	if gate != nil {
		report.Governance = gate
	}
	if startErr == nil && runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code := exitErr.ExitCode()
			report.ExitCode = &code
		}
	} else if startErr == nil && runErr == nil {
		code := 0
		report.ExitCode = &code
	}
	if startErr != nil {
		report.Status = "failed"
		report.Reason = "child process could not start: " + startErr.Error()
	} else if canceled {
		report.Status = "canceled"
		report.Reason = cancelReason
		if runErr != nil {
			report.Reason += ": " + runErr.Error()
		}
	} else if runErr != nil {
		report.Status = "failed"
		report.Reason = runErr.Error()
	}
	if stdoutHashErr == nil {
		report.StdoutSHA256 = stdoutHash
	}
	if stderrHashErr == nil {
		report.StderrSHA256 = stderrHash
	}
	captureErr := errors.Join(stdoutSyncErr, stderrSyncErr, stdoutCloseErr, stderrCloseErr, stdoutHashErr, stderrHashErr)
	if captureErr != nil {
		report.Status = "indeterminate"
		report.Reason = "capture durability or digest failed: " + captureErr.Error()
	}
	if err := writeReportAtomic(rooted, artifactDir, reportRel, report); err != nil {
		return report, "", fmt.Errorf("write role execution report: %w", err)
	}
	if startErr != nil {
		return report, filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".json")), startErr
	}
	if runErr != nil {
		return report, filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".json")), runErr
	}
	if captureErr != nil {
		return report, filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".json")), captureErr
	}
	if canceled {
		return report, filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".json")), fmt.Errorf("role process canceled (%s)", cancelReason)
	}
	return report, filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".json")), nil
}

func verifyLaunchInputs(rooted *os.Root, request Request, prepared Prepared) (*workflowgate.Result, error) {
	manifestFile, err := rooted.Open(filepath.ToSlash(request.WorkspacePath))
	if err != nil {
		return nil, err
	}
	manifestInfo, err := manifestFile.Stat()
	if err != nil || !manifestInfo.Mode().IsRegular() {
		manifestFile.Close()
		return nil, fmt.Errorf("workspace manifest changed type before child start")
	}
	manifestBytes, err := io.ReadAll(manifestFile)
	closeErr := manifestFile.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || hashBytes(manifestBytes) != prepared.ManifestSHA256 {
		return nil, fmt.Errorf("workspace manifest changed before child start")
	}
	policyFile, err := rooted.Open(filepath.ToSlash(request.PolicyPath))
	if err != nil {
		return nil, fmt.Errorf("open sealed policy before child start: %w", err)
	}
	policyInfo, err := policyFile.Stat()
	if err != nil || !policyInfo.Mode().IsRegular() {
		policyFile.Close()
		return nil, fmt.Errorf("sealed policy changed type before child start")
	}
	policyBytes, err := io.ReadAll(policyFile)
	closeErr = policyFile.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || hashBytes(policyBytes) != request.ExpectedPolicySHA256 || !bytes.Equal(policyBytes, prepared.Policy.CanonicalJSON) {
		return nil, fmt.Errorf("sealed policy changed before child start")
	}
	executableHash, err := hashFile(prepared.Command.ExecutablePath)
	if err != nil || executableHash != request.ExpectedExecutableSHA256 {
		return nil, fmt.Errorf("role executable changed before child start")
	}
	toolHashes, err := validateToolPins(request.AllowedTools, request.ExpectedToolSHA256)
	if err != nil || !equalStrings(toolHashes, prepared.AllowedToolSHA256) {
		return nil, fmt.Errorf("allowed tool identity changed before child start")
	}
	if !request.Governed {
		return nil, nil
	}
	result, err := checkGate(request, prepared)
	if err != nil {
		return nil, err
	}
	if err := compareGate(request, result); err != nil {
		return nil, err
	}
	if err := validateGateProtectedPaths(request, prepared, result); err != nil {
		return nil, err
	}
	return &result, nil
}

func checkGate(request Request, prepared Prepared) (workflowgate.Result, error) {
	stage := workflowgate.StageImplementation
	switch prepared.Role.Kind {
	case "oracle-writer":
		stage = workflowgate.StageOracle
	case "verifier", "mutation-runner":
		stage = workflowgate.StageVerification
	case "implementation":
		stage = workflowgate.StageImplementation
	default:
		return workflowgate.Result{}, fmt.Errorf("governed execution is unsupported for role kind %q", prepared.Role.Kind)
	}
	return workflowgate.Check(workflowgate.Request{Root: prepared.Root, WorkspacePath: request.WorkspacePath, ApprovalPath: request.ApprovalPath, ReviewPolicyPath: request.ReviewPolicyPath, Stage: stage, OraclePath: request.OraclePath})
}

func compareGate(request Request, result workflowgate.Result) error {
	checks := map[string][2]string{"approval": {request.ExpectedApprovalSHA256, result.ApprovalSHA256}, "active review policy": {request.ExpectedReviewPolicySHA256, result.ReviewPolicySHA256}, "contract": {request.ExpectedContractSHA256, result.ContractSHA256}, "contract source": {request.ExpectedContractSourceSHA256, result.ContractSourceSHA256}, "oracle policy": {request.ExpectedOraclePolicySHA256, result.OraclePolicySHA256}, "oracle policy file": {request.ExpectedOraclePolicyFileSHA256, result.OraclePolicyFileSHA256}, "oracle": {request.ExpectedOracleSHA256, result.OracleSHA256}}
	for name, pair := range checks {
		if pair[0] != "" && pair[0] != pair[1] {
			return fmt.Errorf("governed %s digest changed", name)
		}
		if name != "oracle" && name != "contract source" && name != "oracle policy file" && pair[0] == "" {
			return fmt.Errorf("governed execution is missing expected %s digest", name)
		}
	}
	if result.Stage != workflowgate.StageOracle && request.ExpectedOracleSHA256 == "" {
		return fmt.Errorf("governed execution is missing the frozen oracle digest")
	}
	if result.WorkspaceManifestSHA256 != request.ExpectedManifestSHA256 {
		return fmt.Errorf("governed workspace manifest digest changed")
	}
	return nil
}

func validateGateProtectedPaths(request Request, prepared Prepared, result workflowgate.Result) error {
	protected := make(map[string]bool, len(request.ProtectedPaths))
	for _, item := range request.ProtectedPaths {
		protected[filepath.ToSlash(filepath.Clean(item))] = true
	}
	for _, artifact := range result.ApprovalArtifacts {
		path := filepath.ToSlash(filepath.Clean(artifact.Path))
		if !protected[path] {
			return fmt.Errorf("governed approval input %q was not pinned into the immutable execution request", artifact.Path)
		}
		if overlapsAny(prepared.Role.WriteRoots, filepath.FromSlash(path)) {
			return fmt.Errorf("governed role write capability overlaps approved authority input %q", artifact.Path)
		}
	}
	return nil
}

func makePolicy(plan capability.Plan, role capability.Role, id string, reads, writes, denies []string, tools []string) policy.Document {
	entries := func(paths []string, reason string) []any {
		output := make([]any, 0, len(paths))
		for _, path := range paths {
			output = append(output, map[string]any{"path": path, "reason": reason})
		}
		return output
	}
	allowed := make([]any, 0, len(tools))
	for _, tool := range tools {
		allowed = append(allowed, map[string]any{"name": tool, "purpose": "explicitly approved child tool"})
	}
	return policy.Document{Policy: map[string]any{"schema": policy.Schema, "id": "sentinel-role-" + id, "version": 1, "status": "draft", "purpose": "noninteractive Sentinel role execution", "enforcement": "host-enforced", "filesystem": map[string]any{"read": entries(reads, "manifest read capability or derived role workspace"), "write": entries(writes, "manifest write capability or private execution scratch"), "deny": entries(denies, "manifest deny capability")}, "network": map[string]any{"mode": "disabled"}, "process": map[string]any{"subject_id": plan.Workspace.ID + "/" + role.ID, "can_invoke_subject": false, "allowed_tools": allowed}}}
}

func privateEnvironment(scratch string) []string {
	private := filepath.Join(scratch, "rw")
	return []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + filepath.Join(private, "home"), "TMPDIR=" + filepath.Join(private, "tmp"), "XDG_CACHE_HOME=" + filepath.Join(private, "cache"), "LANG=C", "LC_ALL=C"}
}

func validateToolPins(tools, expected []string) ([]string, error) {
	if len(tools) != len(expected) {
		return nil, fmt.Errorf("every allowed tool requires an immutable SHA-256 pin")
	}
	hashes := make([]string, len(tools))
	for index, raw := range tools {
		path, err := filepath.EvalSymlinks(raw)
		if err != nil {
			return nil, fmt.Errorf("resolve allowed tool: %w", err)
		}
		hash, err := hashFile(path)
		if err != nil {
			return nil, fmt.Errorf("hash allowed tool: %w", err)
		}
		if expected[index] != hash {
			return nil, fmt.Errorf("allowed tool %q digest changed", raw)
		}
		hashes[index] = hash
	}
	return hashes, nil
}

// ToolIdentity resolves and hashes an explicit tool path for immutable argv
// construction. The returned path is canonical and absolute.
func ToolIdentity(raw string) (string, string, error) {
	if !filepath.IsAbs(raw) || filepath.Clean(raw) != raw {
		return "", "", fmt.Errorf("tool path must be clean and absolute")
	}
	path, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", "", fmt.Errorf("tool is not an executable regular file")
	}
	hash, err := hashFile(path)
	return path, hash, err
}

func validateTools(tools []string) error {
	seen := map[string]bool{}
	for _, raw := range tools {
		if !filepath.IsAbs(raw) || filepath.Clean(raw) != raw {
			return fmt.Errorf("allowed tool path %q must be clean and absolute", raw)
		}
		path, err := filepath.EvalSymlinks(raw)
		if err != nil {
			return fmt.Errorf("resolve allowed tool %q: %w", raw, err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return fmt.Errorf("allowed tool %q is not an executable regular file", raw)
		}
		if seen[path] {
			return fmt.Errorf("allowed tool %q is duplicated", raw)
		}
		seen[path] = true
	}
	return nil
}

func resolveExecutable(raw string) (string, error) {
	var path string
	var err error
	if strings.ContainsRune(raw, filepath.Separator) {
		if !filepath.IsAbs(raw) || filepath.Clean(raw) != raw {
			return "", fmt.Errorf("role executable path must be clean and absolute")
		}
		path, err = filepath.EvalSymlinks(raw)
	} else {
		path, err = exec.LookPath(raw)
		if err == nil && !filepath.IsAbs(path) {
			path, err = filepath.Abs(path)
		}
		if err == nil {
			path, err = filepath.EvalSymlinks(path)
		}
	}
	if err != nil {
		return "", fmt.Errorf("resolve role executable: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("role executable %q is not an executable regular file", raw)
	}
	return path, nil
}

func validateRootsUnderRoot(root string, roots []string) error {
	for _, raw := range roots {
		if err := sentinelrun.ValidatePathUnderRoot(root, raw); err != nil {
			return fmt.Errorf("manifest capability path %q is unsafe: %w", raw, err)
		}
	}
	return nil
}

func rejectManySymlinkComponents(root string, paths []string) error {
	for _, path := range paths {
		if err := rejectSymlinkComponents(root, path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func rejectSymlinkComponents(root, relative string) error {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || strings.ContainsRune(relative, '\\') {
		return fmt.Errorf("path must be normalized and project-relative")
	}
	current, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("component %q is a symbolic link", current)
		}
	}
	return nil
}
func absoluteRoots(root string, roots []string) []string {
	result := make([]string, 0, len(roots))
	for _, item := range roots {
		result = append(result, filepath.Join(root, item))
	}
	return result
}
func overlapsAny(roots []string, path string) bool {
	absolute := filepath.Clean(path)
	for _, root := range roots {
		if root == absolute || strings.HasPrefix(root, absolute+string(filepath.Separator)) || strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
func pathOverlaps(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}
func canonicalTools(tools []string) []string {
	out := make([]string, 0, len(tools))
	for _, raw := range tools {
		path, err := filepath.EvalSymlinks(raw)
		if err == nil {
			out = append(out, path)
		} else {
			out = append(out, raw)
		}
	}
	return out
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func validID(value string) error {
	if value == "" || len(value) > 80 {
		return fmt.Errorf("execution ID is invalid")
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("execution ID contains unsafe characters")
		}
	}
	return nil
}
func validateIDFreeRelativePath(path string) error {
	clean := filepath.Clean(path)
	if filepath.IsAbs(path) || clean != path || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.ContainsRune(path, '\\') {
		return fmt.Errorf("policy path must be normalized and project-relative")
	}
	return nil
}
func structFileRef(path string) ciresult.FileRef {
	return ciresult.FileRef{Path: filepath.ToSlash(path)}
}

func loadReceiptRooted(rooted *os.Root, relative string) (sentinelrun.Receipt, error) {
	if err := validateIDFreeRelativePath(relative); err != nil {
		return sentinelrun.Receipt{}, fmt.Errorf("invalid receipt path: %w", err)
	}
	file, err := rooted.Open(filepath.ToSlash(relative))
	if err != nil {
		return sentinelrun.Receipt{}, fmt.Errorf("open Sentinel receipt beneath project root: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return sentinelrun.Receipt{}, fmt.Errorf("Sentinel receipt is not a regular file")
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return sentinelrun.Receipt{}, fmt.Errorf("read Sentinel receipt beneath project root: %w", err)
	}
	var receipt sentinelrun.Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return sentinelrun.Receipt{}, fmt.Errorf("decode Sentinel receipt: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return sentinelrun.Receipt{}, fmt.Errorf("validate Sentinel receipt: %w", err)
	}
	return receipt, nil
}
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func writeReportAtomic(rooted *os.Root, directory, finalPath string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temporary := finalPath + ".tmp-" + hex.EncodeToString(nonce[:])
	file, err := rooted.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := rooted.Link(temporary, finalPath); err != nil {
		_ = rooted.Remove(temporary)
		return err
	}
	if err := rooted.Remove(temporary); err != nil {
		return err
	}
	if err := syncRootDirectory(rooted, directory); err != nil {
		_ = rooted.Remove(finalPath)
		_ = syncRootDirectory(rooted, directory)
		return fmt.Errorf("publish role execution report: %w", err)
	}
	return nil
}

func syncRootDirectory(rooted *os.Root, path string) error {
	directory, err := rooted.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func hashRootFile(rooted *os.Root, path string) (string, error) {
	file, err := rooted.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("captured output is not a regular file")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
