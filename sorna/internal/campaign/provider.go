package campaign

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/sorna/internal/mutation"
)

// ProviderSchema is the versioned file shape for a provider's prepared
// subject declarations.
const ProviderSchema = "ingen.mutation-provider/v1"

const (
	addressToken = "${SORA_ADDR}"
	urlToken     = "${SORA_URL}"
)

// ProviderManifest maps plan mutation IDs to prepared subject commands. A
// provider may be backed by a compiler, source mutator, container builder, or
// prebuilt fixture; the campaign runner only consumes this boundary. A
// generated provider may also bind itself to the exact plan hash it consumed
// and report the stable semantic identity of that plan.
type ProviderManifest struct {
	Schema             string               `json:"schema" yaml:"schema"`
	ID                 string               `json:"id" yaml:"id"`
	Version            int64                `json:"version" yaml:"version"`
	PlanSchema         string               `json:"plan_schema" yaml:"plan_schema"`
	PlanSHA256         string               `json:"plan_sha256,omitempty" yaml:"plan_sha256,omitempty"`
	PlanSemanticSHA256 string               `json:"plan_semantic_sha256,omitempty" yaml:"plan_semantic_sha256,omitempty"`
	Capabilities       []ProviderCapability `json:"capabilities" yaml:"capabilities"`
	Entries            []ProviderEntry      `json:"entries" yaml:"entries"`
}

// ProviderCapability declares a mutation shape that the provider knows how
// to prepare. Target is included because the same operator may have different
// source-resolution rules for different public operations.
type ProviderCapability struct {
	Plane    string `json:"plane" yaml:"plane"`
	Operator string `json:"operator" yaml:"operator"`
	Target   string `json:"target" yaml:"target"`
}

// ProviderEntry describes one executable subject variant.
type ProviderEntry struct {
	MutationID  string              `json:"mutation_id" yaml:"mutation_id"`
	Command     string              `json:"command" yaml:"command"`
	Args        []string            `json:"args,omitempty" yaml:"args,omitempty"`
	SubjectRoot string              `json:"subject_root,omitempty" yaml:"subject_root,omitempty"`
	SubjectDir  string              `json:"subject_dir,omitempty" yaml:"subject_dir,omitempty"`
	Variant     string              `json:"variant,omitempty" yaml:"variant,omitempty"`
	Provenance  *ProviderProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

// ProviderProvenance records where a provider applied a mutation and which
// source and executable bytes it prepared. It is optional for legacy fixture
// providers, but source-level providers should populate every field.
type ProviderProvenance struct {
	SourceDir        string            `json:"source_dir" yaml:"source_dir"`
	SourceSHA256     string            `json:"source_sha256" yaml:"source_sha256"`
	BinarySHA256     string            `json:"binary_sha256" yaml:"binary_sha256"`
	Location         string            `json:"location" yaml:"location"`
	Before           string            `json:"before" yaml:"before"`
	After            string            `json:"after" yaml:"after"`
	TargetResolution *TargetResolution `json:"target_resolution,omitempty" yaml:"target_resolution,omitempty"`
}

// TargetResolution records the provider's deterministic source-target
// selection. A successful source mutation should report exactly one candidate
// and one applied target; the counts make that decision reviewable without
// requiring a consumer to inspect source code.
type TargetResolution struct {
	Selector       string `json:"selector" yaml:"selector"`
	CandidateCount int    `json:"candidate_count" yaml:"candidate_count"`
	AppliedCount   int    `json:"applied_count" yaml:"applied_count"`
}

// TargetResolutionError is returned when a provider cannot select exactly one
// source target for a planned mutation. It is both human-readable through
// Error and machine-readable through its exported fields and JSON tags.
type TargetResolutionError struct {
	Schema     string           `json:"schema"`
	Status     string           `json:"status"`
	MutationID string           `json:"mutation_id"`
	Plane      string           `json:"plane"`
	Operator   string           `json:"operator"`
	Target     string           `json:"target"`
	Resolution TargetResolution `json:"resolution"`
}

// TargetResolutionErrorSchema identifies the structured preparation error
// emitted when source-target selection is not unique.
const TargetResolutionErrorSchema = "ingen.mutation-target-resolution-error/v1"

func NewTargetResolutionError(spec mutation.Spec, selector string, candidateCount int) *TargetResolutionError {
	return &TargetResolutionError{
		Schema:     TargetResolutionErrorSchema,
		Status:     "blocked",
		MutationID: spec.ID,
		Plane:      spec.Plane,
		Operator:   spec.Operator,
		Target:     spec.Target,
		Resolution: TargetResolution{
			Selector:       selector,
			CandidateCount: candidateCount,
			AppliedCount:   0,
		},
	}
}

func (e *TargetResolutionError) Error() string {
	return fmt.Sprintf("mutation %q target resolution for %s found %d candidate(s), applied %d; expected exactly one", e.MutationID, e.Resolution.Selector, e.Resolution.CandidateCount, e.Resolution.AppliedCount)
}

// PreparedSubject is the resolved command handed to `sorna run`.
type PreparedSubject struct {
	Command     []string
	SubjectRoot string
	SubjectDir  string
	Variant     string
	Provenance  *ProviderProvenance
}

type providerDocument struct {
	Provider ProviderManifest `json:"mutation_provider" yaml:"mutation_provider"`
}

// LoadProviderFile loads and validates a YAML or JSON provider manifest.
func LoadProviderFile(path string) (ProviderManifest, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return ProviderManifest{}, fmt.Errorf("mutation provider must use .json, .yaml, or .yml")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ProviderManifest{}, err
	}
	return LoadProviderBytes(path, contents)
}

// LoadProviderBytes loads a provider manifest from already captured bytes so
// validation and later provenance can share one immutable input.
func LoadProviderBytes(path string, contents []byte) (ProviderManifest, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return ProviderManifest{}, fmt.Errorf("mutation provider must use .json, .yaml, or .yml")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var document providerDocument
	if err := decoder.Decode(&document); err != nil {
		return ProviderManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ProviderManifest{}, fmt.Errorf("parse %s: multiple documents are not supported", path)
		}
		return ProviderManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if problems := ValidateProvider(document.Provider); len(problems) > 0 {
		return ProviderManifest{}, fmt.Errorf("invalid mutation provider: %s", strings.Join(problems, "; "))
	}
	return document.Provider, nil
}

// ValidateProvider returns structural errors in the provider manifest.
func ValidateProvider(provider ProviderManifest) []string {
	problems := make([]string, 0)
	if provider.Schema != ProviderSchema {
		problems = append(problems, fmt.Sprintf("mutation_provider.schema must be %s", ProviderSchema))
	}
	if strings.TrimSpace(provider.ID) == "" {
		problems = append(problems, "mutation_provider.id must be non-empty")
	}
	if provider.Version < 1 {
		problems = append(problems, "mutation_provider.version must be positive")
	}
	if provider.PlanSchema != Schema {
		problems = append(problems, fmt.Sprintf("mutation_provider.plan_schema must be %s", Schema))
	}
	if strings.TrimSpace(provider.PlanSHA256) != "" && !digestPattern.MatchString(provider.PlanSHA256) {
		problems = append(problems, "mutation_provider.plan_sha256 must be a lowercase SHA-256 digest when present")
	}
	if strings.TrimSpace(provider.PlanSemanticSHA256) != "" && !digestPattern.MatchString(provider.PlanSemanticSHA256) {
		problems = append(problems, "mutation_provider.plan_semantic_sha256 must be a lowercase SHA-256 digest when present")
	}
	if len(provider.Capabilities) == 0 {
		problems = append(problems, "mutation_provider.capabilities must contain at least one capability")
	}
	seenCapabilities := make(map[string]bool, len(provider.Capabilities))
	for index, capability := range provider.Capabilities {
		path := fmt.Sprintf("mutation_provider.capabilities[%d]", index)
		if strings.TrimSpace(capability.Plane) == "" {
			problems = append(problems, path+".plane must be non-empty")
		}
		if strings.TrimSpace(capability.Operator) == "" {
			problems = append(problems, path+".operator must be non-empty")
		}
		if strings.TrimSpace(capability.Target) == "" {
			problems = append(problems, path+".target must be non-empty")
		}
		key := capabilityKey(capability.Plane, capability.Operator, capability.Target)
		if seenCapabilities[key] {
			problems = append(problems, fmt.Sprintf("%s duplicates a declared capability", path))
		} else {
			seenCapabilities[key] = true
		}
	}
	if len(provider.Entries) == 0 {
		problems = append(problems, "mutation_provider.entries must contain at least one entry")
	}
	seen := make(map[string]bool, len(provider.Entries))
	for index, entry := range provider.Entries {
		path := fmt.Sprintf("mutation_provider.entries[%d]", index)
		if strings.TrimSpace(entry.MutationID) == "" {
			problems = append(problems, path+".mutation_id must be non-empty")
		} else if seen[entry.MutationID] {
			problems = append(problems, fmt.Sprintf("%s.mutation_id duplicates %q", path, entry.MutationID))
		} else {
			seen[entry.MutationID] = true
		}
		if strings.TrimSpace(entry.Command) == "" {
			problems = append(problems, path+".command must be non-empty")
		}
		if entry.Provenance != nil {
			provenance := entry.Provenance
			if strings.TrimSpace(provenance.SourceDir) == "" {
				problems = append(problems, path+".provenance.source_dir must be non-empty")
			} else if !providerRelativePath(provenance.SourceDir) {
				problems = append(problems, path+".provenance.source_dir must be relative and stay inside the subject root")
			}
			if !digestPattern.MatchString(provenance.SourceSHA256) {
				problems = append(problems, path+".provenance.source_sha256 must be a lowercase SHA-256 digest")
			}
			if !digestPattern.MatchString(provenance.BinarySHA256) {
				problems = append(problems, path+".provenance.binary_sha256 must be a lowercase SHA-256 digest")
			}
			if strings.TrimSpace(provenance.Location) == "" {
				problems = append(problems, path+".provenance.location must be non-empty")
			}
			if strings.TrimSpace(provenance.Before) == "" {
				problems = append(problems, path+".provenance.before must be non-empty")
			}
			if strings.TrimSpace(provenance.After) == "" {
				problems = append(problems, path+".provenance.after must be non-empty")
			}
			if resolution := provenance.TargetResolution; resolution != nil {
				if strings.TrimSpace(resolution.Selector) == "" {
					problems = append(problems, path+".provenance.target_resolution.selector must be non-empty")
				}
				if resolution.CandidateCount < 0 {
					problems = append(problems, path+".provenance.target_resolution.candidate_count must not be negative")
				}
				if resolution.AppliedCount < 0 {
					problems = append(problems, path+".provenance.target_resolution.applied_count must not be negative")
				}
				if resolution.AppliedCount > resolution.CandidateCount {
					problems = append(problems, path+".provenance.target_resolution.applied_count must not exceed candidate_count")
				}
			}
		}
		for argumentIndex, argument := range entry.Args {
			remaining := strings.ReplaceAll(strings.ReplaceAll(argument, addressToken, ""), urlToken, "")
			if strings.Contains(remaining, "${") {
				problems = append(problems, fmt.Sprintf("%s.args[%d] contains an unsupported template token", path, argumentIndex))
			}
		}
	}
	return problems
}

// ValidateForPlan ensures the provider has a prepared entry for every
// mutation in the plan. Extra entries are allowed so one provider can support
// several plans without changing its manifest.
func (provider ProviderManifest) ValidateForPlan(plan Plan) []string {
	problems := make([]string, 0)
	entries := make(map[string]ProviderEntry, len(provider.Entries))
	for _, entry := range provider.Entries {
		entries[entry.MutationID] = entry
	}
	for index, mutation := range plan.Mutations {
		if _, ok := entries[mutation.Spec.ID]; !ok {
			problems = append(problems, fmt.Sprintf("plan.mutations[%d] %q has no provider entry", index, mutation.Spec.ID))
		}
		if !provider.supports(mutation.Spec) {
			problems = append(problems, fmt.Sprintf("plan.mutations[%d] %q uses undeclared provider capability %s", index, mutation.Spec.ID, capabilityKey(mutation.Spec.Plane, mutation.Spec.Operator, mutation.Spec.Target)))
		}
	}
	return problems
}

// Supports reports whether the provider declares the exact plane, operator,
// and target shape of a mutation specification.
func (provider ProviderManifest) Supports(spec mutation.Spec) bool {
	return provider.supports(spec)
}

func (provider ProviderManifest) supports(spec mutation.Spec) bool {
	wanted := capabilityKey(spec.Plane, spec.Operator, spec.Target)
	for _, capability := range provider.Capabilities {
		if capabilityKey(capability.Plane, capability.Operator, capability.Target) == wanted {
			return true
		}
	}
	return false
}

func capabilityKey(plane, operator, target string) string {
	return plane + "|" + operator + "|" + target
}

// Resolve prepares the command for one mutation and expands only the two
// explicit runtime tokens. Arguments remain an argv array; no shell parsing
// or interpolation is performed.
func (provider ProviderManifest) Resolve(mutationID, address, baseURL string) (PreparedSubject, error) {
	if strings.TrimSpace(address) == "" || strings.TrimSpace(baseURL) == "" {
		return PreparedSubject{}, fmt.Errorf("provider address and URL must be non-empty")
	}
	for _, entry := range provider.Entries {
		if entry.MutationID != mutationID {
			continue
		}
		args := make([]string, len(entry.Args))
		for index, argument := range entry.Args {
			args[index] = strings.ReplaceAll(strings.ReplaceAll(argument, addressToken, address), urlToken, baseURL)
		}
		variant := entry.Variant
		if strings.TrimSpace(variant) == "" {
			variant = mutationID
		}
		subjectRoot := entry.SubjectRoot
		if strings.TrimSpace(subjectRoot) == "" {
			subjectRoot = "."
		}
		var provenance *ProviderProvenance
		if entry.Provenance != nil {
			copy := *entry.Provenance
			provenance = &copy
		}
		return PreparedSubject{
			Command:     append([]string{entry.Command}, args...),
			SubjectRoot: subjectRoot,
			SubjectDir:  entry.SubjectDir,
			Variant:     variant,
			Provenance:  provenance,
		}, nil
	}
	return PreparedSubject{}, fmt.Errorf("provider has no entry for mutation %q", mutationID)
}

// VerifyPreparedSubject checks the immutable source and executable identities
// recorded by a source-level provider before the subject is launched.
// Legacy fixture entries without provenance remain valid and are not hashed.
func VerifyPreparedSubject(prepared PreparedSubject) error {
	if prepared.Provenance == nil {
		return nil
	}
	if len(prepared.Command) == 0 {
		return fmt.Errorf("prepared subject has provenance but no command")
	}
	if !providerRelativePath(prepared.Provenance.SourceDir) {
		return fmt.Errorf("prepared source provenance path must stay inside the subject root")
	}
	root, err := filepath.Abs(prepared.SubjectRoot)
	if err != nil {
		return fmt.Errorf("resolve prepared subject root: %w", err)
	}
	sourcePath := filepath.Join(root, filepath.FromSlash(prepared.Provenance.SourceDir))
	sourceHash, err := HashTree(sourcePath)
	if err != nil {
		return fmt.Errorf("hash prepared source tree: %w", err)
	}
	if sourceHash != prepared.Provenance.SourceSHA256 {
		return fmt.Errorf("prepared source hash %q does not match provenance %q", sourceHash, prepared.Provenance.SourceSHA256)
	}

	binaryPath := prepared.Command[0]
	if !filepath.IsAbs(binaryPath) {
		if !providerRelativePath(binaryPath) {
			return fmt.Errorf("prepared command path must stay inside the subject root")
		}
		binaryPath = filepath.Join(root, filepath.FromSlash(binaryPath))
	}
	binaryHash, err := HashFile(binaryPath)
	if err != nil {
		return fmt.Errorf("hash prepared subject binary: %w", err)
	}
	if binaryHash != prepared.Provenance.BinarySHA256 {
		return fmt.Errorf("prepared binary hash %q does not match provenance %q", binaryHash, prepared.Provenance.BinarySHA256)
	}
	return nil
}

func providerRelativePath(path string) bool {
	clean := filepath.Clean(path)
	return clean != "." && !filepath.IsAbs(path) && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
