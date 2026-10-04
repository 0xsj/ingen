// Package workflowgate provides a read-only preflight for governed fresh
// project stages. It checks artifact identity and declared ordering; it does
// not enforce filesystem isolation or imply that roles have not seen other
// project data.
package workflowgate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"ingen/hammond/governance"
	"ingen/herdr-sentinel/internal/capability"
	"ingen/herdr-sentinel/internal/workspace"
	"ingen/sorna/contract"
	"ingen/sorna/oracle"
	"ingen/sorna/policy"
)

type Stage string

const (
	StageOracle         Stage = "oracle"
	StageImplementation Stage = "implementation"
	StageVerification   Stage = "verification"
)

// Request names every input needed by a governed stage. All paths are
// project-relative. OraclePath is required only after oracle freeze.
type Request struct {
	Root             string
	WorkspacePath    string
	ApprovalPath     string
	ReviewPolicyPath string
	Stage            Stage
	OraclePath       string
}

// Result identifies the exact approved contract and current policy used by
// the check. ContractSHA256 and OraclePolicySHA256 are Sorna's sealed,
// canonical content digests suitable for child-process drift checks.
type Result struct {
	Stage                   Stage                         `json:"stage"`
	WorkspaceID             string                        `json:"workspace_id"`
	WorkspaceVersion        int64                         `json:"workspace_version"`
	WorkspaceManifestSHA256 string                        `json:"workspace_manifest_sha256"`
	ApprovalSHA256          string                        `json:"approval_sha256"`
	ContractID              string                        `json:"contract_id"`
	ContractVersion         int64                         `json:"contract_version"`
	ContractSourceSHA256    string                        `json:"contract_source_sha256"`
	ContractSHA256          string                        `json:"contract_sha256"`
	OraclePolicyFileSHA256  string                        `json:"oracle_policy_file_sha256"`
	OraclePolicySHA256      string                        `json:"oracle_policy_sha256"`
	OracleSHA256            string                        `json:"oracle_sha256,omitempty"`
	ApprovalRecordID        string                        `json:"approval_record_id"`
	ApprovalArtifacts       []governance.VerifiedArtifact `json:"approval_artifacts"`
	ReviewPolicySHA256      string                        `json:"review_policy_sha256"`
	Enforcement             string                        `json:"enforcement"`
	Assurance               string                        `json:"assurance"`
}

// Check validates the workspace, the current sealed contract against its
// Hammond approval, and (for implementation or verification) the frozen
// oracle against both the approved contract and current oracle policy.
func Check(request Request) (Result, error) {
	if strings.TrimSpace(request.Root) == "" {
		return Result{}, fmt.Errorf("workflow gate project root is required")
	}
	root, err := filepath.Abs(request.Root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve workflow gate project root: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return Result{}, fmt.Errorf("inspect workflow gate project root: %w", err)
	}
	if !rootInfo.IsDir() {
		return Result{}, fmt.Errorf("workflow gate project root is not a directory")
	}
	if request.Stage != StageOracle && request.Stage != StageImplementation && request.Stage != StageVerification {
		return Result{}, fmt.Errorf("unsupported governed workflow stage %q", request.Stage)
	}
	workspacePath, err := normalizedProjectPath("workspace", request.WorkspacePath)
	if err != nil {
		return Result{}, err
	}
	approvalPath, err := normalizedProjectPath("approval", request.ApprovalPath)
	if err != nil {
		return Result{}, err
	}
	reviewPolicyPath, err := normalizedProjectPath("active review policy", request.ReviewPolicyPath)
	if err != nil {
		return Result{}, err
	}
	workspaceBytes, err := readRooted(root, workspacePath, "Sentinel workspace")
	if err != nil {
		return Result{}, err
	}
	workspaceDocument, err := workspace.LoadBytes(workspacePath, workspaceBytes)
	if err != nil {
		return Result{}, fmt.Errorf("validate Sentinel workspace: %w", err)
	}
	plan, err := capability.FromFileUnderRoot(root, workspacePath)
	if err != nil {
		return Result{}, fmt.Errorf("compile Sentinel workspace capability plan: %w", err)
	}
	planHash := hashBytes(workspaceBytes)
	if plan.Workspace.Manifest.SHA256 != planHash {
		return Result{}, fmt.Errorf("Sentinel workspace changed while it was being validated")
	}

	contractPath, err := normalizedProjectPath("workspace contract", workspaceDocument.Contract.Path)
	if err != nil {
		return Result{}, err
	}
	contractSourceBytes, err := readRooted(root, contractPath, "Sorna contract source")
	if err != nil {
		return Result{}, err
	}
	contractFile := filepath.Join(root, filepath.FromSlash(contractPath))
	contractDocument, err := contract.LoadBytes(contractFile, contractSourceBytes)
	if err != nil {
		return Result{}, fmt.Errorf("load Sorna contract source: %w", err)
	}
	status, _ := contractDocument.Contract["status"].(string)
	if status != "draft" && status != "sealed" {
		return Result{}, fmt.Errorf("workflow gate requires a draft or sealed workspace contract source, got %q", status)
	}
	if err := validateFixtures(root, contractPath, contractDocument, status == "sealed"); err != nil {
		return Result{}, err
	}
	var sealedContract contract.Sealed
	if status == "draft" {
		sealedContract, err = contract.SealWithFixtureReader(contractDocument, func(fixturePath string) ([]byte, error) {
			projectPath, pathErr := projectFixturePath(contractPath, fixturePath)
			if pathErr != nil {
				return nil, pathErr
			}
			return readRooted(root, projectPath, "Sorna contract fixture")
		})
		if err != nil {
			return Result{}, fmt.Errorf("seal current Sorna contract source: %w", err)
		}
	} else {
		canonical, canonicalErr := contract.CanonicalJSON(contractDocument)
		if canonicalErr != nil {
			return Result{}, fmt.Errorf("canonicalize current sealed Sorna contract: %w", canonicalErr)
		}
		sealedContract = contract.Sealed{Document: contractDocument, CanonicalJSON: canonical, SHA256: hashBytes(canonical)}
	}
	contractID, _ := sealedContract.Document.Contract["id"].(string)
	contractVersion, ok := integer(sealedContract.Document.Contract["version"])
	if !ok {
		return Result{}, fmt.Errorf("sealed Sorna contract version is not an integer")
	}
	if contractID != workspaceDocument.ID {
		return Result{}, fmt.Errorf("workspace ID %q does not match contract ID %q", workspaceDocument.ID, contractID)
	}
	expected := governance.ContractReference{
		ProjectID: workspaceDocument.ID,
		ID:        contractID,
		Version:   int(contractVersion),
		Schema:    contract.Schema,
		Artifact:  governance.Artifact{SHA256: sealedContract.SHA256},
	}
	approval, err := governance.VerifyApproved(root, approvalPath, reviewPolicyPath, expected)
	if err != nil {
		return Result{}, fmt.Errorf("verify Hammond approval: %w", err)
	}
	approvedContractBytes, err := readRooted(root, approval.Contract.Artifact.URI, "approved canonical contract")
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(approvedContractBytes, sealedContract.CanonicalJSON) {
		return Result{}, fmt.Errorf("current contract source does not seal to the approved canonical contract bytes")
	}

	oraclePolicyPath, err := normalizedProjectPath("workspace oracle policy", plan.Workspace.OraclePolicy.Path)
	if err != nil {
		return Result{}, err
	}
	policyBytes, err := readRooted(root, oraclePolicyPath, "Sorna oracle policy")
	if err != nil {
		return Result{}, err
	}
	if hashBytes(policyBytes) != plan.Workspace.OraclePolicy.SHA256 {
		return Result{}, fmt.Errorf("Sorna oracle policy changed after workspace validation")
	}
	oraclePolicyFile := filepath.Join(root, filepath.FromSlash(oraclePolicyPath))
	policyDocument, err := policy.LoadBytes(oraclePolicyFile, policyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("load Sorna oracle policy: %w", err)
	}
	sealedPolicy, err := policy.Seal(policyDocument)
	if err != nil {
		return Result{}, fmt.Errorf("seal current Sorna oracle policy: %w", err)
	}

	result := Result{
		Stage:                   request.Stage,
		WorkspaceID:             workspaceDocument.ID,
		WorkspaceVersion:        workspaceDocument.Version,
		WorkspaceManifestSHA256: plan.Workspace.Manifest.SHA256,
		ApprovalSHA256:          approvalDigest(approval.Artifacts),
		ContractID:              contractID,
		ContractVersion:         contractVersion,
		ContractSourceSHA256:    hashBytes(contractSourceBytes),
		ContractSHA256:          sealedContract.SHA256,
		OraclePolicyFileSHA256:  plan.Workspace.OraclePolicy.SHA256,
		OraclePolicySHA256:      sealedPolicy.SHA256,
		ApprovalRecordID:        approval.Record.RecordID,
		ApprovalArtifacts:       append([]governance.VerifiedArtifact(nil), approval.Artifacts...),
		ReviewPolicySHA256:      approval.Policy.Artifact.SHA256,
		Enforcement:             "declaration-only",
		Assurance:               "unverified",
	}
	if request.Stage == StageImplementation || request.Stage == StageVerification {
		oraclePath, err := normalizedProjectPath("frozen oracle", request.OraclePath)
		if err != nil {
			return Result{}, err
		}
		oracleBytes, err := readRooted(root, oraclePath, "frozen oracle")
		if err != nil {
			return Result{}, err
		}
		oracleFile := filepath.Join(root, filepath.FromSlash(oraclePath))
		frozenOracle, err := oracle.LoadBytes(oracleFile, oracleBytes)
		if err != nil {
			return Result{}, fmt.Errorf("load frozen Sorna oracle: %w", err)
		}
		oracleDigest := hashBytes(oracleBytes)
		sidecarPath := filepath.ToSlash(filepath.Join(filepath.Dir(oraclePath), "hash.txt"))
		if sidecar, exists, readErr := readRootedOptional(root, sidecarPath, "frozen oracle hash sidecar"); readErr != nil {
			return Result{}, readErr
		} else if exists {
			if strings.TrimSpace(string(sidecar)) != oracleDigest {
				return Result{}, fmt.Errorf("frozen Sorna oracle hash sidecar does not match oracle bytes")
			}
		}
		if frozenOracle.Contract.ID != contractID || frozenOracle.Contract.Version != contractVersion || frozenOracle.Contract.SHA256 != sealedContract.SHA256 {
			return Result{}, fmt.Errorf("frozen Sorna oracle does not match the currently approved contract identity and digest")
		}
		if frozenOracle.PolicySHA256 != sealedPolicy.SHA256 {
			return Result{}, fmt.Errorf("frozen Sorna oracle does not match the current oracle policy digest")
		}
		result.OracleSHA256 = oracleDigest
	}
	return result, nil
}

func normalizedProjectPath(name, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) != raw || strings.ContainsRune(raw, '\\') {
		return "", fmt.Errorf("workflow gate %s path must be a normalized relative path", name)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("workflow gate %s path must be a local project-relative path", name)
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", fmt.Errorf("workflow gate %s path must be relative to the project root", name)
	}
	clean := filepath.Clean(raw)
	if clean != raw || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workflow gate %s path %q is not normalized beneath the project root", name, raw)
	}
	return filepath.ToSlash(clean), nil
}

func readRooted(root, relative, label string) ([]byte, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open project root for %s: %w", label, err)
	}
	defer rootHandle.Close()
	name := filepath.FromSlash(relative)
	file, err := rootHandle.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s under project root: %w", label, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", label)
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return contents, nil
}

func readRootedOptional(root, relative, label string) ([]byte, bool, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, false, fmt.Errorf("open project root for %s: %w", label, err)
	}
	defer rootHandle.Close()
	name := filepath.FromSlash(relative)
	file, err := rootHandle.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("open %s under project root: %w", label, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s is not a regular file", label)
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", label, err)
	}
	return contents, true, nil
}

func validateFixtures(root, contractSourcePath string, document contract.Document, requireDigest bool) error {
	value, exists := document.Contract["fixtures"]
	if !exists {
		return nil
	}
	fixtures, ok := value.([]any)
	if !ok {
		return fmt.Errorf("Sorna contract fixtures are invalid")
	}
	for index, raw := range fixtures {
		fixture, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("Sorna contract fixture %d is invalid", index)
		}
		fixturePath, present := fixture["path"].(string)
		if !present || strings.TrimSpace(fixturePath) == "" {
			continue
		}
		projectPath, err := projectFixturePath(contractSourcePath, fixturePath)
		if err != nil {
			return fmt.Errorf("Sorna contract fixture %d: %w", index, err)
		}
		fixtureBytes, err := readRooted(root, projectPath, fmt.Sprintf("Sorna contract fixture %d", index))
		if err != nil {
			return err
		}
		declaredDigest, hasDigest := fixture["sha256"].(string)
		if requireDigest && !hasDigest {
			return fmt.Errorf("sealed Sorna contract fixture %d has no sha256", index)
		}
		if hasDigest && hashBytes(fixtureBytes) != declaredDigest {
			return fmt.Errorf("Sorna contract fixture %d bytes do not match its declared sha256", index)
		}
	}
	return nil
}

func projectFixturePath(contractSourcePath, fixturePath string) (string, error) {
	if strings.TrimSpace(fixturePath) == "" || strings.TrimSpace(fixturePath) != fixturePath || strings.ContainsRune(fixturePath, '\\') {
		return "", fmt.Errorf("path must be normalized relative to the contract source")
	}
	parsed, err := url.Parse(fixturePath)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || filepath.IsAbs(fixturePath) || filepath.VolumeName(fixturePath) != "" {
		return "", fmt.Errorf("path must be local and relative to the contract source")
	}
	if filepath.Clean(fixturePath) != fixturePath {
		return "", fmt.Errorf("path is not normalized")
	}
	rootRelative := filepath.Clean(filepath.Join(filepath.Dir(filepath.FromSlash(contractSourcePath)), filepath.FromSlash(fixturePath)))
	if rootRelative == ".." || strings.HasPrefix(rootRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(rootRelative) {
		return "", fmt.Errorf("path escapes the project root")
	}
	return filepath.ToSlash(rootRelative), nil
}

func approvalDigest(artifacts []governance.VerifiedArtifact) string {
	for _, artifact := range artifacts {
		if artifact.Kind == "hammond-approval" {
			return artifact.SHA256
		}
	}
	return ""
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func integer(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	case float64:
		parsed := int64(value)
		return parsed, float64(parsed) == value
	default:
		return 0, false
	}
}
