// Package workspace validates Sentinel's orchestration manifest without
// interpreting Sorna contract or mutation semantics.
package workspace

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const Schema = "ingen.sentinel-workspace/v1"

type Document struct {
	Workspace Workspace `yaml:"sentinel_workspace" json:"sentinel_workspace"`
}

type Workspace struct {
	Schema              string       `yaml:"schema" json:"schema"`
	ID                  string       `yaml:"id" json:"id"`
	Version             int64        `yaml:"version" json:"version"`
	ProjectRoot         string       `yaml:"project_root" json:"project_root"`
	Contract            ContractRef  `yaml:"contract" json:"contract"`
	ImplementationRoots []string     `yaml:"implementation_roots" json:"implementation_roots"`
	Sorna               SornaRefs    `yaml:"sorna" json:"sorna"`
	Delivery            DeliveryRefs `yaml:"delivery" json:"delivery"`
	Roles               []Role       `yaml:"roles" json:"roles"`
}

type ContractRef struct {
	Path string `yaml:"path" json:"path"`
}

type SornaRefs struct {
	OraclePolicy  string `yaml:"oracle_policy" json:"oracle_policy"`
	SubjectPolicy string `yaml:"subject_policy" json:"subject_policy"`
}

type DeliveryRefs struct {
	Workflow string `yaml:"workflow" json:"workflow"`
}

type Role struct {
	ID         string   `yaml:"id" json:"id"`
	Kind       string   `yaml:"kind" json:"kind"`
	Workspace  string   `yaml:"workspace" json:"workspace"`
	ReadRoots  []string `yaml:"read_roots" json:"read_roots"`
	WriteRoots []string `yaml:"write_roots" json:"write_roots"`
	DenyRoots  []string `yaml:"deny_roots" json:"deny_roots"`
}

var roleKinds = map[string]bool{
	"contract-author": true,
	"oracle-writer":   true,
	"implementation":  true,
	"verifier":        true,
	"mutation-runner": true,
}

func LoadFile(path string) (Workspace, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Workspace{}, fmt.Errorf("read Sentinel workspace %s: %w", path, err)
	}
	return LoadBytes(path, contents)
}

func LoadBytes(path string, contents []byte) (Workspace, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Workspace{}, fmt.Errorf("parse Sentinel workspace %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Workspace{}, fmt.Errorf("parse Sentinel workspace %s: multiple documents are not supported", path)
		}
		return Workspace{}, fmt.Errorf("parse Sentinel workspace %s: %w", path, err)
	}
	if problems := Validate(document.Workspace); len(problems) > 0 {
		return Workspace{}, fmt.Errorf("invalid Sentinel workspace: %s", strings.Join(problems, "; "))
	}
	return document.Workspace, nil
}

func Validate(workspace Workspace) []string {
	problems := make([]string, 0)
	if workspace.Schema != Schema {
		problems = append(problems, fmt.Sprintf("sentinel_workspace.schema must be %s", Schema))
	}
	if strings.TrimSpace(workspace.ID) == "" {
		problems = append(problems, "sentinel_workspace.id must be non-empty")
	}
	if workspace.Version < 1 {
		problems = append(problems, "sentinel_workspace.version must be positive")
	}
	validateRelativePath(&problems, "sentinel_workspace.project_root", workspace.ProjectRoot)
	validateRelativePath(&problems, "sentinel_workspace.contract.path", workspace.Contract.Path)
	validateRelativePath(&problems, "sentinel_workspace.sorna.oracle_policy", workspace.Sorna.OraclePolicy)
	validateRelativePath(&problems, "sentinel_workspace.sorna.subject_policy", workspace.Sorna.SubjectPolicy)
	validateRelativePath(&problems, "sentinel_workspace.delivery.workflow", workspace.Delivery.Workflow)

	seenImplementationRoots := make(map[string]bool, len(workspace.ImplementationRoots))
	for index, root := range workspace.ImplementationRoots {
		path := fmt.Sprintf("sentinel_workspace.implementation_roots[%d]", index)
		validateRelativePath(&problems, path, root)
		normalized := filepath.Clean(root)
		if seenImplementationRoots[normalized] {
			problems = append(problems, fmt.Sprintf("%s duplicates %q", path, root))
		}
		seenImplementationRoots[normalized] = true
	}
	if len(workspace.ImplementationRoots) == 0 {
		problems = append(problems, "sentinel_workspace.implementation_roots must contain at least one root")
	}

	seenRoleIDs := make(map[string]bool, len(workspace.Roles))
	seenRoleWorkspaces := make(map[string]bool, len(workspace.Roles))
	seenRoleKinds := make(map[string]bool, len(workspace.Roles))
	var oracleWriter *Role
	for index := range workspace.Roles {
		role := &workspace.Roles[index]
		path := fmt.Sprintf("sentinel_workspace.roles[%d]", index)
		if strings.TrimSpace(role.ID) == "" {
			problems = append(problems, path+".id must be non-empty")
		} else if seenRoleIDs[role.ID] {
			problems = append(problems, fmt.Sprintf("%s.id duplicates %q", path, role.ID))
		}
		seenRoleIDs[role.ID] = true
		if !roleKinds[role.Kind] {
			problems = append(problems, fmt.Sprintf("%s.kind %q is unsupported", path, role.Kind))
		}
		seenRoleKinds[role.Kind] = true
		validateRelativePath(&problems, path+".workspace", role.Workspace)
		normalizedWorkspace := filepath.Clean(role.Workspace)
		if seenRoleWorkspaces[normalizedWorkspace] {
			problems = append(problems, fmt.Sprintf("%s.workspace duplicates %q", path, role.Workspace))
		}
		seenRoleWorkspaces[normalizedWorkspace] = true
		validateRoots(&problems, path+".read_roots", role.ReadRoots)
		validateRoots(&problems, path+".write_roots", role.WriteRoots)
		validateRoots(&problems, path+".deny_roots", role.DenyRoots)
		if role.Kind == "oracle-writer" {
			oracleWriter = role
		}
	}
	if len(workspace.Roles) == 0 {
		problems = append(problems, "sentinel_workspace.roles must contain at least one role")
	}
	for _, requiredKind := range []string{"contract-author", "oracle-writer", "implementation", "verifier", "mutation-runner"} {
		if !seenRoleKinds[requiredKind] {
			problems = append(problems, fmt.Sprintf("sentinel_workspace.roles must include a %s role", requiredKind))
		}
	}
	if oracleWriter == nil {
		problems = append(problems, "sentinel_workspace.roles must include an oracle-writer")
	} else {
		if !containsRoot(oracleWriter.ReadRoots, workspace.Contract.Path) {
			problems = append(problems, "oracle-writer must read the declared contract path")
		}
		for _, implementationRoot := range workspace.ImplementationRoots {
			if !containsRoot(oracleWriter.DenyRoots, implementationRoot) {
				problems = append(problems, fmt.Sprintf("oracle-writer must deny implementation root %q", implementationRoot))
			}
		}
	}
	return problems
}

func validateRoots(problems *[]string, prefix string, roots []string) {
	seen := make(map[string]bool, len(roots))
	for index, root := range roots {
		path := fmt.Sprintf("%s[%d]", prefix, index)
		validateRelativePath(problems, path, root)
		normalized := filepath.Clean(root)
		if seen[normalized] {
			*problems = append(*problems, fmt.Sprintf("%s duplicates %q", path, root))
		}
		seen[normalized] = true
	}
}

func validateRelativePath(problems *[]string, path, raw string) {
	if strings.TrimSpace(raw) == "" {
		*problems = append(*problems, path+" must be non-empty")
		return
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		*problems = append(*problems, fmt.Sprintf("%s must stay inside the project root: %q", path, raw))
	}
}

func containsRoot(roots []string, wanted string) bool {
	wanted = filepath.Clean(wanted)
	for _, root := range roots {
		if filepath.Clean(root) == wanted {
			return true
		}
	}
	return false
}
