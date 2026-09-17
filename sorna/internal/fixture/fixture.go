// Package fixture verifies provider-supplied oracle fixture paths against a
// sealed Sorna contract without changing the portable contract artifact.
package fixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/sorna/internal/contract"
)

const (
	ProviderSchema = "ingen.fixture-provider/v1"
	HandoffSchema  = "ingen.fixture-handoff/v1"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ProviderManifest is the provider-supplied declaration of files available
// for oracle-owned contract fixtures. Paths are relative to the provider root
// supplied to Bind.
type ProviderManifest struct {
	Schema   string            `json:"schema" yaml:"schema"`
	ID       string            `json:"id" yaml:"id"`
	Version  int64             `json:"version" yaml:"version"`
	Fixtures []ProviderFixture `json:"fixtures" yaml:"fixtures"`
}

type ProviderFixture struct {
	ID     string `json:"id" yaml:"id"`
	Path   string `json:"path" yaml:"path"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

// Handoff records the verified relationship between a sealed contract and
// the provider files used for its oracle fixtures. It contains no fixture
// bytes and keeps paths relative to the provider root.
type Handoff struct {
	Schema   string            `json:"schema"`
	Status   string            `json:"status"`
	Contract ContractReference `json:"contract"`
	Provider ProviderReference `json:"provider"`
	Fixtures []BoundFixture    `json:"fixtures"`
}

type ContractReference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

type ProviderReference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

type BoundFixture struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type providerDocument struct {
	Provider ProviderManifest `json:"fixture_provider" yaml:"fixture_provider"`
}

// LoadProviderFile loads and validates a JSON, YAML, or YML provider manifest.
func LoadProviderFile(path string) (ProviderManifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ProviderManifest{}, err
	}
	return LoadProviderBytes(path, contents)
}

// LoadProviderBytes loads a provider manifest from already captured bytes.
func LoadProviderBytes(path string, contents []byte) (ProviderManifest, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return ProviderManifest{}, fmt.Errorf("fixture provider must use .json, .yaml, or .yml")
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
		return ProviderManifest{}, fmt.Errorf("invalid fixture provider: %s", strings.Join(problems, "; "))
	}
	return document.Provider, nil
}

// ValidateProvider returns structural errors in a fixture provider manifest.
func ValidateProvider(provider ProviderManifest) []string {
	problems := make([]string, 0)
	if provider.Schema != ProviderSchema {
		problems = append(problems, fmt.Sprintf("fixture_provider.schema must be %s", ProviderSchema))
	}
	if strings.TrimSpace(provider.ID) == "" {
		problems = append(problems, "fixture_provider.id must be non-empty")
	}
	if provider.Version < 1 {
		problems = append(problems, "fixture_provider.version must be positive")
	}
	if len(provider.Fixtures) == 0 {
		problems = append(problems, "fixture_provider.fixtures must contain at least one fixture")
	}
	seen := make(map[string]bool, len(provider.Fixtures))
	for index, fixture := range provider.Fixtures {
		path := fmt.Sprintf("fixture_provider.fixtures[%d]", index)
		if strings.TrimSpace(fixture.ID) == "" {
			problems = append(problems, path+".id must be non-empty")
		} else if seen[fixture.ID] {
			problems = append(problems, fmt.Sprintf("%s.id duplicates %q", path, fixture.ID))
		} else {
			seen[fixture.ID] = true
		}
		if !relativePath(fixture.Path) {
			problems = append(problems, path+".path must be relative and stay inside the provider root")
		}
		if !isDigest(fixture.SHA256) {
			problems = append(problems, path+".sha256 must be a lowercase SHA-256 digest")
		}
	}
	return problems
}

// CanonicalProviderJSON returns deterministic JSON bytes for a valid provider
// manifest. The wrapper is part of the versioned handoff shape.
func CanonicalProviderJSON(provider ProviderManifest) ([]byte, error) {
	if problems := ValidateProvider(provider); len(problems) > 0 {
		return nil, fmt.Errorf("invalid fixture provider: %s", strings.Join(problems, "; "))
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(providerDocument{Provider: provider}); err != nil {
		return nil, fmt.Errorf("canonicalize fixture provider: %w", err)
	}
	return buffer.Bytes(), nil
}

// Bind verifies every oracle-owned contract fixture against a provider file
// and returns a portable handoff artifact. The provider root is never written
// into the artifact; only the normalized relative paths are retained.
func Bind(document contract.Document, contractSHA256 string, provider ProviderManifest, root string) (Handoff, error) {
	if problems := ValidateProvider(provider); len(problems) > 0 {
		return Handoff{}, fmt.Errorf("invalid fixture provider: %s", strings.Join(problems, "; "))
	}
	if !isDigest(contractSHA256) {
		return Handoff{}, fmt.Errorf("contract SHA-256 must be a lowercase digest")
	}
	canonicalContract, err := contract.CanonicalJSON(document)
	if err != nil {
		return Handoff{}, err
	}
	if got := hashBytes(canonicalContract); got != contractSHA256 {
		return Handoff{}, fmt.Errorf("contract SHA-256 %q does not match canonical contract %q", contractSHA256, got)
	}
	if status, _ := document.Contract["status"].(string); status != "sealed" {
		return Handoff{}, fmt.Errorf("fixture binding requires a sealed contract, got %q", status)
	}

	contractFixtures, err := oracleFixtures(document)
	if err != nil {
		return Handoff{}, err
	}
	if len(contractFixtures) == 0 {
		return Handoff{}, fmt.Errorf("contract has no oracle fixtures to bind")
	}
	if len(provider.Fixtures) != len(contractFixtures) {
		return Handoff{}, fmt.Errorf("fixture provider has %d fixtures, contract declares %d", len(provider.Fixtures), len(contractFixtures))
	}

	rootPath, err := providerRoot(root)
	if err != nil {
		return Handoff{}, err
	}
	providerByID := make(map[string]ProviderFixture, len(provider.Fixtures))
	for _, item := range provider.Fixtures {
		providerByID[item.ID] = item
	}

	bound := make([]BoundFixture, 0, len(contractFixtures))
	seen := make(map[string]bool, len(contractFixtures))
	for index, declared := range contractFixtures {
		id := declared.id
		if seen[id] {
			return Handoff{}, fmt.Errorf("contract.fixtures[%d].id duplicates %q", index, id)
		}
		seen[id] = true
		if declared.owner != "oracle" {
			return Handoff{}, fmt.Errorf("contract.fixtures[%d].owner must be oracle for fixture binding", index)
		}
		provided, ok := providerByID[id]
		if !ok {
			return Handoff{}, fmt.Errorf("fixture provider has no entry for contract fixture %q", id)
		}
		if provided.SHA256 != declared.sha256 {
			return Handoff{}, fmt.Errorf("fixture %q provider digest %q does not match contract digest %q", id, provided.SHA256, declared.sha256)
		}
		path, err := resolveProviderPath(rootPath, provided.Path)
		if err != nil {
			return Handoff{}, fmt.Errorf("fixture %q: %w", id, err)
		}
		digest, size, err := fileDigest(path)
		if err != nil {
			return Handoff{}, fmt.Errorf("fixture %q: %w", id, err)
		}
		if digest != declared.sha256 {
			return Handoff{}, fmt.Errorf("fixture %q bytes digest %q does not match contract digest %q", id, digest, declared.sha256)
		}
		bound = append(bound, BoundFixture{ID: id, Path: normalizePath(provided.Path), SHA256: digest, Bytes: size})
	}

	providerBytes, err := CanonicalProviderJSON(provider)
	if err != nil {
		return Handoff{}, err
	}
	handoff := Handoff{
		Schema: HandoffSchema,
		Status: "verified",
		Contract: ContractReference{
			ID:      stringField(document.Contract["id"]),
			Version: integerField(document.Contract["version"]),
			SHA256:  contractSHA256,
		},
		Provider: ProviderReference{
			ID:      provider.ID,
			Version: provider.Version,
			SHA256:  hashBytes(providerBytes),
		},
		Fixtures: bound,
	}
	if problems := ValidateHandoff(handoff); len(problems) > 0 {
		return Handoff{}, fmt.Errorf("invalid fixture handoff: %s", strings.Join(problems, "; "))
	}
	return handoff, nil
}

// ValidateHandoff returns structural errors in a verified fixture handoff.
func ValidateHandoff(handoff Handoff) []string {
	problems := make([]string, 0)
	if handoff.Schema != HandoffSchema {
		problems = append(problems, fmt.Sprintf("schema must be %s", HandoffSchema))
	}
	if handoff.Status != "verified" {
		problems = append(problems, "status must be verified")
	}
	if strings.TrimSpace(handoff.Contract.ID) == "" {
		problems = append(problems, "contract.id must be non-empty")
	}
	if handoff.Contract.Version < 1 {
		problems = append(problems, "contract.version must be positive")
	}
	if !isDigest(handoff.Contract.SHA256) {
		problems = append(problems, "contract.sha256 must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(handoff.Provider.ID) == "" {
		problems = append(problems, "provider.id must be non-empty")
	}
	if handoff.Provider.Version < 1 {
		problems = append(problems, "provider.version must be positive")
	}
	if !isDigest(handoff.Provider.SHA256) {
		problems = append(problems, "provider.sha256 must be a lowercase SHA-256 digest")
	}
	if len(handoff.Fixtures) == 0 {
		problems = append(problems, "fixtures must contain at least one fixture")
	}
	seen := make(map[string]bool, len(handoff.Fixtures))
	for index, fixture := range handoff.Fixtures {
		path := fmt.Sprintf("fixtures[%d]", index)
		if strings.TrimSpace(fixture.ID) == "" {
			problems = append(problems, path+".id must be non-empty")
		} else if seen[fixture.ID] {
			problems = append(problems, fmt.Sprintf("%s.id duplicates %q", path, fixture.ID))
		} else {
			seen[fixture.ID] = true
		}
		if !relativePath(fixture.Path) {
			problems = append(problems, path+".path must be relative and stay inside the provider root")
		}
		if !isDigest(fixture.SHA256) {
			problems = append(problems, path+".sha256 must be a lowercase SHA-256 digest")
		}
		if fixture.Bytes < 0 {
			problems = append(problems, path+".bytes must not be negative")
		}
	}
	return problems
}

// CanonicalJSON returns deterministic JSON bytes for a valid handoff.
func CanonicalJSON(handoff Handoff) ([]byte, error) {
	if problems := ValidateHandoff(handoff); len(problems) > 0 {
		return nil, fmt.Errorf("invalid fixture handoff: %s", strings.Join(problems, "; "))
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(handoff); err != nil {
		return nil, fmt.Errorf("canonicalize fixture handoff: %w", err)
	}
	return buffer.Bytes(), nil
}

// WriteFile writes a canonical handoff JSON artifact and returns its hash.
func WriteFile(path string, handoff Handoff) (string, error) {
	contents, err := CanonicalJSON(handoff)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return "", err
	}
	return hashBytes(contents), nil
}

// LoadFile loads and validates a canonical handoff JSON artifact.
func LoadFile(path string) (Handoff, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Handoff{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var handoff Handoff
	if err := decoder.Decode(&handoff); err != nil {
		return Handoff{}, fmt.Errorf("parse fixture handoff %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Handoff{}, fmt.Errorf("parse fixture handoff %s: multiple JSON values are not supported", path)
		}
		return Handoff{}, fmt.Errorf("parse fixture handoff %s: %w", path, err)
	}
	if problems := ValidateHandoff(handoff); len(problems) > 0 {
		return Handoff{}, fmt.Errorf("invalid fixture handoff: %s", strings.Join(problems, "; "))
	}
	canonical, err := CanonicalJSON(handoff)
	if err != nil {
		return Handoff{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return Handoff{}, fmt.Errorf("fixture handoff %s is not canonical JSON", path)
	}
	return handoff, nil
}

type declaredFixture struct {
	id     string
	owner  string
	sha256 string
}

func oracleFixtures(document contract.Document) ([]declaredFixture, error) {
	value, ok := document.Contract["fixtures"]
	if !ok {
		return nil, fmt.Errorf("contract has no fixtures field")
	}
	fixtures, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("contract.fixtures must be a list")
	}
	declared := make([]declaredFixture, 0, len(fixtures))
	for index, value := range fixtures {
		fixture, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contract.fixtures[%d] must be an object", index)
		}
		id, ok := fixture["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("contract.fixtures[%d].id must be a non-empty string", index)
		}
		owner, _ := fixture["owner"].(string)
		sha, ok := fixture["sha256"].(string)
		if !ok || !isDigest(sha) {
			return nil, fmt.Errorf("contract.fixtures[%d].sha256 must be a lowercase SHA-256 digest", index)
		}
		declared = append(declared, declaredFixture{id: id, owner: owner, sha256: sha})
	}
	return declared, nil
}

func providerRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("provider root must not be empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve provider root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("stat provider root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("provider root must be a directory")
	}
	return filepath.EvalSymlinks(absolute)
}

func resolveProviderPath(root, relative string) (string, error) {
	if !relativePath(relative) {
		return "", fmt.Errorf("provider path must be relative and stay inside the provider root")
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve provider path: %w", err)
	}
	if !withinRoot(root, absolute) {
		return "", fmt.Errorf("provider path escapes the provider root")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve provider path: %w", err)
	}
	if !withinRoot(root, resolved) {
		return "", fmt.Errorf("provider path resolves outside the provider root")
	}
	return resolved, nil
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("provider path is not a regular file")
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func withinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "."
}

func relativePath(value string) bool {
	if strings.TrimSpace(value) == "" || strings.Contains(value, "\\") || filepath.IsAbs(value) {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func normalizePath(value string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
}

func isDigest(value string) bool {
	return digestPattern.MatchString(value)
}

func stringField(value any) string {
	result, _ := value.(string)
	return result
}

func integerField(value any) int64 {
	switch value := value.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case int32:
		return int64(value)
	case uint:
		return int64(value)
	case uint64:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func hashBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
