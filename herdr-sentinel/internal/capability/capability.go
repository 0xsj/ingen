// Package capability compiles a Sentinel workspace into a host-adapter plan.
//
// The plan is a derived handoff artifact. It checks that the declared
// capabilities are internally coherent, but it does not enforce them.
package capability

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workspace"
)

const Schema = "ingen.sentinel-capability-plan/v1"

type Plan struct {
	Schema              string       `json:"schema"`
	Workspace           WorkspaceRef `json:"workspace"`
	ImplementationRoots []string     `json:"implementation_roots"`
	Enforcement         string       `json:"enforcement"`
	Assurance           string       `json:"assurance"`
	Roles               []Role       `json:"roles"`
}

type WorkspaceRef struct {
	ID            string           `json:"id"`
	Version       int64            `json:"version"`
	Manifest      ciresult.FileRef `json:"manifest"`
	OraclePolicy  ciresult.FileRef `json:"oracle_policy"`
	SubjectPolicy ciresult.FileRef `json:"subject_policy"`
}

type Role struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Workspace  string   `json:"workspace"`
	ReadRoots  []string `json:"read_roots"`
	WriteRoots []string `json:"write_roots"`
	DenyRoots  []string `json:"deny_roots"`
}

// FromFile compiles a validated workspace manifest and hashes the exact bytes
// that were used to produce the plan.
func FromFile(path string) (Plan, error) {
	return FromFileUnderRoot(".", path)
}

// FromFileUnderRoot compiles a validated workspace manifest and hashes the
// exact bytes used to produce the plan, resolving all manifest references
// relative to root.
func FromFileUnderRoot(root, path string) (Plan, error) {
	path = filepath.Clean(path)
	if err := validateRelativePath("workspace manifest", path); err != nil {
		return Plan{}, err
	}
	resolvedPath, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: path})
	if err != nil {
		return Plan{}, fmt.Errorf("resolve Sentinel workspace %s: %w", path, err)
	}
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return Plan{}, fmt.Errorf("read Sentinel workspace %s: %w", path, err)
	}
	loaded, err := workspace.LoadBytes(path, contents)
	if err != nil {
		return Plan{}, err
	}
	digest := sha256.Sum256(contents)
	oraclePolicy, err := fileReference(root, loaded.Sorna.OraclePolicy)
	if err != nil {
		return Plan{}, err
	}
	subjectPolicy, err := fileReference(root, loaded.Sorna.SubjectPolicy)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Schema: Schema,
		Workspace: WorkspaceRef{
			ID:      loaded.ID,
			Version: loaded.Version,
			Manifest: ciresult.FileRef{
				Path:   path,
				SHA256: hex.EncodeToString(digest[:]),
			},
			OraclePolicy:  oraclePolicy,
			SubjectPolicy: subjectPolicy,
		},
		ImplementationRoots: cleanRoots(loaded.ImplementationRoots),
		Enforcement:         "declaration-only",
		Assurance:           "unverified",
		Roles:               make([]Role, 0, len(loaded.Roles)),
	}
	for _, sourceRole := range loaded.Roles {
		plan.Roles = append(plan.Roles, Role{
			ID:         sourceRole.ID,
			Kind:       sourceRole.Kind,
			Workspace:  filepath.Clean(sourceRole.Workspace),
			ReadRoots:  cleanRoots(sourceRole.ReadRoots),
			WriteRoots: cleanRoots(sourceRole.WriteRoots),
			DenyRoots:  cleanRoots(sourceRole.DenyRoots),
		})
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// Validate checks the derived plan and its cross-root invariants. It does not
// infer host-specific policy or claim that a process would be contained.
func (p Plan) Validate() error {
	if p.Schema != Schema {
		return fmt.Errorf("Sentinel capability plan schema must be %s, got %q", Schema, p.Schema)
	}
	if strings.TrimSpace(p.Workspace.ID) == "" {
		return fmt.Errorf("Sentinel capability plan workspace id is required")
	}
	if p.Workspace.Version < 1 {
		return fmt.Errorf("Sentinel capability plan workspace version must be positive")
	}
	if err := validateFileRef("workspace manifest", p.Workspace.Manifest); err != nil {
		return err
	}
	if err := validateRelativePath("workspace manifest", p.Workspace.Manifest.Path); err != nil {
		return err
	}
	if err := validatePolicyRef("oracle policy", p.Workspace.OraclePolicy); err != nil {
		return err
	}
	if err := validatePolicyRef("subject policy", p.Workspace.SubjectPolicy); err != nil {
		return err
	}
	if p.Enforcement != "declaration-only" {
		return fmt.Errorf("Sentinel capability plan enforcement must be declaration-only, got %q", p.Enforcement)
	}
	if p.Assurance != "unverified" {
		return fmt.Errorf("Sentinel capability plan assurance must be unverified, got %q", p.Assurance)
	}
	if err := validateRoots("implementation_roots", p.ImplementationRoots); err != nil {
		return err
	}
	if len(p.ImplementationRoots) == 0 {
		return fmt.Errorf("Sentinel capability plan needs at least one implementation root")
	}
	if len(p.Roles) == 0 {
		return fmt.Errorf("Sentinel capability plan needs at least one role")
	}

	seenIDs := make(map[string]bool, len(p.Roles))
	for index, role := range p.Roles {
		if err := role.validate(index, seenIDs); err != nil {
			return err
		}
	}

	for _, role := range p.Roles {
		if role.Kind != "oracle-writer" {
			continue
		}
		for _, implementationRoot := range p.ImplementationRoots {
			if containsOverlapping(role.ReadRoots, implementationRoot) || containsOverlapping(role.WriteRoots, implementationRoot) {
				return fmt.Errorf("oracle-writer must not allow implementation root %q", implementationRoot)
			}
			if !containsOverlapping(role.DenyRoots, implementationRoot) {
				return fmt.Errorf("oracle-writer must deny implementation root %q", implementationRoot)
			}
		}
	}
	return nil
}

func (r Role) validate(index int, seenIDs map[string]bool) error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("Sentinel capability role %d needs an id", index+1)
	}
	if seenIDs[r.ID] {
		return fmt.Errorf("Sentinel capability role ID %q was duplicated", r.ID)
	}
	seenIDs[r.ID] = true
	if strings.TrimSpace(r.Kind) == "" {
		return fmt.Errorf("Sentinel capability role %q needs a kind", r.ID)
	}
	if err := validateRelativePath("role "+r.ID+" workspace", r.Workspace); err != nil {
		return err
	}
	if err := validateRoots(r.ID+" read_roots", r.ReadRoots); err != nil {
		return err
	}
	if err := validateRoots(r.ID+" write_roots", r.WriteRoots); err != nil {
		return err
	}
	if err := validateRoots(r.ID+" deny_roots", r.DenyRoots); err != nil {
		return err
	}
	for _, allowed := range append(append([]string{}, r.ReadRoots...), r.WriteRoots...) {
		for _, denied := range r.DenyRoots {
			if pathsOverlap(allowed, denied) {
				return fmt.Errorf("Sentinel capability role %q allows root %q and denies overlapping root %q", r.ID, allowed, denied)
			}
		}
	}
	return nil
}

func cleanRoots(roots []string) []string {
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		cleaned = append(cleaned, filepath.Clean(root))
	}
	return cleaned
}

func validateRoots(name string, roots []string) error {
	seen := make(map[string]bool, len(roots))
	for _, root := range roots {
		if err := validateRelativePath(name, root); err != nil {
			return err
		}
		normalized := filepath.Clean(root)
		if seen[normalized] {
			return fmt.Errorf("Sentinel capability %s root %q was duplicated", name, root)
		}
		seen[normalized] = true
	}
	return nil
}

func containsOverlapping(roots []string, wanted string) bool {
	for _, root := range roots {
		if pathsOverlap(root, wanted) {
			return true
		}
	}
	return false
}

func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || strings.HasPrefix(left, right+string(filepath.Separator)) || strings.HasPrefix(right, left+string(filepath.Separator))
}

func validateFileRef(name string, ref ciresult.FileRef) error {
	if err := ciresult.ValidateFileRef(name, &ref); err != nil {
		return err
	}
	if strings.TrimSpace(ref.SHA256) == "" {
		return fmt.Errorf("Sentinel capability plan %s sha256 is required", name)
	}
	return nil
}

func validatePolicyRef(name string, ref ciresult.FileRef) error {
	if err := validateFileRef(name, ref); err != nil {
		return err
	}
	return validateRelativePath(name, ref.Path)
}

func fileReference(root, path string) (ciresult.FileRef, error) {
	path = filepath.Clean(path)
	if err := validateRelativePath("policy", path); err != nil {
		return ciresult.FileRef{}, err
	}
	resolvedPath, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: path})
	if err != nil {
		return ciresult.FileRef{}, fmt.Errorf("resolve Sentinel policy %s: %w", path, err)
	}
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ciresult.FileRef{}, fmt.Errorf("read Sentinel policy %s: %w", path, err)
	}
	digest := sha256.Sum256(contents)
	return ciresult.FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}, nil
}

func validateRelativePath(name, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("Sentinel capability %s path must not be empty", name)
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("Sentinel capability %s path must stay inside the project root: %q", name, raw)
	}
	return nil
}

func WriteJSON(w io.Writer, p Plan) error {
	if err := p.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(p)
}

func SaveFile(path string, p Plan) error {
	if err := p.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Sentinel capability plan: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".sentinel-capability-output-*")
	if err != nil {
		return fmt.Errorf("create temporary Sentinel capability output in %s: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Sentinel capability output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Sentinel capability output permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Sentinel capability output: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Sentinel capability output: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Sentinel capability output %s: %w", path, err)
	}
	return nil
}

func LoadFile(path string) (Plan, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read Sentinel capability plan %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var plan Plan
	if err := decoder.Decode(&plan); err != nil {
		return Plan{}, fmt.Errorf("parse Sentinel capability plan %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Plan{}, fmt.Errorf("parse Sentinel capability plan %s: multiple JSON values are not supported", path)
		}
		return Plan{}, fmt.Errorf("parse Sentinel capability plan %s: %w", path, err)
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, fmt.Errorf("validate Sentinel capability plan %s: %w", path, err)
	}
	return plan, nil
}
