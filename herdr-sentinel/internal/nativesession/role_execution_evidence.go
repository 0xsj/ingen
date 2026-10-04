package nativesession

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/evidence"
	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/roleexec"
	"ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workflowgate"
)

type roleExecutionBinding struct {
	Root, WorkspacePath, ReceiptPath, RoleID, ExecutionID, PolicyPath            string
	ManifestSHA256, PolicySHA256, ExecutableSHA256                               string
	Governed                                                                     bool
	ApprovalPath, ReviewPolicyPath, OraclePath                                   string
	ApprovalSHA256, ReviewPolicySHA256, ContractSHA256                           string
	ContractSourceSHA256, OraclePolicySHA256, OraclePolicyFileSHA256             string
	OracleSHA256                                                                 string
	AllowedTools, AllowedToolSHA256, ProtectedPaths                              []string
	Agent, AgentExecutable, PromptPath, PromptSHA256, AgentModel                 string
	AgentProvider, AgentCredentialEnv                                            string
	AgentBrokerPort, AgentMaxRequests, AgentMaxOutputTokens, AgentTimeoutSeconds int
	AgentBrokerProfile                                                           bool
}

type roleEvidenceArtifact struct {
	ID, Kind, Path, SHA256 string
}

var brokerCredentialEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// verifyRoleExecutionEvidence only recognizes the exact Sentinel executable
// followed by the role-execute subcommand. Other native commands retain their
// original collection behavior.
func verifyRoleExecutionEvidence(root string, receipt *run.Receipt, record Record, terminal *nativejournal.Event) ([]roleEvidenceArtifact, error) {
	if terminal == nil || terminal.Kind != nativejournal.KindWrapperTerminal {
		return nil, nil
	}
	binding, recognized, err := parseRoleExecutionIntent(record.Intent)
	if err != nil || !recognized {
		return nil, err
	}
	if binding.Root != root || binding.WorkspacePath != receipt.Workspace.File.Path || binding.ReceiptPath != record.Intent.ReceiptPath || binding.RoleID != record.Intent.RoleID || binding.ManifestSHA256 != record.Intent.WorkspaceManifestSHA256 {
		return nil, fmt.Errorf("native role-execute argv does not match the committed native session and receipt identity")
	}
	if binding.ExecutionID == record.Intent.SessionID || binding.ExecutionID == "" {
		return nil, fmt.Errorf("native role-execute argv has an invalid or reused execution ID")
	}
	wantPolicyPath := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", binding.ExecutionID+".policy.json"))
	if binding.PolicyPath != wantPolicyPath {
		return nil, fmt.Errorf("native role-execute argv policy path does not match its execution ID")
	}
	wantReportPath := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", binding.ExecutionID+".json"))
	reportArtifactID := roleEvidenceID(record.Intent.SessionID, "report")
	priorReportSHA := receiptArtifactSHA(receipt, reportArtifactID)
	verified, err := evidence.Verify(root, wantReportPath, priorReportSHA)
	if err != nil {
		return nil, fmt.Errorf("verify contained Sentinel role-execution evidence: %w", err)
	}
	report := verified.Report
	if report.ExecutionID != binding.ExecutionID || report.WorkspaceID != record.Intent.WorkspaceID || report.RoleID != record.Intent.RoleID || report.RoleKind != record.Intent.RoleKind {
		return nil, fmt.Errorf("contained role-execution report identity does not match native intent and argv")
	}
	if report.WorkspaceManifestPath != binding.WorkspacePath || report.ManifestSHA256 != binding.ManifestSHA256 || report.PolicyPath != binding.PolicyPath || report.PolicySHA256 != binding.PolicySHA256 || report.ExecutableSHA256 != binding.ExecutableSHA256 {
		return nil, fmt.Errorf("contained role-execution report does not match the exact manifest, policy, or executable pins in argv")
	}
	wantStdout := filepath.ToSlash(filepath.Join(filepath.Dir(wantReportPath), binding.ExecutionID+".stdout"))
	wantStderr := filepath.ToSlash(filepath.Join(filepath.Dir(wantReportPath), binding.ExecutionID+".stderr"))
	if report.StdoutPath != wantStdout || report.StderrPath != wantStderr {
		return nil, fmt.Errorf("contained role-execution report output paths do not match its execution ID")
	}
	if record.State == nativejournal.StateCompleted && (report.Status != "completed" || report.ExitCode == nil || *report.ExitCode != 0) {
		return nil, fmt.Errorf("successful native wrapper requires a completed role report with contained exit code zero")
	}
	if !sameIDs(report.AllowedTools, binding.AllowedTools) || !sameIDs(report.AllowedToolSHA256, binding.AllowedToolSHA256) {
		return nil, fmt.Errorf("contained role-execution report allowed-tool pins do not match argv")
	}
	if err := verifyRoleAgentContext(root, binding, report); err != nil {
		return nil, err
	}
	if err := verifyRoleGovernance(root, record, binding, report.Governance); err != nil {
		return nil, err
	}
	files, err := roleExecutionFiles(verified, record.Intent.SessionID, receipt.Workspace.File)
	if err != nil {
		return nil, err
	}
	for _, item := range files {
		if current := receiptArtifactSHA(receipt, item.ID); current != "" && current != item.SHA256 {
			return nil, fmt.Errorf("collected role-execution evidence %q changed since it was attached", item.Path)
		}
	}
	return files, nil
}

func parseRoleExecutionIntent(intent nativejournal.Intent) (roleExecutionBinding, bool, error) {
	if len(intent.Argv) < 3 || intent.Argv[0] != intent.SentinelExecutable || intent.Argv[1] != "role" || intent.Argv[2] != "execute" {
		return roleExecutionBinding{}, false, nil
	}
	args := intent.Argv[3:]
	delimiter := -1
	for i, arg := range args {
		if arg == "--" {
			delimiter = i
			break
		}
	}
	hasDelimiter := delimiter >= 0
	if delimiter < 0 {
		delimiter = len(args)
	}
	binding := roleExecutionBinding{AllowedTools: []string{}, AllowedToolSHA256: []string{}, ProtectedPaths: []string{}}
	brokerPortText, brokerMaxRequestsText, brokerMaxOutputTokensText, brokerTimeoutSecondsText := "", "", "", ""
	values := map[string]*string{
		"--root": &binding.Root, "--workspace": &binding.WorkspacePath, "--receipt": &binding.ReceiptPath,
		"--role": &binding.RoleID, "--execution-id": &binding.ExecutionID, "--policy-path": &binding.PolicyPath,
		"--expected-manifest-sha256": &binding.ManifestSHA256, "--expected-policy-sha256": &binding.PolicySHA256,
		"--expected-executable-sha256": &binding.ExecutableSHA256, "--approval": &binding.ApprovalPath,
		"--review-policy": &binding.ReviewPolicyPath, "--oracle": &binding.OraclePath,
		"--expected-approval-sha256": &binding.ApprovalSHA256, "--expected-review-policy-sha256": &binding.ReviewPolicySHA256,
		"--expected-contract-sha256": &binding.ContractSHA256, "--expected-contract-source-sha256": &binding.ContractSourceSHA256,
		"--expected-oracle-policy-sha256": &binding.OraclePolicySHA256, "--expected-oracle-policy-file-sha256": &binding.OraclePolicyFileSHA256,
		"--expected-oracle-sha256": &binding.OracleSHA256,
		"--agent":                  &binding.Agent, "--agent-executable": &binding.AgentExecutable,
		"--prompt-file": &binding.PromptPath, "--expected-prompt-sha256": &binding.PromptSHA256,
		"--agent-model":    &binding.AgentModel,
		"--agent-provider": &binding.AgentProvider, "--agent-credential-env": &binding.AgentCredentialEnv,
		"--agent-broker-port": &brokerPortText, "--agent-max-requests": &brokerMaxRequestsText,
		"--agent-max-output-tokens": &brokerMaxOutputTokensText, "--agent-timeout-seconds": &brokerTimeoutSecondsText,
	}
	seen := make(map[string]bool)
	for i := 0; i < delimiter; i++ {
		name := args[i]
		switch name {
		case "--governed":
			if seen[name] {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv repeats --governed")
			}
			seen[name] = true
			binding.Governed = true
		case "--protect-path":
			if i+1 >= delimiter || args[i+1] == "" {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv has an empty --protect-path")
			}
			value := args[i+1]
			if !normalizedRolePath(value) || contains(binding.ProtectedPaths, value) {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv has an invalid or duplicate protected path %q", value)
			}
			binding.ProtectedPaths = append(binding.ProtectedPaths, value)
			i++
		case "--allow-tool":
			if i+2 >= delimiter || args[i+1] == "" || args[i+2] != "--allow-tool-sha256" || i+3 >= delimiter {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv has an unpaired allowed-tool pin")
			}
			tool, digest := args[i+1], args[i+3]
			if !filepath.IsAbs(tool) || filepath.Clean(tool) != tool || !validRoleDigest(digest) {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv has an invalid allowed-tool identity")
			}
			binding.AllowedTools = append(binding.AllowedTools, tool)
			binding.AllowedToolSHA256 = append(binding.AllowedToolSHA256, digest)
			i += 3
		case "--allow-tool-sha256":
			return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv contains an unpaired --allow-tool-sha256")
		default:
			value, known := values[name]
			if !known {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv contains unsupported option %q", name)
			}
			if seen[name] {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv repeats %s", name)
			}
			if i+1 >= delimiter || args[i+1] == "" {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv option %s has no value", name)
			}
			*value = args[i+1]
			seen[name] = true
			i++
		}
	}
	for _, required := range []string{"--root", "--workspace", "--receipt", "--role", "--execution-id", "--policy-path", "--expected-manifest-sha256", "--expected-policy-sha256", "--expected-executable-sha256"} {
		if !seen[required] {
			return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv is missing %s", required)
		}
	}
	if !filepath.IsAbs(binding.Root) || filepath.Clean(binding.Root) != binding.Root || !normalizedRolePath(binding.WorkspacePath) || !normalizedRolePath(binding.ReceiptPath) || !normalizedRolePath(binding.PolicyPath) || !validRoleExecutionID(binding.ExecutionID) || !validRoleDigest(binding.ManifestSHA256) || !validRoleDigest(binding.PolicySHA256) || !validRoleDigest(binding.ExecutableSHA256) {
		return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv has malformed root/path/identity/pin values")
	}
	if binding.Governed {
		for _, required := range []string{"--approval", "--review-policy", "--oracle", "--expected-approval-sha256", "--expected-review-policy-sha256", "--expected-contract-sha256", "--expected-contract-source-sha256", "--expected-oracle-policy-sha256", "--expected-oracle-policy-file-sha256"} {
			if !seen[required] {
				return roleExecutionBinding{}, true, fmt.Errorf("governed native role-execute argv is missing %s", required)
			}
		}
		if !normalizedRolePath(binding.ApprovalPath) || !normalizedRolePath(binding.ReviewPolicyPath) || !normalizedRolePath(binding.OraclePath) {
			return roleExecutionBinding{}, true, fmt.Errorf("governed native role-execute argv has malformed governance artifact paths")
		}
		for _, digest := range []string{binding.ApprovalSHA256, binding.ReviewPolicySHA256, binding.ContractSHA256, binding.ContractSourceSHA256, binding.OraclePolicySHA256, binding.OraclePolicyFileSHA256} {
			if !validRoleDigest(digest) {
				return roleExecutionBinding{}, true, fmt.Errorf("governed native role-execute argv has malformed governance digest")
			}
		}
		if intent.RoleKind == "oracle-writer" {
			if binding.OracleSHA256 != "" && !validRoleDigest(binding.OracleSHA256) {
				return roleExecutionBinding{}, true, fmt.Errorf("governed native role-execute argv has malformed optional oracle digest")
			}
		} else if !validRoleDigest(binding.OracleSHA256) {
			return roleExecutionBinding{}, true, fmt.Errorf("governed native role-execute argv is missing the frozen oracle digest")
		}
	} else {
		for _, name := range []string{"--approval", "--review-policy", "--oracle", "--expected-approval-sha256", "--expected-review-policy-sha256", "--expected-contract-sha256", "--expected-contract-source-sha256", "--expected-oracle-policy-sha256", "--expected-oracle-policy-file-sha256", "--expected-oracle-sha256"} {
			if seen[name] {
				return roleExecutionBinding{}, true, fmt.Errorf("ungoverned native role-execute argv must not contain %s", name)
			}
		}
	}
	if binding.Agent != "" {
		if hasDelimiter || binding.Agent != agentlaunch.CodexAgent || !filepath.IsAbs(binding.AgentExecutable) || filepath.Clean(binding.AgentExecutable) != binding.AgentExecutable || !normalizedRolePath(binding.PromptPath) || !validRoleDigest(binding.PromptSHA256) {
			return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv has malformed or unsupported typed-agent arguments")
		}
		brokerFields := []string{"--agent-provider", "--agent-credential-env", "--agent-broker-port", "--agent-max-requests", "--agent-max-output-tokens", "--agent-timeout-seconds"}
		brokerFieldCount := 0
		for _, field := range brokerFields {
			if seen[field] {
				brokerFieldCount++
			}
		}
		if brokerFieldCount != 0 {
			if brokerFieldCount != len(brokerFields) {
				return roleExecutionBinding{}, true, fmt.Errorf("broker-backed typed-agent argv must provide every fixed broker option")
			}
			if binding.AgentProvider != "openai-broker" || !brokerCredentialEnvPattern.MatchString(binding.AgentCredentialEnv) || binding.AgentCredentialEnv == "INGEN_CODEX_BROKER_TOKEN" || strings.TrimSpace(binding.AgentModel) == "" {
				return roleExecutionBinding{}, true, fmt.Errorf("typed-agent broker provider or credential environment name is unsupported")
			}
			parsed, err := parseCanonicalBoundedInt("broker port", brokerPortText, 1, 65535)
			if err != nil {
				return roleExecutionBinding{}, true, err
			}
			binding.AgentBrokerPort = parsed
			parsed, err = parseCanonicalBoundedInt("broker maximum requests", brokerMaxRequestsText, 1, 64)
			if err != nil {
				return roleExecutionBinding{}, true, err
			}
			binding.AgentMaxRequests = parsed
			parsed, err = parseCanonicalBoundedInt("broker maximum output tokens", brokerMaxOutputTokensText, 1, 8192)
			if err != nil {
				return roleExecutionBinding{}, true, err
			}
			binding.AgentMaxOutputTokens = parsed
			parsed, err = parseCanonicalBoundedInt("broker timeout seconds", brokerTimeoutSecondsText, 1, 1800)
			if err != nil {
				return roleExecutionBinding{}, true, err
			}
			binding.AgentTimeoutSeconds = parsed
			binding.AgentBrokerProfile = true
		}
	} else {
		for _, name := range []string{"--agent-executable", "--prompt-file", "--expected-prompt-sha256", "--agent-model", "--agent-provider", "--agent-credential-env", "--agent-broker-port", "--agent-max-requests", "--agent-max-output-tokens", "--agent-timeout-seconds"} {
			if seen[name] {
				return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv contains %s without --agent codex", name)
			}
		}
		if !hasDelimiter || delimiter == len(args)-1 || strings.TrimSpace(args[delimiter+1]) == "" {
			return roleExecutionBinding{}, true, fmt.Errorf("native role-execute argv is missing its command delimiter or command")
		}
	}
	return binding, true, nil
}

func verifyRoleAgentContext(root string, binding roleExecutionBinding, report evidence.Report) error {
	if binding.Agent == "" {
		if report.AgentContext != nil {
			return fmt.Errorf("untyped native role-execute argv has a typed agent context in its report")
		}
		return nil
	}
	context := report.AgentContext
	wantNetworkMode := "disabled"
	if binding.AgentBrokerProfile {
		wantNetworkMode = "allowlist"
	}
	if context == nil || report.ExecutablePath != binding.AgentExecutable || context.Agent != "codex-cli" || context.Mode != "exec-ephemeral" || context.ExecutionID != binding.ExecutionID || context.Root != root || context.NetworkMode != wantNetworkMode || context.PromptSourcePath != binding.PromptPath || context.PromptSourceSHA256 != binding.PromptSHA256 || context.Model != binding.AgentModel || context.PromptSnapshotSHA256 != binding.PromptSHA256 {
		return fmt.Errorf("contained role report Codex context does not match immutable native argv pins")
	}
	if binding.AgentBrokerProfile {
		broker := context.Broker
		endpoint := fmt.Sprintf("http://127.0.0.1:%d/v1", binding.AgentBrokerPort)
		if binding.AgentProvider != "openai-broker" || broker == nil || broker.Provider != "openai-responses" || broker.Endpoint != endpoint || broker.CredentialSource != "environment" || broker.CredentialEnv != binding.AgentCredentialEnv || broker.MaxRequests != binding.AgentMaxRequests || broker.MaxOutputTokens != binding.AgentMaxOutputTokens || broker.TimeoutSeconds != binding.AgentTimeoutSeconds {
			return fmt.Errorf("contained role report broker context does not match immutable native argv profile")
		}
		if err := verifyBrokerExecution(binding, context, report.BrokerExecution); err != nil {
			return err
		}
	} else if context.Broker != nil || report.BrokerExecution != nil {
		return fmt.Errorf("offline typed role report unexpectedly contains broker configuration or execution statistics")
	}
	wantSnapshot := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", binding.ExecutionID+".prompt"))
	if context.PromptSnapshotPath != wantSnapshot {
		return fmt.Errorf("contained role report Codex prompt snapshot path does not match execution identity")
	}
	_, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: context.ExecutionID, Root: context.Root, WorkingDir: context.WorkingDirectory,
		HomePath: context.HomePath, CodexHome: context.CodexHomePath,
		PromptSourcePath: context.PromptSourcePath, PromptSourceSHA256: context.PromptSourceSHA256,
		PromptSnapshotPath: context.PromptSnapshotPath, PromptSnapshotSHA256: context.PromptSnapshotSHA256,
		PromptBytes: context.PromptBytes, Model: context.Model, Broker: context.Broker,
	})
	if err != nil || !reflect.DeepEqual(args, context.Args) {
		return fmt.Errorf("contained role report Codex argv does not match fixed Sentinel profile")
	}
	return nil
}

func verifyBrokerExecution(binding roleExecutionBinding, context *agentlaunch.CodexContext, report *roleexec.BrokerExecution) error {
	if context == nil || context.Broker == nil || report == nil {
		return fmt.Errorf("broker-backed role report omits its broker context or execution snapshot")
	}
	broker := context.Broker
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v1", binding.AgentBrokerPort)
	if binding.AgentProvider != "openai-broker" || report.Provider != "openai-responses" || report.Endpoint != endpoint || report.Model != context.Model || report.MaxRequests != binding.AgentMaxRequests || report.MaxOutputTokens != binding.AgentMaxOutputTokens || report.TimeoutSeconds != binding.AgentTimeoutSeconds {
		return fmt.Errorf("broker execution model, endpoint, or limits do not match immutable native argv")
	}
	if report.Provider != broker.Provider || report.Endpoint != broker.Endpoint || report.MaxRequests != broker.MaxRequests || report.MaxOutputTokens != broker.MaxOutputTokens || report.TimeoutSeconds != broker.TimeoutSeconds {
		return fmt.Errorf("broker execution metadata does not match the reported typed-agent context")
	}
	return nil
}

func verifyRoleGovernance(root string, record Record, binding roleExecutionBinding, result *workflowgate.Result) error {
	if !binding.Governed {
		if result != nil {
			return fmt.Errorf("ungoverned role-execution report unexpectedly contains a governance result")
		}
		return nil
	}
	if result == nil {
		return fmt.Errorf("governed role-execution report is missing its governance result")
	}
	var stage workflowgate.Stage
	switch record.Intent.RoleKind {
	case "oracle-writer":
		stage = workflowgate.StageOracle
	case "implementation":
		stage = workflowgate.StageImplementation
	case "verifier", "mutation-runner":
		stage = workflowgate.StageVerification
	default:
		return fmt.Errorf("governed native role-execute argv is unsupported for role kind %q", record.Intent.RoleKind)
	}
	if result.Stage != stage || result.WorkspaceID != record.Intent.WorkspaceID || result.WorkspaceManifestSHA256 != record.Intent.WorkspaceManifestSHA256 ||
		result.ApprovalSHA256 != binding.ApprovalSHA256 || result.ReviewPolicySHA256 != binding.ReviewPolicySHA256 ||
		result.ContractSHA256 != binding.ContractSHA256 || result.ContractSourceSHA256 != binding.ContractSourceSHA256 ||
		result.OraclePolicySHA256 != binding.OraclePolicySHA256 || result.OraclePolicyFileSHA256 != binding.OraclePolicyFileSHA256 ||
		result.OracleSHA256 != binding.OracleSHA256 {
		return fmt.Errorf("role-execution governance result does not match exact argv pins or native identity")
	}
	approvalFound := false
	for _, artifact := range result.ApprovalArtifacts {
		if artifact.Kind == "hammond-approval" && artifact.Path == binding.ApprovalPath {
			approvalFound = true
		}
		if !contains(binding.ProtectedPaths, artifact.Path) {
			return fmt.Errorf("role-execution argv does not protect selected governance artifact %q", artifact.Path)
		}
	}
	if !approvalFound || !contains(binding.ProtectedPaths, record.Intent.ReceiptPath) {
		return fmt.Errorf("role-execution argv does not protect the selected approval and receipt inputs")
	}
	current, err := workflowgate.Check(workflowgate.Request{Root: root, WorkspacePath: binding.WorkspacePath, ApprovalPath: binding.ApprovalPath, ReviewPolicyPath: binding.ReviewPolicyPath, Stage: stage, OraclePath: binding.OraclePath})
	if err != nil {
		return fmt.Errorf("revalidate governed role-execution inputs during collection: %w", err)
	}
	if !sameRoleGovernance(result, &current) {
		return fmt.Errorf("governed inputs changed after role execution")
	}
	return nil
}

func roleExecutionFiles(verified evidence.Verified, sessionID string, manifest ciresult.FileRef) ([]roleEvidenceArtifact, error) {
	base := "native-session-" + sessionID + "-role-execution-"
	files := []roleEvidenceArtifact{{ID: base + "report", Kind: "role-execution-report", Path: filepath.ToSlash(filepath.Join(filepath.Dir(verified.Report.PolicyPath), verified.Report.ExecutionID+".json")), SHA256: verified.ReportSHA256}}
	seen := map[string]bool{}
	manifestSeen := false
	policySeen := false
	agentPromptSourceSeen := verified.Report.AgentContext == nil
	agentPromptSnapshotSeen := verified.Report.AgentContext == nil
	stdoutSeen := verified.Report.StdoutSHA256 == ""
	stderrSeen := verified.Report.StderrSHA256 == ""
	for _, file := range verified.Files {
		if !validRoleDigest(file.SHA256) || roleDigest(file.Bytes) != file.SHA256 {
			return nil, fmt.Errorf("role-execution evidence file %q digest does not match its exact bytes", file.Path)
		}
		if !normalizedRolePath(file.Path) {
			return nil, fmt.Errorf("role-execution evidence contains an invalid path")
		}
		identity := file.Kind + "\x00" + file.Path
		if seen[identity] {
			return nil, fmt.Errorf("role-execution evidence contains a duplicate kind/path")
		}
		seen[identity] = true
		switch file.Kind {
		case "workspace-manifest":
			manifestSeen = file.Path == manifest.Path && file.SHA256 == manifest.SHA256
		case "sealed-policy":
			policySeen = file.Path == verified.Report.PolicyPath
		case "stdout":
			if verified.Report.StdoutSHA256 == "" {
				if file.Path != verified.Report.StdoutPath {
					return nil, fmt.Errorf("unpinned role stdout capture path does not match the report")
				}
				stdoutSeen = true
				continue
			}
			stdoutSeen = file.Path == verified.Report.StdoutPath && file.SHA256 == verified.Report.StdoutSHA256
		case "stderr":
			if verified.Report.StderrSHA256 == "" {
				if file.Path != verified.Report.StderrPath {
					return nil, fmt.Errorf("unpinned role stderr capture path does not match the report")
				}
				stderrSeen = true
				continue
			}
			stderrSeen = file.Path == verified.Report.StderrPath && file.SHA256 == verified.Report.StderrSHA256
		case "contract-source", "oracle-policy":
		case "agent-prompt-source", "agent-prompt-snapshot":
			context := verified.Report.AgentContext
			if context == nil {
				return nil, fmt.Errorf("untyped role-execution report contains agent prompt evidence")
			}
			if file.Kind == evidence.FileAgentPromptSource {
				agentPromptSourceSeen = file.Path == context.PromptSourcePath && file.SHA256 == context.PromptSourceSHA256
			} else {
				agentPromptSnapshotSeen = file.Path == context.PromptSnapshotPath && file.SHA256 == context.PromptSnapshotSHA256
			}
		default:
			if !strings.HasPrefix(file.Kind, "governance-") {
				return nil, fmt.Errorf("role-execution evidence contains unsupported file kind %q", file.Kind)
			}
		}
		if file.Kind != "workspace-manifest" {
			pathHash := roleDigest([]byte(file.Kind + "\x00" + file.Path))
			files = append(files, roleEvidenceArtifact{ID: base + file.Kind + "-" + pathHash[:12], Kind: "role-execution-" + file.Kind, Path: file.Path, SHA256: file.SHA256})
		}
	}
	if !manifestSeen || !policySeen || !stdoutSeen || !stderrSeen || !agentPromptSourceSeen || !agentPromptSnapshotSeen {
		return nil, fmt.Errorf("role-execution evidence is missing or mismatches manifest, policy, stdout, or stderr")
	}
	return files, nil
}

func roleEvidenceID(sessionID, component string) string {
	return "native-session-" + sessionID + "-role-execution-" + component
}

func roleExecutionExecutionID(intent nativejournal.Intent) string {
	binding, recognized, err := parseRoleExecutionIntent(intent)
	if err != nil || !recognized {
		return ""
	}
	return binding.ExecutionID
}

func receiptArtifactSHA(receipt *run.Receipt, id string) string {
	if receipt == nil {
		return ""
	}
	for _, artifact := range receipt.Artifacts {
		if artifact.ID == id {
			return artifact.Ref.SHA256
		}
	}
	return ""
}

func normalizedRolePath(path string) bool {
	return path != "" && !filepath.IsAbs(path) && !strings.ContainsRune(path, '\\') && filepath.Clean(path) == path && path != "." && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func parseCanonicalBoundedInt(label, value string, minimum, maximum int) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || strconv.Itoa(parsed) != value || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("native role-execute %s must be a canonical integer between %d and %d", label, minimum, maximum)
	}
	return parsed, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validRoleExecutionID(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func validRoleDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func roleDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func sameRoleGovernance(left, right *workflowgate.Result) bool {
	return reflect.DeepEqual(left, right)
}
