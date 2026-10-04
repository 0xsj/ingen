// Package evidence verifies Sentinel role-execution reports and the exact
// project files they reference. It validates recorded evidence only; it does
// not attest the host or independently verify role behavior.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/hammond/governance"
	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/roleexec"
	"ingen/herdr-sentinel/internal/workspace"
	"ingen/sorna/contract"
	"ingen/sorna/execution"
	"ingen/sorna/policy"
)

const ExplanationSchema = "ingen.sentinel-role-execution-explanation/v1"

const (
	FileWorkspaceManifest = "workspace-manifest"
	FileSealedPolicy      = "sealed-policy"
	FileStdout            = "stdout"
	FileStderr            = "stderr"
	FileAgentPromptSource = "agent-prompt-source"
	FileAgentPrompt       = "agent-prompt-snapshot"
	FileContractSource    = "contract-source"
	FileContractFixture   = "contract-fixture"
	FileOraclePolicy      = "oracle-policy"
)

const maxEvidenceFileBytes = 128 << 20

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var executionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
var brokerCredentialNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Report is the public role-execution report shape produced by roleexec.
type Report = roleexec.Report

// File contains exact bytes read beneath the project root and their verified
// digest. Paths are normalized and relative to the root passed to Verify.
type File struct {
	Kind   string
	Path   string
	SHA256 string
	Bytes  []byte
}

// Verified holds a validated report and its referenced evidence bytes. The
// report itself remains separate from Files so consumers do not need to
// duplicate its custody record.
type Verified struct {
	Report       Report
	ReportBytes  []byte
	ReportSHA256 string
	Files        []File

	root       string
	reportPath string
	snapshot   *verifiedSnapshot
}

type verifiedSnapshot struct {
	report      Report
	reportBytes []byte
	reportSHA   string
	reportPath  string
	root        string
	files       []File
}

// Verify reads a report and its referenced files beneath root, then checks
// their exact bytes and cross-file identities. An empty expectedSHA256 is
// accepted for the first local collection only; callers should supply the
// previously recorded report digest for later collection or custody replay.
// This function is read-only and does not claim independent host attestation.
func Verify(root, reportPath, expectedSHA256 string) (Verified, error) {
	if strings.TrimSpace(root) == "" {
		return Verified{}, fmt.Errorf("evidence root is required")
	}
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		return Verified{}, fmt.Errorf("resolve evidence root: %w", err)
	}
	canonicalRoot, err = filepath.EvalSymlinks(canonicalRoot)
	if err != nil {
		return Verified{}, fmt.Errorf("canonicalize evidence root: %w", err)
	}
	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil || !rootInfo.IsDir() {
		return Verified{}, fmt.Errorf("evidence root is not a directory")
	}
	relativeReport, err := cleanRelative("role-execution report", reportPath)
	if err != nil {
		return Verified{}, err
	}
	if expectedSHA256 != "" && !validSHA256(expectedSHA256) {
		return Verified{}, fmt.Errorf("expected report SHA-256 is malformed")
	}
	rooted, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return Verified{}, fmt.Errorf("open evidence root: %w", err)
	}
	defer rooted.Close()
	if err := rejectSymlinkComponents(canonicalRoot, relativeReport); err != nil {
		return Verified{}, fmt.Errorf("report path contains a symbolic link: %w", err)
	}

	reportBytes, err := readRooted(rooted, relativeReport)
	if err != nil {
		return Verified{}, fmt.Errorf("read role-execution report: %w", err)
	}
	reportSHA := digest(reportBytes)
	if expectedSHA256 != "" && reportSHA != expectedSHA256 {
		return Verified{}, fmt.Errorf("role-execution report digest does not match expected SHA-256")
	}
	if err := rejectDuplicateJSONKeys(reportBytes); err != nil {
		return Verified{}, fmt.Errorf("strictly decode role-execution report: %w", err)
	}
	var rawReport map[string]json.RawMessage
	if err := json.Unmarshal(reportBytes, &rawReport); err != nil {
		return Verified{}, fmt.Errorf("decode role-execution report fields: %w", err)
	}
	if rawContext, exists := rawReport["agent_context"]; exists && bytes.Equal(bytes.TrimSpace(rawContext), []byte("null")) {
		return Verified{}, fmt.Errorf("agent_context cannot be null when present")
	}
	if rawBroker, exists := rawReport["broker_execution"]; exists && bytes.Equal(bytes.TrimSpace(rawBroker), []byte("null")) {
		return Verified{}, fmt.Errorf("broker_execution cannot be null when present")
	}
	if rawAgent, exists := rawReport["agent_context"]; exists && !bytes.Equal(bytes.TrimSpace(rawAgent), []byte("null")) {
		var agentFields map[string]json.RawMessage
		if err := json.Unmarshal(rawAgent, &agentFields); err != nil {
			return Verified{}, fmt.Errorf("decode agent context fields: %w", err)
		}
		if rawBroker, present := agentFields["broker"]; present {
			if bytes.Equal(bytes.TrimSpace(rawBroker), []byte("null")) {
				return Verified{}, fmt.Errorf("agent_context.broker cannot be null when present")
			}
			if err := requireNonNullJSONFields(rawBroker, "provider endpoint credential_source credential_env max_requests max_output_tokens timeout_seconds"); err != nil {
				return Verified{}, fmt.Errorf("broker context is incomplete: %w", err)
			}
		}
	}
	if rawBroker, exists := rawReport["broker_execution"]; exists {
		if err := requireNonNullJSONFields(rawBroker, "status provider endpoint model max_requests max_output_tokens timeout_seconds stats"); err != nil {
			return Verified{}, fmt.Errorf("broker execution is incomplete: %w", err)
		}
		var brokerFields map[string]json.RawMessage
		if err := json.Unmarshal(rawBroker, &brokerFields); err != nil {
			return Verified{}, fmt.Errorf("decode broker execution fields: %w", err)
		}
		if err := requireNonNullJSONFields(brokerFields["stats"], "started_at requests_received requests_forwarded requests_rejected upstream_failures in_flight last_status"); err != nil {
			return Verified{}, fmt.Errorf("broker statistics are incomplete: %w", err)
		}
		var statsFields map[string]json.RawMessage
		if err := json.Unmarshal(brokerFields["stats"], &statsFields); err != nil {
			return Verified{}, fmt.Errorf("decode broker statistics fields: %w", err)
		}
		for _, optional := range []string{"closed_at", "last_completed_at"} {
			if value, present := statsFields[optional]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return Verified{}, fmt.Errorf("broker statistics field %s cannot be null when present", optional)
			}
		}
	}
	if err := requireJSONFields(reportBytes, reportRequiredFields); err != nil {
		return Verified{}, fmt.Errorf("role-execution report is incomplete: %w", err)
	}
	var report Report
	if err := decodeStrict(reportBytes, &report); err != nil {
		return Verified{}, fmt.Errorf("decode role-execution report: %w", err)
	}
	if err := validateReport(report); err != nil {
		return Verified{}, err
	}
	expectedDir := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions"))
	if relativeReport != filepath.ToSlash(filepath.Join(expectedDir, report.ExecutionID+".json")) || report.PolicyPath != filepath.ToSlash(filepath.Join(expectedDir, report.ExecutionID+".policy.json")) || report.StdoutPath != filepath.ToSlash(filepath.Join(expectedDir, report.ExecutionID+".stdout")) || report.StderrPath != filepath.ToSlash(filepath.Join(expectedDir, report.ExecutionID+".stderr")) {
		return Verified{}, fmt.Errorf("role-execution report and capture paths do not match execution identity")
	}
	if report.AllowedTools == nil || report.AllowedToolSHA256 == nil || report.Limitations == nil || report.DeclaredReadRoots == nil || report.DeclaredWriteRoots == nil || report.DeclaredDenyRoots == nil || report.DerivedReadRoots == nil || report.DerivedWriteRoots == nil {
		return Verified{}, fmt.Errorf("role-execution report required array fields cannot be null")
	}

	verified := Verified{Report: report, ReportBytes: bytes.Clone(reportBytes), ReportSHA256: reportSHA, root: canonicalRoot, reportPath: relativeReport}
	seenPaths := map[string]string{relativeReport: reportSHA}
	seenKinds := make(map[string]bool)
	addContents := func(kind, path string, contents []byte, expected string) error {
		cleanPath, err := cleanRelative(kind, path)
		if err != nil {
			return err
		}
		actual := digest(contents)
		if expected != "" && actual != expected {
			return fmt.Errorf("%s %q digest mismatch", kind, cleanPath)
		}
		if cleanPath == relativeReport {
			return fmt.Errorf("role-execution report cannot also be a referenced input")
		}
		if previous, found := seenPaths[cleanPath]; found && previous != actual {
			return fmt.Errorf("evidence path %q is reused with conflicting bytes", cleanPath)
		}
		identity := kind + "\x00" + cleanPath
		if seenKinds[identity] {
			return fmt.Errorf("evidence path %q is referenced more than once as %s", cleanPath, kind)
		}
		seenPaths[cleanPath] = actual
		seenKinds[identity] = true
		verified.Files = append(verified.Files, File{Kind: kind, Path: cleanPath, SHA256: actual, Bytes: bytes.Clone(contents)})
		return nil
	}
	addFile := func(kind, path, expected string, optional bool) error {
		cleanPath, err := cleanRelative(kind, path)
		if err != nil {
			return err
		}
		if err := rejectSymlinkComponents(canonicalRoot, cleanPath); err != nil {
			return fmt.Errorf("%s path contains a symbolic link: %w", kind, err)
		}
		contents, err := readRooted(rooted, cleanPath)
		if err != nil {
			if optional && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("read %s %q: %w", kind, cleanPath, err)
		}
		return addContents(kind, cleanPath, contents, expected)
	}

	if err := verifyWorkspaceAndPolicy(rooted, canonicalRoot, &verified, addContents); err != nil {
		return Verified{}, err
	}
	for _, item := range []struct {
		kind, path, hash string
		optional         bool
	}{{FileStdout, report.StdoutPath, report.StdoutSHA256, report.Status == "indeterminate" && report.StdoutSHA256 == ""}, {FileStderr, report.StderrPath, report.StderrSHA256, report.Status == "indeterminate" && report.StderrSHA256 == ""}} {
		if item.hash != "" && !validSHA256(item.hash) {
			return Verified{}, fmt.Errorf("%s digest is malformed", item.kind)
		}
		if err := addFile(item.kind, item.path, item.hash, item.optional); err != nil {
			return Verified{}, err
		}
	}
	if report.AgentContext != nil {
		if err := verifyAgentContext(rooted, canonicalRoot, report, addContents); err != nil {
			return Verified{}, err
		}
	}
	if report.Governance != nil {
		if err := verifyGovernance(&verified, addFile); err != nil {
			return Verified{}, err
		}
	}
	sort.Slice(verified.Files, func(i, j int) bool {
		if verified.Files[i].Kind == verified.Files[j].Kind {
			return verified.Files[i].Path < verified.Files[j].Path
		}
		return verified.Files[i].Kind < verified.Files[j].Kind
	})
	privateReport := Report{}
	if err := decodeStrict(reportBytes, &privateReport); err != nil {
		return Verified{}, err
	}
	privateFiles := make([]File, len(verified.Files))
	for i, item := range verified.Files {
		privateFiles[i] = File{Kind: item.Kind, Path: item.Path, SHA256: item.SHA256, Bytes: bytes.Clone(item.Bytes)}
	}
	verified.snapshot = &verifiedSnapshot{report: privateReport, reportBytes: bytes.Clone(reportBytes), reportSHA: reportSHA, reportPath: relativeReport, root: canonicalRoot, files: privateFiles}
	return verified, nil
}

// BuildCIResult returns a deterministic producer envelope. sourceRoot is an
// identity label (normally the project root); FileRef paths remain relative to
// the project root used by Verify. Passing is a role process outcome only and
// is not a behavioral correctness verdict.
func (v Verified) BuildCIResult(sourceRoot string) (ciresult.Artifact, error) {
	if v.snapshot == nil || v.root == "" || v.reportPath == "" || v.ReportSHA256 == "" {
		return ciresult.Artifact{}, fmt.Errorf("CI result requires a value returned by Verify")
	}
	snapshot := v.snapshot
	if v.root != snapshot.root || v.reportPath != snapshot.reportPath || v.ReportSHA256 != snapshot.reportSHA || !bytes.Equal(v.ReportBytes, snapshot.reportBytes) || !reflect.DeepEqual(v.Report, snapshot.report) || !sameFiles(v.Files, snapshot.files) {
		return ciresult.Artifact{}, fmt.Errorf("verified evidence was modified after verification")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		return ciresult.Artifact{}, fmt.Errorf("CI source root is required")
	}
	resolvedSource, err := filepath.Abs(sourceRoot)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("resolve CI source root: %w", err)
	}
	resolvedSource, err = filepath.EvalSymlinks(resolvedSource)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("canonicalize CI source root: %w", err)
	}
	info, err := os.Stat(resolvedSource)
	if err != nil || !info.IsDir() || resolvedSource != snapshot.root {
		return ciresult.Artifact{}, fmt.Errorf("CI source root must equal the verified project root")
	}
	report := snapshot.report
	status, exitCode, resultError := ciStatus(report)
	explanation := struct {
		Schema       string `json:"schema"`
		ExecutionID  string `json:"execution_id"`
		WorkspaceID  string `json:"workspace_id"`
		RoleID       string `json:"role_id"`
		RoleKind     string `json:"role_kind"`
		Outcome      string `json:"outcome"`
		ReportStatus string `json:"report_status"`
		ReportSHA256 string `json:"report_sha256"`
		Enforcement  string `json:"enforcement"`
		Assurance    string `json:"assurance"`
		Meaning      string `json:"meaning"`
	}{ExplanationSchema, report.ExecutionID, report.WorkspaceID, report.RoleID, report.RoleKind, status, report.Status, snapshot.reportSHA, report.Enforcement, report.Assurance, explanationMeaning(report)}
	reportJSON := bytes.Clone(snapshot.reportBytes)
	explanationJSON, err := json.Marshal(explanation)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	inputs := map[string]ciresult.FileRef{
		"role-execution-report": {Path: snapshot.reportPath, SHA256: snapshot.reportSHA},
	}
	var policyRef *ciresult.FileRef
	for _, file := range snapshot.files {
		ref := ciresult.FileRef{Path: file.Path, SHA256: file.SHA256}
		name := file.Kind
		if name == FileSealedPolicy {
			policyCopy := ref
			policyRef = &policyCopy
		}
		key := name
		if _, exists := inputs[key]; exists {
			key = name + ":" + file.Path
		}
		inputs[key] = ref
	}
	artifact := ciresult.Artifact{
		Schema: ciresult.Schema, Tool: "sentinel", Kind: "role-execution",
		Status: status, ExitCode: exitCode, CreatedAt: report.FinishedAt,
		Source: ciresult.Source{Root: resolvedSource}, Policy: policyRef, Inputs: inputs,
		Report: json.RawMessage(reportJSON), Explanation: explanationJSON, Error: resultError,
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("build Sentinel role-execution CI result: %w", err)
	}
	return artifact, nil
}

func explanationMeaning(report Report) string {
	meaning := "This result describes role process execution only; it does not establish behavioral correctness or independent host attestation."
	if report.AgentContext != nil && report.AgentContext.Broker != nil {
		meaning += " Broker statistics describe Sentinel-local request lifecycle only; they do not attest provider inference, model context, or retention."
	}
	if report.Governance != nil && report.Governance.Stage != "oracle" {
		meaning += " The frozen-oracle digest is recorded without a path in role-execution report v1 and is not re-read here."
	}
	return meaning
}

const reportRequiredFields = "schema execution_id workspace_id role_id role_kind manifest_sha256 policy_sha256 workspace_manifest_path policy_path executable_path executable_sha256 allowed_tools allowed_tool_sha256 backend enforcement assurance enforcement_scope limitations network_mode declared_read_roots declared_write_roots declared_deny_roots derived_read_roots derived_write_roots stdout_path stderr_path started_at finished_at status"

func validateReport(report Report) error {
	if report.Schema != roleexec.ReportSchema {
		return fmt.Errorf("unsupported role-execution report schema %q", report.Schema)
	}
	if !executionIDPattern.MatchString(report.ExecutionID) || report.ExecutionID == "." || report.ExecutionID == ".." {
		return fmt.Errorf("role-execution report has invalid execution ID")
	}
	for name, value := range map[string]string{"workspace ID": report.WorkspaceID, "role ID": report.RoleID, "role kind": report.RoleKind} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("role-execution report %s is required", name)
		}
	}
	for name, value := range map[string]string{"manifest": report.ManifestSHA256, "policy": report.PolicySHA256, "executable": report.ExecutableSHA256} {
		if !validSHA256(value) {
			return fmt.Errorf("role-execution report %s SHA-256 is malformed", name)
		}
	}
	wantNetworkMode := "disabled"
	if report.AgentContext != nil && report.AgentContext.Broker != nil {
		wantNetworkMode = "allowlist"
	}
	if report.Backend != "macos-seatbelt" || report.Enforcement != "host-enforced" || report.Assurance != "unverified" || report.NetworkMode != wantNetworkMode || report.EnforcementScope != "filesystem, tool, and network rules" {
		return fmt.Errorf("role-execution report contains unsupported enforcement or assurance claims")
	}
	requiredLimitations := []string{"executable digest is a prelaunch byte check, not running-image identity", "Darwin runtime bootstrap and ancestor metadata are available", "no process namespace", "no host attestation", "noninteractive execution only"}
	if report.AgentContext == nil {
		requiredLimitations = append(requiredLimitations, "no control over prior role context")
	} else if report.AgentContext.Broker == nil {
		requiredLimitations = append(requiredLimitations, "Codex was requested in a fresh ephemeral CLI mode; Sentinel cannot attest provider-side model context or retention", "provider credentials are not inherited and network access is disabled; remote inference is not available in this profile")
	} else {
		requiredLimitations = append(requiredLimitations, "provider-side model context and retention are not attested", "upstream credentials are read by Sentinel and are not inherited by the Codex child", "broker counters are local request lifecycle observations and do not attest provider inference or retention", "macOS Seatbelt localhost network permission includes local host addresses at the pinned TCP port; it is not an IPv4-only grant")
	}
	for _, limitation := range requiredLimitations {
		if !contains(report.Limitations, limitation) {
			return fmt.Errorf("role-execution report omits required limitation %q", limitation)
		}
	}
	if err := validateBrokerExecution(report); err != nil {
		return err
	}
	if !filepath.IsAbs(report.ExecutablePath) || filepath.Clean(report.ExecutablePath) != report.ExecutablePath {
		return fmt.Errorf("role-execution executable path must be clean and absolute")
	}
	if len(report.AllowedTools) != len(report.AllowedToolSHA256) {
		return fmt.Errorf("role-execution allowed-tool paths and digests have different lengths")
	}
	seenTools := map[string]bool{}
	for i, tool := range report.AllowedTools {
		if !filepath.IsAbs(tool) || filepath.Clean(tool) != tool || seenTools[tool] || !validSHA256(report.AllowedToolSHA256[i]) {
			return fmt.Errorf("role-execution report has inconsistent allowed-tool identity")
		}
		seenTools[tool] = true
	}
	for _, collection := range [][]string{report.DeclaredReadRoots, report.DeclaredWriteRoots, report.DeclaredDenyRoots, report.DerivedReadRoots, report.DerivedWriteRoots} {
		for _, item := range collection {
			if !filepath.IsAbs(item) || filepath.Clean(item) != item {
				return fmt.Errorf("role-execution report contains a noncanonical capability root")
			}
		}
	}
	for _, path := range []struct{ label, value string }{{"workspace manifest", report.WorkspaceManifestPath}, {"sealed policy", report.PolicyPath}, {"stdout", report.StdoutPath}, {"stderr", report.StderrPath}} {
		if _, err := cleanRelative(path.label, path.value); err != nil {
			return err
		}
	}
	if report.StdoutPath == report.StderrPath || report.PolicyPath == report.StdoutPath || report.PolicyPath == report.StderrPath {
		return fmt.Errorf("role-execution report reuses a policy or output path")
	}
	started, err := time.Parse(time.RFC3339Nano, report.StartedAt)
	if err != nil {
		return fmt.Errorf("role-execution start time is invalid: %w", err)
	}
	finished, err := time.Parse(time.RFC3339Nano, report.FinishedAt)
	if err != nil || finished.Before(started) {
		return fmt.Errorf("role-execution finish time is invalid or precedes start")
	}
	switch report.Status {
	case "completed":
		if report.ExitCode == nil || *report.ExitCode != 0 || report.Reason != "" || !validSHA256(report.StdoutSHA256) || !validSHA256(report.StderrSHA256) {
			return fmt.Errorf("completed role execution requires exit code zero and both capture digests")
		}
	case "failed":
		if report.Reason == "" || report.ExitCode == nil && !strings.HasPrefix(report.Reason, "child process could not start:") || report.ExitCode != nil && *report.ExitCode == 0 || !validExitCode(report.ExitCode) || !validSHA256(report.StdoutSHA256) || !validSHA256(report.StderrSHA256) {
			return fmt.Errorf("failed role execution has inconsistent reason, exit code, or capture digests")
		}
	case "canceled":
		if report.Reason == "" || report.ExitCode == nil || !validExitCode(report.ExitCode) || !validSHA256(report.StdoutSHA256) || !validSHA256(report.StderrSHA256) {
			return fmt.Errorf("canceled role execution requires a reason and both capture digests")
		}
	case "indeterminate":
		if report.Reason == "" || !validExitCode(report.ExitCode) || report.StdoutSHA256 != "" && !validSHA256(report.StdoutSHA256) || report.StderrSHA256 != "" && !validSHA256(report.StderrSHA256) {
			return fmt.Errorf("indeterminate role execution has malformed reason or capture digest")
		}
	default:
		return fmt.Errorf("unsupported role-execution status %q", report.Status)
	}
	if report.Governance != nil {
		if report.Governance.Enforcement != "declaration-only" || report.Governance.Assurance != "unverified" {
			return fmt.Errorf("governance metadata contains unsupported enforcement or assurance claims")
		}
		if report.Governance.Stage != "oracle" && report.Governance.Stage != "implementation" && report.Governance.Stage != "verification" {
			return fmt.Errorf("role-execution governance stage is unsupported")
		}
		if report.Governance.Stage == "oracle" && report.RoleKind != "oracle-writer" || report.Governance.Stage == "implementation" && report.RoleKind != "implementation" || report.Governance.Stage == "verification" && report.RoleKind != "verifier" && report.RoleKind != "mutation-runner" {
			return fmt.Errorf("governance stage does not match workspace role kind")
		}
		for name, value := range map[string]string{
			"workspace manifest": report.Governance.WorkspaceManifestSHA256,
			"approval":           report.Governance.ApprovalSHA256,
			"contract source":    report.Governance.ContractSourceSHA256,
			"contract":           report.Governance.ContractSHA256,
			"oracle policy file": report.Governance.OraclePolicyFileSHA256,
			"oracle policy":      report.Governance.OraclePolicySHA256,
			"review policy":      report.Governance.ReviewPolicySHA256,
		} {
			if !validSHA256(value) {
				return fmt.Errorf("governance %s digest is malformed", name)
			}
		}
		if report.Governance.Stage == "oracle" && report.Governance.OracleSHA256 != "" || report.Governance.Stage != "oracle" && !validSHA256(report.Governance.OracleSHA256) {
			return fmt.Errorf("governance frozen oracle digest is missing or malformed")
		}
		if report.Governance.WorkspaceID != report.WorkspaceID || report.Governance.WorkspaceManifestSHA256 != report.ManifestSHA256 {
			return fmt.Errorf("governance metadata does not match role-execution workspace identity")
		}
		if strings.TrimSpace(report.Governance.ApprovalRecordID) == "" {
			return fmt.Errorf("governance approval record ID is required")
		}
	}
	return nil
}

func validateBrokerExecution(report Report) error {
	var broker *agentlaunch.BrokerContext
	if report.AgentContext != nil {
		broker = report.AgentContext.Broker
	}
	if broker == nil {
		if report.BrokerExecution != nil {
			return fmt.Errorf("role-execution report has broker statistics without a broker agent profile")
		}
		return nil
	}
	execution := report.BrokerExecution
	if execution == nil {
		return fmt.Errorf("broker agent profile requires a broker execution snapshot")
	}
	_, err := validateBrokerEndpoint(broker.Endpoint)
	if err != nil || broker.Provider != "openai-responses" || broker.CredentialSource != "environment" || !brokerCredentialNamePattern.MatchString(broker.CredentialEnv) || broker.CredentialEnv == "INGEN_CODEX_BROKER_TOKEN" || broker.MaxRequests < 1 || broker.MaxRequests > 64 || broker.MaxOutputTokens < 1 || broker.MaxOutputTokens > 8192 || broker.TimeoutSeconds < 1 || broker.TimeoutSeconds > 1800 {
		return fmt.Errorf("Codex broker context has malformed provider, local endpoint, credential reference, or bounded limits")
	}
	if strings.TrimSpace(report.AgentContext.Model) == "" || execution.Provider != broker.Provider || execution.Endpoint != broker.Endpoint || execution.Model != report.AgentContext.Model || execution.MaxRequests != broker.MaxRequests || execution.MaxOutputTokens != broker.MaxOutputTokens || execution.TimeoutSeconds != broker.TimeoutSeconds {
		return fmt.Errorf("broker execution identity, model, or limits do not match the typed-agent context")
	}
	stats := execution.Stats
	if stats.RequestsReceived < 0 || stats.RequestsForwarded < 0 || stats.RequestsRejected < 0 || stats.UpstreamFailures < 0 || stats.InFlight < 0 || stats.InFlight > 1 || stats.LastStatus < 0 || stats.LastStatus > 599 || stats.LastStatus > 0 && stats.LastStatus < 100 || stats.RequestsForwarded > int64(broker.MaxRequests) || stats.RequestsForwarded > stats.RequestsReceived || stats.RequestsRejected > stats.RequestsReceived || stats.UpstreamFailures > stats.RequestsForwarded || stats.InFlight > stats.RequestsReceived || stats.InFlight > stats.RequestsForwarded {
		return fmt.Errorf("broker execution statistics violate request, failure, or configured budget bounds")
	}
	reportStarted, startErr := time.Parse(time.RFC3339Nano, report.StartedAt)
	reportFinished, finishErr := time.Parse(time.RFC3339Nano, report.FinishedAt)
	if startErr != nil || finishErr != nil || reportFinished.Before(reportStarted) {
		return fmt.Errorf("broker statistics cannot be linked to malformed role-execution times")
	}
	parseStatTime := func(label, value string, required bool) (time.Time, error) {
		if value == "" {
			if required {
				return time.Time{}, fmt.Errorf("broker %s timestamp is required", label)
			}
			return time.Time{}, nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil || parsed.Before(reportStarted) || parsed.After(reportFinished) {
			return time.Time{}, fmt.Errorf("broker %s timestamp is malformed or outside role execution", label)
		}
		return parsed, nil
	}
	if execution.Status == "unavailable" {
		if stats != (roleexec.BrokerStats{}) {
			return fmt.Errorf("unavailable broker execution must not claim server times or request statistics")
		}
		if (report.Status != "failed" && report.Status != "indeterminate") || report.ExitCode != nil {
			return fmt.Errorf("an unavailable broker cannot accompany a launched role outcome")
		}
		return nil
	}
	if execution.Status != "closed" && execution.Status != "indeterminate" {
		return fmt.Errorf("broker execution has unsupported lifecycle status %q", execution.Status)
	}
	if report.Status == "completed" && execution.Status != "closed" {
		return fmt.Errorf("completed broker-backed execution requires a closed broker lifecycle")
	}
	started, err := parseStatTime("start", stats.StartedAt, execution.Status == "closed" || stats.StartedAt != "")
	if err != nil {
		return err
	}
	closed, err := parseStatTime("close", stats.ClosedAt, execution.Status == "closed")
	if err != nil {
		return err
	}
	lastCompleted, err := parseStatTime("last completion", stats.LastCompletedAt, stats.RequestsForwarded > 0 && execution.Status == "closed")
	if err != nil {
		return err
	}
	if !started.IsZero() && !closed.IsZero() && closed.Before(started) || !lastCompleted.IsZero() && !started.IsZero() && lastCompleted.Before(started) || !lastCompleted.IsZero() && !closed.IsZero() && lastCompleted.After(closed) {
		return fmt.Errorf("broker execution statistics have inconsistent lifecycle timestamps")
	}
	if execution.Status == "closed" {
		if stats.InFlight != 0 || stats.RequestsReceived-stats.RequestsForwarded < stats.RequestsRejected {
			return fmt.Errorf("closed broker statistics have impossible request counters or unsettled forwarded work")
		}
		if stats.RequestsForwarded == 0 && (stats.LastStatus != 0 || stats.LastCompletedAt != "") {
			return fmt.Errorf("broker statistics claim an upstream response without a forwarded request")
		}
	}
	if stats.RequestsForwarded == 0 && (stats.LastStatus != 0 || stats.LastCompletedAt != "") {
		return fmt.Errorf("broker statistics claim an upstream result without a forwarded request")
	}
	return nil
}

func validateBrokerEndpoint(value string) (int, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Path != "/v1" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return 0, fmt.Errorf("broker endpoint must be exact local HTTP /v1 URL")
	}
	portText := parsed.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != portText || value != fmt.Sprintf("http://127.0.0.1:%d/v1", port) {
		return 0, fmt.Errorf("broker endpoint port is malformed")
	}
	return port, nil
}

func verifyAgentContext(rooted *os.Root, root string, report Report, addContents func(string, string, []byte, string) error) error {
	context := report.AgentContext
	wantNetworkMode := "disabled"
	if context.Broker != nil {
		wantNetworkMode = "allowlist"
	}
	if context.Schema != agentlaunch.ContextSchema || context.Agent != "codex-cli" || context.Mode != "exec-ephemeral" || context.ExecutionID != report.ExecutionID || context.Root != root || context.NetworkMode != wantNetworkMode || context.Stdin != agentlaunch.StdinPromptSource {
		return fmt.Errorf("Codex agent context does not match the local role execution")
	}
	runtimeRoot := filepath.Join(root, ".ingen", "artifacts", "role-executions", ".runtime", report.ExecutionID)
	expectedSnapshot := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", report.ExecutionID+".prompt"))
	if context.WorkingDirectory != runtimeRoot || context.HomePath != filepath.Join(runtimeRoot, "rw", "home") || context.CodexHomePath != filepath.Join(runtimeRoot, "rw", "codex-home") || context.PromptSnapshotPath != expectedSnapshot {
		return fmt.Errorf("Codex agent context private paths do not match execution identity")
	}
	if context.PromptBytes < 1 || context.PromptBytes > agentlaunch.MaxPromptBytes || !validSHA256(context.PromptSourceSHA256) || context.PromptSourceSHA256 != context.PromptSnapshotSHA256 || report.ExecutablePath == "" {
		return fmt.Errorf("Codex agent context prompt or executable identity is malformed")
	}
	sourcePath, err := cleanRelative("Codex prompt source", context.PromptSourcePath)
	if err != nil {
		return err
	}
	sourceAbs := filepath.Join(root, filepath.FromSlash(sourcePath))
	if !execution.PathCovered(report.DeclaredReadRoots, sourceAbs) {
		return fmt.Errorf("Codex prompt source is outside the explicitly declared role read roots")
	}
	for _, denied := range report.DeclaredDenyRoots {
		if execution.PathCovered([]string{denied}, sourceAbs) {
			return fmt.Errorf("Codex prompt source is beneath a declared deny root")
		}
	}
	for _, writable := range report.DeclaredWriteRoots {
		if execution.PathCovered([]string{writable}, sourceAbs) {
			return fmt.Errorf("Codex prompt source is beneath a declared write root")
		}
	}
	args, err := expectedCodexArgs(context)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(args, context.Args) {
		return fmt.Errorf("Codex invocation arguments do not match the fixed Sentinel launch profile")
	}
	if err := rejectSymlinkComponents(root, sourcePath); err != nil {
		return fmt.Errorf("Codex prompt source path contains a symlink: %w", err)
	}
	sourceBytes, err := readRooted(rooted, sourcePath)
	if err != nil {
		return fmt.Errorf("read Codex prompt source: %w", err)
	}
	if err := agentlaunch.ValidatePrompt(sourceBytes); err != nil || len(sourceBytes) != context.PromptBytes || digest(sourceBytes) != context.PromptSourceSHA256 {
		return fmt.Errorf("Codex prompt source bytes do not match the recorded bounded digest")
	}
	if err := addContents(FileAgentPromptSource, sourcePath, sourceBytes, context.PromptSourceSHA256); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(root, context.PromptSnapshotPath); err != nil {
		return fmt.Errorf("Codex prompt snapshot path contains a symlink: %w", err)
	}
	snapshotBytes, err := readRooted(rooted, context.PromptSnapshotPath)
	if err != nil {
		return fmt.Errorf("read Codex prompt snapshot: %w", err)
	}
	if err := agentlaunch.ValidatePrompt(snapshotBytes); err != nil || len(snapshotBytes) != context.PromptBytes || !bytes.Equal(sourceBytes, snapshotBytes) || digest(snapshotBytes) != context.PromptSnapshotSHA256 {
		return fmt.Errorf("Codex prompt snapshot bytes do not match the recorded exact source")
	}
	return addContents(FileAgentPrompt, context.PromptSnapshotPath, snapshotBytes, context.PromptSnapshotSHA256)
}

func expectedCodexArgs(context *agentlaunch.CodexContext) ([]string, error) {
	_, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: context.ExecutionID, Root: context.Root, WorkingDir: context.WorkingDirectory,
		HomePath: context.HomePath, CodexHome: context.CodexHomePath,
		PromptSourcePath: context.PromptSourcePath, PromptSourceSHA256: context.PromptSourceSHA256,
		PromptSnapshotPath: context.PromptSnapshotPath, PromptSnapshotSHA256: context.PromptSnapshotSHA256,
		PromptBytes: context.PromptBytes, Model: context.Model, Broker: context.Broker,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid fixed Codex context: %w", err)
	}
	return args, nil
}

func validExitCode(code *int) bool { return code == nil || *code >= -1 && *code <= 255 }

func verifyWorkspaceAndPolicy(rooted *os.Root, root string, verified *Verified, addContents func(string, string, []byte, string) error) error {
	report := verified.Report
	for label, path := range map[string]string{"workspace manifest": report.WorkspaceManifestPath, "sealed policy": report.PolicyPath} {
		if err := rejectSymlinkComponents(root, path); err != nil {
			return fmt.Errorf("%s path contains a symbolic link: %w", label, err)
		}
	}
	manifestBytes, err := readRooted(rooted, report.WorkspaceManifestPath)
	if err != nil {
		return fmt.Errorf("read workspace manifest: %w", err)
	}
	if digest(manifestBytes) != report.ManifestSHA256 {
		return fmt.Errorf("workspace manifest digest mismatch")
	}
	if err := addContents(FileWorkspaceManifest, report.WorkspaceManifestPath, manifestBytes, report.ManifestSHA256); err != nil {
		return err
	}
	manifest, err := workspace.LoadBytes(report.WorkspaceManifestPath, manifestBytes)
	if err != nil {
		return fmt.Errorf("validate referenced workspace manifest: %w", err)
	}
	if manifest.ID != report.WorkspaceID {
		return fmt.Errorf("role-execution report workspace ID does not match workspace manifest")
	}
	var role *workspace.Role
	for index := range manifest.Roles {
		if manifest.Roles[index].ID == report.RoleID {
			role = &manifest.Roles[index]
			break
		}
	}
	if role == nil || role.Kind != report.RoleKind {
		return fmt.Errorf("role-execution role identity does not match workspace manifest")
	}
	for _, capability := range []struct {
		name  string
		paths []string
	}{{"read", role.ReadRoots}, {"write", role.WriteRoots}, {"deny", role.DenyRoots}} {
		for _, path := range capability.paths {
			clean, err := cleanRelative("role "+capability.name+" capability", path)
			if err != nil || clean == "." {
				return fmt.Errorf("workspace role has unsafe %s capability root %q", capability.name, path)
			}
			if err := rejectSymlinkComponents(root, clean); err != nil {
				return fmt.Errorf("workspace role capability contains a symbolic link: %w", err)
			}
		}
	}
	if err := rejectSymlinkComponents(root, role.Workspace); err != nil {
		return fmt.Errorf("role workspace path contains a symbolic link: %w", err)
	}
	if !sameAbsoluteRoots(root, role.ReadRoots, report.DeclaredReadRoots) || !sameAbsoluteRoots(root, role.WriteRoots, report.DeclaredWriteRoots) || !sameAbsoluteRoots(root, role.DenyRoots, report.DeclaredDenyRoots) {
		return fmt.Errorf("role-execution declared capabilities do not match workspace manifest")
	}
	scratchRelative := filepath.Join(role.Workspace, ".sentinel-roleexec", report.ExecutionID)
	if report.AgentContext != nil {
		scratchRelative = filepath.Join(".ingen", "artifacts", "role-executions", ".runtime", report.ExecutionID)
	}
	scratch := filepath.Join(root, scratchRelative)
	if err := rejectSymlinkComponents(root, filepath.ToSlash(scratchRelative)); err != nil {
		return fmt.Errorf("role scratch path contains a symbolic link: %w", err)
	}
	if !equalPathLists(report.DerivedReadRoots, []string{scratch}) || !equalPathLists(report.DerivedWriteRoots, []string{filepath.Join(scratch, "rw")}) {
		return fmt.Errorf("role-execution private scratch roots do not match role identity")
	}
	policyBytes, err := readRooted(rooted, report.PolicyPath)
	if err != nil {
		return fmt.Errorf("read sealed role policy: %w", err)
	}
	if digest(policyBytes) != report.PolicySHA256 {
		return fmt.Errorf("sealed role policy digest mismatch")
	}
	if err := addContents(FileSealedPolicy, report.PolicyPath, policyBytes, report.PolicySHA256); err != nil {
		return err
	}
	if err := rejectDuplicateJSONKeys(policyBytes); err != nil {
		return fmt.Errorf("strictly decode sealed Sorna policy: %w", err)
	}
	document, err := policy.LoadBytes(report.PolicyPath, policyBytes)
	if err != nil {
		return fmt.Errorf("validate sealed Sorna policy: %w", err)
	}
	status, _ := document.Policy["status"].(string)
	if status != "sealed" {
		return fmt.Errorf("referenced role policy is not sealed")
	}
	canonical, err := policy.CanonicalJSON(document)
	if err != nil {
		return fmt.Errorf("canonicalize sealed Sorna policy: %w", err)
	}
	if !bytes.Equal(canonical, policyBytes) || digest(canonical) != report.PolicySHA256 {
		return fmt.Errorf("sealed Sorna policy is not canonical or its digest does not match the report")
	}
	if err := comparePolicyIdentity(document.Policy, root, manifest.ID, *role, report); err != nil {
		return err
	}
	if report.Governance != nil {
		for label, path := range map[string]string{"contract source": manifest.Contract.Path, "oracle policy": manifest.Sorna.OraclePolicy} {
			if err := rejectSymlinkComponents(root, path); err != nil {
				return fmt.Errorf("governed %s path contains a symbolic link: %w", label, err)
			}
		}
		contractBytes, err := readRooted(rooted, manifest.Contract.Path)
		if err != nil {
			return fmt.Errorf("read governed contract source: %w", err)
		}
		if digest(contractBytes) != report.Governance.ContractSourceSHA256 {
			return fmt.Errorf("governed contract source digest mismatch")
		}
		if err := addContents(FileContractSource, manifest.Contract.Path, contractBytes, report.Governance.ContractSourceSHA256); err != nil {
			return err
		}
		contractDocument, err := contract.LoadBytes(manifest.Contract.Path, contractBytes)
		if err != nil {
			return fmt.Errorf("validate governed contract source: %w", err)
		}
		contractStatus, _ := contractDocument.Contract["status"].(string)
		var contractSealed contract.Sealed
		if contractStatus == "sealed" {
			canonical, canonicalErr := contract.CanonicalJSON(contractDocument)
			if canonicalErr != nil {
				return fmt.Errorf("canonicalize governed sealed contract source: %w", canonicalErr)
			}
			contractSealed = contract.Sealed{Document: contractDocument, CanonicalJSON: canonical, SHA256: digest(canonical)}
		} else {
			contractSealed, err = contract.SealWithFixtureReader(contractDocument, func(fixture string) ([]byte, error) {
				if filepath.IsAbs(fixture) || strings.ContainsRune(fixture, '\\') {
					return nil, fmt.Errorf("contract fixture path must be project-relative")
				}
				fixturePath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(manifest.Contract.Path), filepath.FromSlash(fixture))))
				cleanFixture, err := cleanRelative("contract fixture", fixturePath)
				if err != nil {
					return nil, err
				}
				if err := rejectSymlinkComponents(root, cleanFixture); err != nil {
					return nil, err
				}
				contents, err := readRooted(rooted, cleanFixture)
				if err != nil {
					return nil, err
				}
				if err := addContents(FileContractFixture, cleanFixture, contents, ""); err != nil {
					return nil, err
				}
				return contents, nil
			})
			if err != nil {
				return fmt.Errorf("seal governed contract source: %w", err)
			}
		}
		if contractSealed.SHA256 != report.Governance.ContractSHA256 {
			return fmt.Errorf("governed contract does not seal to recorded canonical digest")
		}
		if contractSealed.Document.Contract["id"] != report.Governance.ContractID {
			return fmt.Errorf("governed contract ID does not match recorded identity")
		}
		if !equalInteger(contractSealed.Document.Contract["version"], report.Governance.ContractVersion) {
			return fmt.Errorf("governed contract version does not match recorded identity")
		}
		oraclePolicyBytes, err := readRooted(rooted, manifest.Sorna.OraclePolicy)
		if err != nil {
			return fmt.Errorf("read governed oracle policy: %w", err)
		}
		if digest(oraclePolicyBytes) != report.Governance.OraclePolicyFileSHA256 {
			return fmt.Errorf("governed oracle policy file digest mismatch")
		}
		if err := addContents(FileOraclePolicy, manifest.Sorna.OraclePolicy, oraclePolicyBytes, report.Governance.OraclePolicyFileSHA256); err != nil {
			return err
		}
		oraclePolicyDocument, err := policy.LoadBytes(manifest.Sorna.OraclePolicy, oraclePolicyBytes)
		if err != nil {
			return fmt.Errorf("validate governed oracle policy: %w", err)
		}
		sealedOraclePolicy, err := policy.Seal(oraclePolicyDocument)
		if err != nil || sealedOraclePolicy.SHA256 != report.Governance.OraclePolicySHA256 {
			return fmt.Errorf("governed oracle policy does not seal to recorded digest")
		}
		if report.Governance.WorkspaceVersion != manifest.Version {
			return fmt.Errorf("governance workspace version does not match manifest")
		}
	}
	return nil
}

func comparePolicyIdentity(document map[string]any, root, workspaceID string, role workspace.Role, report Report) error {
	if getString(document, "id") != "sentinel-role-"+report.ExecutionID || getString(document, "purpose") != "noninteractive Sentinel role execution" || getString(document, "enforcement") != "host-enforced" {
		return fmt.Errorf("sealed policy identity does not match role-execution report")
	}
	filesystem, ok := document["filesystem"].(map[string]any)
	if !ok {
		return fmt.Errorf("sealed policy filesystem block is missing")
	}
	policyReads, err := policyPathList(filesystem, "read")
	if err != nil {
		return err
	}
	policyWrites, err := policyPathList(filesystem, "write")
	if err != nil {
		return err
	}
	policyDenies, err := policyPathList(filesystem, "deny")
	if err != nil {
		return err
	}
	wantReads := append(absoluteRoots(root, role.ReadRoots), report.DerivedReadRoots...)
	wantWrites := append(absoluteRoots(root, role.WriteRoots), report.DerivedWriteRoots...)
	wantDenies := absoluteRoots(root, role.DenyRoots)
	if !equalPathLists(policyReads, wantReads) || !equalPathLists(policyWrites, wantWrites) || !equalPathLists(policyDenies, wantDenies) {
		return fmt.Errorf("sealed policy filesystem roots do not match workspace capabilities and private scratch")
	}
	network, ok := document["network"].(map[string]any)
	if !ok {
		return fmt.Errorf("sealed role policy network block is missing")
	}
	if report.AgentContext != nil && report.AgentContext.Broker != nil {
		broker := report.AgentContext.Broker
		port, err := validateBrokerEndpoint(broker.Endpoint)
		if err != nil {
			return fmt.Errorf("sealed role policy broker endpoint is malformed: %w", err)
		}
		allow, valid := network["allow"].([]any)
		if getString(network, "mode") != "allowlist" || len(network) != 2 || len(allow) != 1 {
			return fmt.Errorf("broker role policy must allow only its loopback broker endpoint")
		}
		entry, valid := allow[0].(map[string]any)
		ports, portsValid := entry["ports"].([]any)
		if !valid || len(entry) != 4 || getString(entry, "host") != "localhost" || getString(entry, "direction") != "outbound" || getString(entry, "purpose") != "local Codex Responses broker" || !portsValid || len(ports) != 1 || !equalInteger(ports[0], int64(port)) {
			return fmt.Errorf("broker role policy has an extra or mismatched network permission")
		}
	} else if getString(network, "mode") != "disabled" || len(network) != 1 {
		return fmt.Errorf("offline role policy must disable network access without exceptions")
	}
	process, ok := document["process"].(map[string]any)
	if !ok || getString(process, "subject_id") != workspaceID+"/"+role.ID || process["can_invoke_subject"] != false {
		return fmt.Errorf("sealed role policy process identity does not match workspace role")
	}
	allowed, ok := process["allowed_tools"].([]any)
	if !ok || len(allowed) != len(report.AllowedTools) {
		return fmt.Errorf("sealed role policy allowed tools do not match report")
	}
	for i, item := range allowed {
		entry, ok := item.(map[string]any)
		if !ok || getString(entry, "name") != report.AllowedTools[i] {
			return fmt.Errorf("sealed role policy allowed tools do not match report")
		}
	}
	return nil
}

func verifyGovernance(verified *Verified, addFile func(string, string, string, bool) error) error {
	gate := verified.Report.Governance
	if gate == nil {
		return nil
	}
	if len(gate.ApprovalArtifacts) == 0 {
		return fmt.Errorf("governance report omits approval artifact references")
	}
	artifacts := make(map[string]governance.VerifiedArtifact, len(gate.ApprovalArtifacts))
	for _, artifact := range gate.ApprovalArtifacts {
		if !validSHA256(artifact.SHA256) || strings.TrimSpace(artifact.Kind) == "" {
			return fmt.Errorf("governance approval artifact has malformed identity")
		}
		if _, duplicate := artifacts[artifact.Kind]; duplicate {
			return fmt.Errorf("governance approval artifact kind %q is duplicated", artifact.Kind)
		}
		artifacts[artifact.Kind] = artifact
		if err := addFile("governance-"+artifact.Kind, artifact.Path, artifact.SHA256, false); err != nil {
			return err
		}
	}
	approval, ok := artifacts["hammond-approval"]
	if !ok || approval.SHA256 != gate.ApprovalSHA256 {
		return fmt.Errorf("governance approval record artifact does not match recorded digest")
	}
	approvedContract, ok := artifacts["sorna-contract"]
	if !ok || approvedContract.SHA256 != gate.ContractSHA256 {
		return fmt.Errorf("approved canonical contract artifact does not match recorded digest")
	}
	reviewPolicy, ok := artifacts["hammond-review-policy"]
	if !ok || reviewPolicy.SHA256 != gate.ReviewPolicySHA256 {
		return fmt.Errorf("approved review policy artifact does not match recorded digest")
	}
	if active, ok := artifacts["active-hammond-review-policy"]; ok && active.SHA256 != gate.ReviewPolicySHA256 {
		return fmt.Errorf("active review policy artifact does not match recorded digest")
	}
	contractFile, ok := findFile(verified.Files, "governance-sorna-contract", approvedContract.Path)
	if !ok {
		return fmt.Errorf("approved canonical contract bytes were not retained")
	}
	if contractFile.SHA256 != gate.ContractSHA256 {
		return fmt.Errorf("approved canonical contract bytes do not match the sealed source identity")
	}
	activePolicy := reviewPolicy.Path
	if active, ok := artifacts["active-hammond-review-policy"]; ok {
		activePolicy = active.Path
	}
	approved, err := governance.VerifyApproved(verified.root, approval.Path, activePolicy, governance.ContractReference{
		ProjectID: gate.WorkspaceID, ID: gate.ContractID, Version: int(gate.ContractVersion), Schema: contract.Schema,
		Artifact: governance.Artifact{URI: approvedContract.Path, SHA256: gate.ContractSHA256},
	})
	if err != nil {
		return fmt.Errorf("verify governed approval snapshot: %w", err)
	}
	if approved.Record.RecordID != gate.ApprovalRecordID {
		return fmt.Errorf("governance approval record ID does not match verified Hammond record")
	}
	if !sameGovernanceArtifacts(gate.ApprovalArtifacts, approved.Artifacts) {
		return fmt.Errorf("governance artifact identities differ from verified Hammond snapshot")
	}
	return nil
}

func getString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}
func equalInteger(value any, expected int64) bool {
	switch number := value.(type) {
	case json.Number:
		actual, err := number.Int64()
		return err == nil && actual == expected
	case float64:
		return int64(number) == expected && float64(int64(number)) == number
	case int:
		return int64(number) == expected
	case int64:
		return number == expected
	default:
		return false
	}
}

func findFile(files []File, kind, path string) (File, bool) {
	for _, file := range files {
		if file.Kind == kind && file.Path == path {
			return file, true
		}
	}
	return File{}, false
}

func sameFiles(a, b []File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].Path != b[i].Path || a[i].SHA256 != b[i].SHA256 || !bytes.Equal(a[i].Bytes, b[i].Bytes) {
			return false
		}
	}
	return true
}

func sameGovernanceArtifacts(a, b []governance.VerifiedArtifact) bool {
	key := func(items []governance.VerifiedArtifact) []string {
		result := make([]string, len(items))
		for i, item := range items {
			result[i] = item.Kind + "\x00" + item.Path + "\x00" + item.SHA256
		}
		sort.Strings(result)
		return result
	}
	return reflect.DeepEqual(key(a), key(b))
}

func readRooted(rooted *os.Root, path string) ([]byte, error) {
	file, err := openEvidenceFile(rooted, filepath.ToSlash(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("referenced evidence is not a regular file")
	}
	if info.Size() > maxEvidenceFileBytes {
		return nil, fmt.Errorf("referenced evidence exceeds %d bytes", maxEvidenceFileBytes)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxEvidenceFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxEvidenceFileBytes {
		return nil, fmt.Errorf("referenced evidence exceeds %d bytes", maxEvidenceFileBytes)
	}
	return contents, nil
}

func rejectSymlinkComponents(root, relative string) error {
	current := root
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("component %q is a symbolic link", current)
		}
	}
	return nil
}

func cleanRelative(label, raw string) (string, error) {
	if raw == "" || filepath.IsAbs(raw) || strings.ContainsRune(raw, '\\') {
		return "", fmt.Errorf("%s path must be project-relative", label)
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != raw {
		return "", fmt.Errorf("%s path must be normalized and stay beneath project root", label)
	}
	return filepath.ToSlash(clean), nil
}

func sameAbsoluteRoots(root string, relative, absolute []string) bool {
	want := absoluteRoots(root, relative)
	return equalPathLists(want, absolute)
}

func absoluteRoots(root string, paths []string) []string {
	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = filepath.Join(root, filepath.FromSlash(path))
	}
	return result
}

func equalPathLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if filepath.Clean(a[i]) != filepath.Clean(b[i]) {
			return false
		}
	}
	return true
}

func policyPathList(filesystem map[string]any, name string) ([]string, error) {
	entries, ok := filesystem[name].([]any)
	if !ok {
		return nil, fmt.Errorf("sealed policy filesystem.%s is malformed", name)
	}
	paths := make([]string, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("sealed policy filesystem.%s[%d] is malformed", name, i)
		}
		path := getString(entry, "path")
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, fmt.Errorf("sealed policy filesystem.%s[%d].path is not canonical", name, i)
		}
		paths[i] = path
	}
	return paths, nil
}

func ciStatus(report Report) (string, int, string) {
	switch report.Status {
	case "completed":
		return "passed", 0, ""
	case "failed":
		return "failed", 1, ""
	case "canceled":
		reason := report.Reason
		if reason == "" {
			reason = "role execution was canceled"
		}
		return "error", 2, reason
	default:
		reason := report.Reason
		if reason == "" {
			reason = "role execution outcome is indeterminate"
		}
		return "error", 2, reason
	}
}

func decodeStrict(contents []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func requireJSONFields(contents []byte, fields string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(contents, &object); err != nil {
		return err
	}
	for _, field := range strings.Fields(fields) {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("missing required field %q", field)
		}
	}
	return nil
}

func requireNonNullJSONFields(contents []byte, fields string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(contents, &object); err != nil {
		return err
	}
	for _, field := range strings.Fields(fields) {
		value, ok := object[field]
		if !ok {
			return fmt.Errorf("missing required field %q", field)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("required field %q cannot be null", field)
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
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
		seen := make(map[string]bool)
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
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = true
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil || closeToken != json.Delim('}') {
			return fmt.Errorf("malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil || closeToken != json.Delim(']') {
			return fmt.Errorf("malformed JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func validSHA256(value string) bool { return sha256Pattern.MatchString(value) }
func digest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
