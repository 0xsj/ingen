// Package adapter provides Sentinel's handoff to an enforcement authority.
//
// The adapters delegate execution to Sorna's CLI. Sentinel validates the
// capability plan and selects immutable input references; Sorna owns policy
// interpretation, macOS Seatbelt enforcement, and evidence semantics.
package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/capability"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

type Prepared struct {
	Backend                  string
	Enforcement              string
	RoleID                   string
	RoleWorkspace            string
	WorkspaceID              string
	Plan                     ciresult.FileRef
	Oracle                   ciresult.FileRef
	Policy                   ciresult.FileRef
	SubjectPolicy            ciresult.FileRef
	AppliedOraclePath        string
	AppliedPolicyPath        string
	AppliedSubjectPolicyPath string
	Command                  []string
	policyDir                string
}

// PrepareOracle builds a Sorna CLI invocation for the Sentinel oracle-writer
// role. The returned command is a delegation, not an enforcement attestation.
func PrepareOracle(plan capability.Plan, root string, command, sornaCommand []string) (Prepared, error) {
	if err := plan.Validate(); err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel oracle adapter: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel oracle adapter: command must contain an executable")
	}
	if len(sornaCommand) == 0 || strings.TrimSpace(sornaCommand[0]) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel oracle adapter: Sorna command must contain an executable")
	}
	if _, err := readVerifiedFile("workspace manifest", plan.Workspace.Manifest, root); err != nil {
		return Prepared{}, err
	}
	policyContents, err := readVerifiedFile("oracle policy", plan.Workspace.OraclePolicy, root)
	if err != nil {
		return Prepared{}, err
	}
	policyDir, err := os.MkdirTemp("", "ingen-sentinel-policy-*")
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel oracle adapter: create policy snapshot: %w", err)
	}
	policySnapshotPath, err := snapshotFile(policyDir, "oracle-policy.yaml", policyContents)
	if err != nil {
		_ = os.RemoveAll(policyDir)
		return Prepared{}, fmt.Errorf("prepare Sentinel oracle adapter: write policy snapshot: %w", err)
	}
	for _, role := range plan.Roles {
		if role.Kind != "oracle-writer" {
			continue
		}
		wrapped := append([]string(nil), sornaCommand...)
		wrapped = append(wrapped,
			"sandbox", "exec",
			"--policy", policySnapshotPath,
			"--root", root,
			"--",
		)
		wrapped = append(wrapped, command...)
		return Prepared{
			Backend:           "sorna-cli",
			Enforcement:       "pending-host-enforcement",
			RoleID:            role.ID,
			RoleWorkspace:     role.Workspace,
			WorkspaceID:       plan.Workspace.ID,
			Plan:              plan.Workspace.Manifest,
			Policy:            plan.Workspace.OraclePolicy,
			AppliedPolicyPath: policySnapshotPath,
			Command:           wrapped,
			policyDir:         policyDir,
		}, nil
	}
	_ = os.RemoveAll(policyDir)
	return Prepared{}, fmt.Errorf("prepare Sentinel oracle adapter: capability plan has no oracle-writer role")
}

// PrepareVerifier builds the Sorna managed-run command for the verifier role.
// Sentinel binds the exact oracle and policy bytes, while Sorna remains the
// authority for sandbox enforcement, subject lifecycle, and evidence output.
func PrepareVerifier(
	plan capability.Plan,
	root string,
	oraclePath string,
	baseURL string,
	subjectRoot string,
	subjectCommand string,
	readyPath string,
	variant string,
	outputDir string,
	subjectArgs []string,
	sornaCommand []string,
) (Prepared, error) {
	if err := plan.Validate(); err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if strings.TrimSpace(subjectRoot) == "" {
		subjectRoot = root
	}
	if _, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: subjectRoot}); err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: subject root: %w", err)
	}
	if strings.TrimSpace(readyPath) == "" {
		readyPath = "/healthz"
	}
	if strings.TrimSpace(variant) == "" {
		variant = "sentinel-verifier"
	}
	if strings.TrimSpace(oraclePath) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: oracle path is required")
	}
	if err := validateRelativePath("frozen oracle", oraclePath); err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: %w", err)
	}
	if strings.TrimSpace(baseURL) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: base URL is required")
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: base URL must be an absolute http or https URL: %q", baseURL)
	}
	if strings.TrimSpace(subjectCommand) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: subject command is required")
	}
	if strings.TrimSpace(outputDir) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: output directory is required")
	}
	if err := validateRelativePath("output directory", outputDir); err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: %w", err)
	}
	if len(sornaCommand) == 0 || strings.TrimSpace(sornaCommand[0]) == "" {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: Sorna command must contain an executable")
	}

	if _, err := readVerifiedFile("workspace manifest", plan.Workspace.Manifest, root); err != nil {
		return Prepared{}, err
	}
	oraclePolicyContents, err := readVerifiedFile("oracle policy", plan.Workspace.OraclePolicy, root)
	if err != nil {
		return Prepared{}, err
	}
	subjectPolicyContents, err := readVerifiedFile("subject policy", plan.Workspace.SubjectPolicy, root)
	if err != nil {
		return Prepared{}, err
	}
	oracleRef, oracleContents, err := readFileReference(oraclePath, root)
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: read frozen oracle: %w", err)
	}

	verifierRole, ok := findRole(plan, "verifier")
	if !ok {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: capability plan has no verifier role")
	}

	policyDir, err := os.MkdirTemp("", "ingen-sentinel-verifier-*")
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: create snapshots: %w", err)
	}
	oracleSnapshot, err := snapshotFile(policyDir, "frozen-oracle.yaml", oracleContents)
	if err != nil {
		_ = os.RemoveAll(policyDir)
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: write oracle snapshot: %w", err)
	}
	oraclePolicySnapshot, err := snapshotFile(policyDir, "oracle-policy.yaml", oraclePolicyContents)
	if err != nil {
		_ = os.RemoveAll(policyDir)
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: write oracle policy snapshot: %w", err)
	}
	subjectPolicySnapshot, err := snapshotFile(policyDir, "subject-policy.yaml", subjectPolicyContents)
	if err != nil {
		_ = os.RemoveAll(policyDir)
		return Prepared{}, fmt.Errorf("prepare Sentinel verifier adapter: write subject policy snapshot: %w", err)
	}

	command := append([]string{}, sornaCommand...)
	command = append(command,
		"run",
		"--oracle", oracleSnapshot,
		"--policy", oraclePolicySnapshot,
		"--subject-policy", subjectPolicySnapshot,
		"--subject-root", subjectRoot,
		"--base-url", baseURL,
		"--subject-command", subjectCommand,
	)
	for _, arg := range subjectArgs {
		command = append(command, "--subject-arg", arg)
	}
	command = append(command,
		"--ready-path", readyPath,
		"--subject-variant", variant,
		"--output-dir", outputDir,
	)

	return Prepared{
		Backend:                  "sorna-cli",
		Enforcement:              "pending-host-enforcement",
		RoleID:                   verifierRole.ID,
		RoleWorkspace:            verifierRole.Workspace,
		WorkspaceID:              plan.Workspace.ID,
		Plan:                     plan.Workspace.Manifest,
		Oracle:                   oracleRef,
		Policy:                   plan.Workspace.OraclePolicy,
		SubjectPolicy:            plan.Workspace.SubjectPolicy,
		AppliedOraclePath:        oracleSnapshot,
		AppliedPolicyPath:        oraclePolicySnapshot,
		AppliedSubjectPolicyPath: subjectPolicySnapshot,
		Command:                  command,
		policyDir:                policyDir,
	}, nil
}

// Close removes the temporary policy snapshot after the delegated process has
// finished. It is safe to call more than once.
func (p *Prepared) Close() error {
	if p == nil || p.policyDir == "" {
		return nil
	}
	directory := p.policyDir
	p.policyDir = ""
	p.AppliedOraclePath = ""
	p.AppliedPolicyPath = ""
	p.AppliedSubjectPolicyPath = ""
	return os.RemoveAll(directory)
}

func findRole(plan capability.Plan, kind string) (capability.Role, bool) {
	for _, role := range plan.Roles {
		if role.Kind == kind {
			return role, true
		}
	}
	return capability.Role{}, false
}

func snapshotFile(dir, name string, contents []byte) (string, error) {
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		return "", fmt.Errorf("invalid snapshot filename %q", name)
	}
	path := filepath.Join(dir, filepath.Base(name))
	if err := os.WriteFile(path, contents, 0o444); err != nil {
		return "", err
	}
	return path, nil
}

func readFileReference(path, root string) (ciresult.FileRef, []byte, error) {
	resolvedPath, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: path})
	if err != nil {
		return ciresult.FileRef{}, nil, err
	}
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ciresult.FileRef{}, nil, err
	}
	digest := sha256.Sum256(contents)
	return ciresult.FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}, contents, nil
}

func validateRelativePath(name, value string) error {
	clean := filepath.Clean(value)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must be relative to the workspace: %q", name, value)
	}
	return nil
}

func readVerifiedFile(name string, reference ciresult.FileRef, root string) ([]byte, error) {
	resolvedPath, err := sentinelrun.ResolveFileRefUnderRoot(root, reference)
	if err != nil {
		return nil, fmt.Errorf("prepare Sentinel adapter: read %s %s: %w", name, reference.Path, err)
	}
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("prepare Sentinel adapter: read %s %s: %w", name, reference.Path, err)
	}
	digest := sha256.Sum256(contents)
	actual := hex.EncodeToString(digest[:])
	if actual != reference.SHA256 {
		return nil, fmt.Errorf("prepare Sentinel adapter: %s %q changed after capability planning", name, reference.Path)
	}
	return contents, nil
}
