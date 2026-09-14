package campaign

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
// generated provider may also bind itself to the exact plan hash it consumed.
type ProviderManifest struct {
	Schema     string          `json:"schema" yaml:"schema"`
	ID         string          `json:"id" yaml:"id"`
	Version    int64           `json:"version" yaml:"version"`
	PlanSchema string          `json:"plan_schema" yaml:"plan_schema"`
	PlanSHA256 string          `json:"plan_sha256,omitempty" yaml:"plan_sha256,omitempty"`
	Entries    []ProviderEntry `json:"entries" yaml:"entries"`
}

// ProviderEntry describes one executable subject variant.
type ProviderEntry struct {
	MutationID  string   `json:"mutation_id" yaml:"mutation_id"`
	Command     string   `json:"command" yaml:"command"`
	Args        []string `json:"args,omitempty" yaml:"args,omitempty"`
	SubjectRoot string   `json:"subject_root,omitempty" yaml:"subject_root,omitempty"`
	SubjectDir  string   `json:"subject_dir,omitempty" yaml:"subject_dir,omitempty"`
	Variant     string   `json:"variant,omitempty" yaml:"variant,omitempty"`
}

// PreparedSubject is the resolved command handed to `sorna run`.
type PreparedSubject struct {
	Command     []string
	SubjectRoot string
	SubjectDir  string
	Variant     string
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
	}
	return problems
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
		return PreparedSubject{
			Command:     append([]string{entry.Command}, args...),
			SubjectRoot: subjectRoot,
			SubjectDir:  entry.SubjectDir,
			Variant:     variant,
		}, nil
	}
	return PreparedSubject{}, fmt.Errorf("provider has no entry for mutation %q", mutationID)
}
