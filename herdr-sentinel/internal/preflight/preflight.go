// Package preflight checks the declarations that make up a fresh InGen
// project. It reports readiness without pretending to execute a workflow or
// enforce a host policy.
package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/capability"
	sentinelrun "ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workspace"
	"ingen/nublar/workflow"
	sornacontract "ingen/sorna/contract"
	sornapolicy "ingen/sorna/policy"
)

const Schema = "ingen.sentinel-preflight/v1"

type Result struct {
	Schema      string  `json:"schema"`
	WorkspaceID string  `json:"workspace_id"`
	Workspace   string  `json:"workspace"`
	Status      string  `json:"status"`
	Assurance   string  `json:"assurance"`
	Checks      []Check `json:"checks"`
}

type Check struct {
	ID     string `json:"id"`
	Owner  string `json:"owner"`
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
}

// Run validates the declarations under root. A result with status "blocked"
// contains a structural failure; "incomplete" means the project is
// structurally coherent but still has an unfinished handoff such as a
// missing contract or unimplemented host enforcement.
func Run(root, workspacePath string) (Result, error) {
	root, err := absoluteRoot(root)
	if err != nil {
		return Result{}, err
	}
	workspacePath = filepath.Clean(workspacePath)
	workspaceFile, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: workspacePath})
	if err != nil {
		return Result{}, fmt.Errorf("resolve Sentinel workspace: %w", err)
	}
	loaded, err := workspace.LoadFile(workspaceFile)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Schema:      Schema,
		WorkspaceID: loaded.ID,
		Workspace:   workspacePath,
		Status:      "ready",
		Assurance:   "unverified",
		Checks:      make([]Check, 0, 7),
	}
	result.add("workspace", "sentinel", "pass", workspacePath, "workspace manifest is valid")

	plan, err := capability.FromFileUnderRoot(root, workspacePath)
	if err != nil {
		result.add("capability-plan", "sentinel", "fail", workspacePath, err.Error())
	} else {
		result.add("capability-plan", "sentinel", "pass", workspacePath, "role declarations and root boundaries are coherent")
		result.Assurance = plan.Assurance
	}

	result.checkPolicy(root, "oracle-policy", "sorna", loaded.Sorna.OraclePolicy)
	result.checkPolicy(root, "subject-policy", "sorna", loaded.Sorna.SubjectPolicy)
	result.checkWorkflow(root, loaded.Delivery.Workflow)
	result.checkContract(root, loaded.Contract.Path)
	result.checkPolicyBindings(root, loaded)
	result.add("host-enforcement", "sorna", "pending", "", "Sentinel local sessions currently declare capabilities but do not apply an operating-system sandbox")

	if result.has("fail") {
		result.Status = "blocked"
	} else if result.has("pending") {
		result.Status = "incomplete"
	}
	return result, nil
}

func (result *Result) checkPolicy(root, id, owner, path string) {
	resolved, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: path})
	if err != nil {
		result.add(id, owner, "fail", path, err.Error())
		return
	}
	document, err := sornapolicy.LoadFile(resolved)
	if err != nil {
		result.add(id, owner, "fail", path, err.Error())
		return
	}
	enforcement, _ := document.Policy["enforcement"].(string)
	result.add(id, owner, "pass", path, fmt.Sprintf("valid %s policy (%s)", id, enforcement))
}

func (result *Result) checkWorkflow(root, path string) {
	resolved, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: path})
	if err != nil {
		result.add("delivery-workflow", "nublar", "fail", path, err.Error())
		return
	}
	if _, err := workflow.LoadFile(resolved); err != nil {
		result.add("delivery-workflow", "nublar", "fail", path, err.Error())
		return
	}
	result.add("delivery-workflow", "nublar", "pass", path, "valid declarative result workflow")
}

func (result *Result) checkContract(root, path string) {
	if err := sentinelrun.ValidatePathUnderRoot(root, path); err != nil {
		result.add("contract", "sorna", "fail", path, err.Error())
		return
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		result.add("contract", "sorna", "fail", path, err.Error())
		return
	}
	contractPath := filepath.Join(rootPath, filepath.Clean(path))
	if _, err := os.Stat(contractPath); os.IsNotExist(err) {
		result.add("contract", "malcolm", "pending", path, "contract file has not been produced from the Malcolm specification")
		return
	} else if err != nil {
		result.add("contract", "malcolm", "fail", path, err.Error())
		return
	}
	document, err := sornacontract.LoadFile(contractPath)
	if err != nil {
		result.add("contract", "sorna", "fail", path, err.Error())
		return
	}
	status, _ := document.Contract["status"].(string)
	if status == "draft" {
		sealed, err := verifySealedSnapshot(root, path)
		if err != nil {
			result.add("contract", "sorna", "fail", path, err.Error())
			return
		}
		if sealed {
			result.add("contract", "sorna", "pass", path, "valid draft source with a verified sealed canonical snapshot")
			return
		}
		result.add("contract", "sorna", "pending", path, "valid draft contract; seal it before oracle generation and implementation")
		return
	}
	result.add("contract", "sorna", "pass", path, fmt.Sprintf("valid %s contract", status))
}

func (result *Result) checkPolicyBindings(root string, loaded workspace.Workspace) {
	contractPath, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: loaded.Contract.Path})
	if err != nil {
		return
	}
	contractDocument, err := sornacontract.LoadFile(contractPath)
	if err != nil {
		return
	}
	contractID, ok := contractDocument.Contract["id"].(string)
	if !ok || contractID == "" {
		return
	}
	bindings := []struct{ name, path string }{
		{name: "oracle", path: loaded.Sorna.OraclePolicy},
		{name: "subject", path: loaded.Sorna.SubjectPolicy},
	}
	for _, binding := range bindings {
		policyPath, resolveErr := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: binding.path})
		if resolveErr != nil {
			return
		}
		policyDocument, loadErr := sornapolicy.LoadFile(policyPath)
		if loadErr != nil {
			return
		}
		policySubjectID, subjectErr := sornapolicy.SubjectID(policyDocument)
		if subjectErr != nil {
			result.add("policy-binding", "sorna", "fail", binding.path, fmt.Sprintf("%s policy has no valid subject identity: %v", binding.name, subjectErr))
			return
		}
		if policySubjectID != contractID {
			result.add("policy-binding", "sorna", "fail", binding.path, fmt.Sprintf("%s policy subject ID %q does not match contract ID %q", binding.name, policySubjectID, contractID))
			return
		}
	}
	result.add("policy-binding", "sorna", "pass", loaded.Contract.Path, fmt.Sprintf("oracle and subject policies bind to contract %q", contractID))
}

func verifySealedSnapshot(root, contractPath string) (bool, error) {
	canonicalPath := filepath.Join(filepath.Dir(contractPath), "canonical.json")
	hashPath := filepath.Join(filepath.Dir(contractPath), "hash.txt")
	canonical, canonicalExists, err := optionalFileUnderRoot(root, canonicalPath)
	if err != nil {
		return false, err
	}
	hashFile, hashExists, err := optionalFileUnderRoot(root, hashPath)
	if err != nil {
		return false, err
	}
	if !canonicalExists && !hashExists {
		return false, nil
	}
	if !canonicalExists || !hashExists {
		return false, fmt.Errorf("sealed contract snapshot needs both %s and %s", canonicalPath, hashPath)
	}
	sealedDocument, err := sornacontract.LoadFile(canonical)
	if err != nil {
		return false, fmt.Errorf("load sealed contract snapshot %s: %w", canonicalPath, err)
	}
	status, _ := sealedDocument.Contract["status"].(string)
	if status != "sealed" {
		return false, fmt.Errorf("sealed contract snapshot %s must have status sealed, got %q", canonicalPath, status)
	}
	canonicalBytes, err := os.ReadFile(canonical)
	if err != nil {
		return false, fmt.Errorf("read sealed contract snapshot %s: %w", canonicalPath, err)
	}
	hashBytes, err := os.ReadFile(hashFile)
	if err != nil {
		return false, fmt.Errorf("read sealed contract hash %s: %w", hashPath, err)
	}
	digest := sha256.Sum256(canonicalBytes)
	if strings.TrimSpace(string(hashBytes)) != hex.EncodeToString(digest[:]) {
		return false, fmt.Errorf("sealed contract hash %s does not match canonical bytes", hashPath)
	}
	return true, nil
}

func optionalFileUnderRoot(root, path string) (string, bool, error) {
	if err := sentinelrun.ValidatePathUnderRoot(root, path); err != nil {
		return "", false, err
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", false, fmt.Errorf("resolve preflight root: %w", err)
	}
	candidate := filepath.Join(rootPath, filepath.Clean(path))
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("stat optional preflight file %s: %w", path, err)
	}
	resolved, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: path})
	if err != nil {
		return "", false, err
	}
	return resolved, true, nil
}

func (result *Result) add(id, owner, status, path, detail string) {
	result.Checks = append(result.Checks, Check{ID: id, Owner: owner, Status: status, Path: path, Detail: detail})
}

func (result Result) has(status string) bool {
	for _, check := range result.Checks {
		if check.Status == status {
			return true
		}
	}
	return false
}

func (result Result) ExitCode() int {
	if result.Status == "blocked" {
		return 1
	}
	return 0
}

func (result Result) WriteJSON(writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func (result Result) WriteText(writer io.Writer) error {
	if _, err := fmt.Fprintf(writer, "workspace: %s (%s)\nstatus: %s\nassurance: %s\n", result.WorkspaceID, result.Workspace, result.Status, result.Assurance); err != nil {
		return err
	}
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(writer, "[%s] %s (%s): %s", check.Status, check.ID, check.Owner, check.Detail); err != nil {
			return err
		}
		if check.Path != "" {
			if _, err := fmt.Fprintf(writer, " [%s]", check.Path); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}
	}
	return nil
}

func absoluteRoot(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "."
	}
	root, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve Sentinel preflight root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("read Sentinel preflight root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Sentinel preflight root %s is not a directory", root)
	}
	return root, nil
}
