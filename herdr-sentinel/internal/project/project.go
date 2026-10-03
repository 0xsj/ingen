// Package project creates the local scaffold used by a fresh InGen project.
// It does not invent a behavioral contract or claim that the generated policy
// files have been enforced by a host.
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workspace"
)

const WorkspacePath = ".ingen/workspace.yaml"

type Options struct {
	Root               string
	ID                 string
	ImplementationRoot string
}

type Result struct {
	Root      string
	Workspace string
	Files     []string
}

// Initialize creates a fresh-project scaffold. Existing files are never
// replaced; callers can edit the generated declarations and rerun the
// verification workflow from the same project root.
func Initialize(options Options) (Result, error) {
	root := options.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve project root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Result{}, fmt.Errorf("read project root %s: %w", root, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("project root %s is not a directory", root)
	}

	id := strings.TrimSpace(options.ID)
	if err := validateID(id); err != nil {
		return Result{}, err
	}
	implementationRoot := strings.TrimSpace(options.ImplementationRoot)
	if implementationRoot == "" {
		implementationRoot = "src"
	}
	if err := validateRelativePath("implementation root", implementationRoot); err != nil {
		return Result{}, err
	}

	workspaceDocument := defaultWorkspace(id, implementationRoot)
	workspaceBytes, err := yaml.Marshal(workspaceDocument)
	if err != nil {
		return Result{}, fmt.Errorf("encode generated Sentinel workspace: %w", err)
	}

	files := map[string][]byte{
		WorkspacePath:                 []byte(string(workspaceBytes)),
		".ingen/brief.md":             []byte("# Project brief\n\nDescribe the public behavior this project must provide.\n"),
		".ingen/contract/spec.malc":   []byte("# Contract author: replace this scaffold with the approved Malcolm specification.\n"),
		".ingen/policy/oracle.yaml":   []byte(oraclePolicy(id, implementationRoot)),
		".ingen/policy/subject.yaml":  []byte(subjectPolicy(id, implementationRoot)),
		".ingen/nublar/workflow.yaml": []byte(workflow(id)),
	}
	for path := range files {
		if err := run.ValidatePathUnderRoot(root, path); err != nil {
			return Result{}, fmt.Errorf("validate project file %s: %w", path, err)
		}
		if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
			return Result{}, fmt.Errorf("refusing to overwrite existing project file %s", path)
		} else if !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("check project file %s: %w", path, err)
		}
	}

	directories := []string{
		".ingen/contract",
		".ingen/governance",
		".ingen/policy",
		".ingen/fixtures",
		".ingen/artifacts/oracle",
		".ingen/artifacts/evidence",
		".ingen/artifacts/mutations",
		".ingen/artifacts/provenance",
		".ingen/artifacts/custody",
		".ingen/artifacts/sessions",
		".ingen/sessions/contract-author",
		".ingen/sessions/governance-reviewer",
		".ingen/sessions/oracle-writer",
		".ingen/sessions/implementation",
		".ingen/sessions/verifier",
		".ingen/sessions/mutation-runner",
		".ingen/nublar",
		implementationRoot,
		"docs",
	}
	for _, path := range directories {
		if err := run.ValidateDirectoryPathUnderRoot(root, path); err != nil {
			return Result{}, fmt.Errorf("validate project directory %s: %w", path, err)
		}
	}
	for _, path := range directories {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			return Result{}, fmt.Errorf("create project directory %s: %w", path, err)
		}
	}

	created := make([]string, 0, len(files))
	for path, contents := range files {
		file, err := os.OpenFile(filepath.Join(root, path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return Result{}, fmt.Errorf("write project file %s: %w", path, err)
		}
		_, writeErr := file.Write(contents)
		closeErr := file.Close()
		if writeErr != nil {
			return Result{}, fmt.Errorf("write project file %s: %w", path, writeErr)
		}
		if closeErr != nil {
			return Result{}, fmt.Errorf("close project file %s: %w", path, closeErr)
		}
		created = append(created, path)
	}
	return Result{Root: root, Workspace: WorkspacePath, Files: created}, nil
}

func defaultWorkspace(id, implementationRoot string) workspace.Document {
	return workspace.Document{Workspace: workspace.Workspace{
		Schema:              workspace.Schema,
		ID:                  id,
		Version:             1,
		ProjectRoot:         ".",
		Contract:            workspace.ContractRef{Path: ".ingen/contract/contract.json"},
		ImplementationRoots: []string{implementationRoot},
		Sorna: workspace.SornaRefs{
			OraclePolicy:  ".ingen/policy/oracle.yaml",
			SubjectPolicy: ".ingen/policy/subject.yaml",
		},
		Delivery: workspace.DeliveryRefs{Workflow: ".ingen/nublar/workflow.yaml"},
		Roles: []workspace.Role{
			{
				ID: "contract-author", Kind: "contract-author", Workspace: ".ingen/sessions/contract-author",
				ReadRoots: []string{".ingen/brief.md", "docs"}, WriteRoots: []string{".ingen/contract/spec.malc"},
				DenyRoots: []string{implementationRoot, ".git"},
			},
			{
				ID: "governance-reviewer", Kind: "contract-author", Workspace: ".ingen/sessions/governance-reviewer",
				ReadRoots: []string{".ingen/contract"}, WriteRoots: []string{".ingen/governance"},
				DenyRoots: []string{implementationRoot, ".ingen/oracle", ".git"},
			},
			{
				ID: "oracle-writer", Kind: "oracle-writer", Workspace: ".ingen/sessions/oracle-writer",
				ReadRoots: []string{".ingen/contract/contract.json", ".ingen/fixtures"}, WriteRoots: []string{".ingen/artifacts/oracle"},
				DenyRoots: []string{implementationRoot, ".ingen/sessions/implementation", ".ingen/artifacts/evidence", ".git"},
			},
			{
				ID: "implementation", Kind: "implementation", Workspace: ".ingen/sessions/implementation",
				ReadRoots: []string{".ingen/contract/contract.json", "docs"}, WriteRoots: []string{implementationRoot},
				DenyRoots: []string{".ingen/artifacts/oracle", ".ingen/artifacts/evidence", ".git"},
			},
			{
				ID: "verifier", Kind: "verifier", Workspace: ".ingen/sessions/verifier",
				ReadRoots: []string{".ingen/contract/contract.json", ".ingen/artifacts/oracle", implementationRoot}, WriteRoots: []string{".ingen/artifacts/evidence"},
				DenyRoots: []string{".git"},
			},
			{
				ID: "mutation-runner", Kind: "mutation-runner", Workspace: ".ingen/sessions/mutation-runner",
				ReadRoots: []string{".ingen/contract/contract.json", ".ingen/artifacts/oracle", ".ingen/artifacts/evidence"}, WriteRoots: []string{".ingen/artifacts/mutations"},
				DenyRoots: []string{".git"},
			},
		},
	}}
}

func oraclePolicy(id, implementationRoot string) string {
	return fmt.Sprintf(`policy:
  schema: ingen.policy/v1
  id: %s-oracle
  version: 1
  status: draft
  purpose: independent-oracle-generation
  enforcement: host-enforced
  filesystem:
    read:
      - path: .ingen/contract
        reason: approved contract and public fixtures
      - path: .ingen/fixtures
        reason: public oracle fixtures
    write:
      - path: .ingen/artifacts/oracle
        reason: frozen oracle output and generation evidence
    deny:
      - path: %s
        reason: implementation source must not enter oracle generation
      - path: .git
        reason: repository history and metadata are not oracle inputs
  network:
    mode: disabled
  process:
    subject_id: %s
    can_invoke_subject: false
    allowed_tools:
      - name: go
        purpose: deterministic Sorna oracle tooling
`, id, implementationRoot, id)
}

func subjectPolicy(id, implementationRoot string) string {
	return fmt.Sprintf(`policy:
  schema: ingen.policy/v1
  id: %s-subject
  version: 1
  status: draft
  purpose: managed-subject-public-boundary
  enforcement: host-enforced
  filesystem:
    read:
      - path: %s
        reason: compiled subject implementation
    write: []
    deny:
      - path: .ingen/contract
        reason: subject must not read private contract input
      - path: .ingen/artifacts/oracle
        reason: subject must not read frozen oracle evidence
      - path: .git
        reason: repository history and metadata are outside the subject boundary
  network:
    mode: allowlist
    allow:
      - host: localhost
        ports: [8080]
        direction: inbound
        purpose: local Sorna readiness and public requests
  process:
    subject_id: %s
    can_invoke_subject: false
`, id, implementationRoot, id)
}

func workflow(id string) string {
	return fmt.Sprintf(`schema: ingen.nublar-workflow/v1
id: %s-delivery
checks:
  # Herdr coordination is optional; Sorna evidence is the required gate.
  - id: sentinel-run
    tool: sentinel
    result: .ingen/artifacts/sentinel-run.json
    required: false
  - id: sorna-evidence
    tool: sorna
    result: .ingen/artifacts/evidence-ci-result.json
`, id)
}

func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("project id must not be empty")
	}
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("project id %q contains unsupported character %q", id, character)
	}
	return nil
}

func validateRelativePath(name, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must stay inside the project root: %q", name, raw)
	}
	return nil
}
