// Package campaign prepares deterministic mutation campaign inputs without
// applying a mutation or launching a subject process.
package campaign

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

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

// Schema is the versioned JSON shape for a ready-to-execute campaign plan.
const Schema = "ingen.mutation-plan/v1"

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CatalogueReference binds a plan to the exact catalogue bytes it consumed.
type CatalogueReference struct {
	Path    string `json:"path"`
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

// MutationEntry preserves declaration order as an explicit execution order.
type MutationEntry struct {
	Sequence int           `json:"sequence"`
	Spec     mutation.Spec `json:"spec"`
}

// Plan is the immutable input handoff between campaign review and a future
// mutation provider. The plan is not permission to edit a working tree.
type Plan struct {
	Schema              string                   `json:"schema"`
	Status              string                   `json:"status"`
	Catalogue           CatalogueReference       `json:"catalogue"`
	Contract            runner.ContractReference `json:"contract"`
	Oracle              runner.OracleReference   `json:"oracle"`
	Baseline            runner.BaselineReference `json:"baseline"`
	OraclePolicySHA256  string                   `json:"oracle_policy_sha256"`
	SubjectPolicySHA256 string                   `json:"subject_policy_sha256,omitempty"`
	Mutations           []MutationEntry          `json:"mutations"`
}

// BuildRequest supplies the already loaded and independently checked inputs
// needed to make a campaign plan.
type BuildRequest struct {
	CataloguePath       string
	CatalogueSHA256     string
	Catalogue           mutation.Catalogue
	Contract            contract.Document
	ContractSHA256      string
	Oracle              oracle.Artifact
	Baseline            runner.BaselineReference
	SubjectPolicySHA256 string
}

// Build validates all cross-artifact identities and creates a ready plan.
func Build(request BuildRequest) (Plan, error) {
	if strings.TrimSpace(request.CataloguePath) == "" {
		return Plan{}, fmt.Errorf("campaign catalogue path must not be empty")
	}
	if !digestPattern.MatchString(request.CatalogueSHA256) {
		return Plan{}, fmt.Errorf("campaign catalogue hash must be a lowercase SHA-256 digest")
	}
	if !digestPattern.MatchString(request.ContractSHA256) {
		return Plan{}, fmt.Errorf("campaign contract hash must be a lowercase SHA-256 digest")
	}
	if problems := mutation.ValidateAgainstContract(request.Catalogue, request.Contract); len(problems) > 0 {
		return Plan{}, fmt.Errorf("invalid campaign catalogue binding: %s", strings.Join(problems, "; "))
	}
	if problems := oracle.Validate(request.Oracle); len(problems) > 0 {
		return Plan{}, fmt.Errorf("invalid campaign oracle: %s", strings.Join(problems, "; "))
	}
	oracleHash, err := oracle.Hash(request.Oracle)
	if err != nil {
		return Plan{}, fmt.Errorf("hash campaign oracle: %w", err)
	}
	contractID, _ := request.Contract.Contract["id"].(string)
	contractVersion, ok := integer(request.Contract.Contract["version"])
	if !ok {
		return Plan{}, fmt.Errorf("campaign contract version must be an integer")
	}
	contractReference := runner.ContractReference{
		ID:      contractID,
		Version: contractVersion,
		SHA256:  request.ContractSHA256,
	}
	if request.Oracle.Contract.ID != contractID || request.Oracle.Contract.Version != contractVersion {
		return Plan{}, fmt.Errorf("campaign oracle contract %s@%d does not match contract %s@%d", request.Oracle.Contract.ID, request.Oracle.Contract.Version, contractID, contractVersion)
	}
	if request.Oracle.Contract.SHA256 != request.ContractSHA256 {
		return Plan{}, fmt.Errorf("campaign contract hash %q does not match frozen oracle contract hash %q", request.ContractSHA256, request.Oracle.Contract.SHA256)
	}
	oracleReference := runner.OracleReference{Schema: request.Oracle.Schema, SHA256: oracleHash}
	if strings.TrimSpace(request.Baseline.EvidencePath) == "" || strings.TrimSpace(request.Baseline.RunID) == "" {
		return Plan{}, fmt.Errorf("campaign baseline must contain an evidence path and run ID")
	}
	if request.Baseline.Contract != contractReference {
		return Plan{}, fmt.Errorf("campaign baseline contract does not match campaign contract")
	}
	if request.Baseline.Oracle == nil || *request.Baseline.Oracle != oracleReference {
		return Plan{}, fmt.Errorf("campaign baseline oracle does not match campaign oracle")
	}
	if strings.TrimSpace(request.SubjectPolicySHA256) != "" && !digestPattern.MatchString(request.SubjectPolicySHA256) {
		return Plan{}, fmt.Errorf("campaign subject policy hash must be a lowercase SHA-256 digest")
	}

	entries := make([]MutationEntry, 0, len(request.Catalogue.Mutations))
	for index, spec := range request.Catalogue.Mutations {
		entries = append(entries, MutationEntry{Sequence: index + 1, Spec: spec})
	}
	plan := Plan{
		Schema: Schema,
		Status: "ready",
		Catalogue: CatalogueReference{
			Path:    request.CataloguePath,
			ID:      request.Catalogue.ID,
			Version: request.Catalogue.Version,
			SHA256:  request.CatalogueSHA256,
		},
		Contract:            contractReference,
		Oracle:              oracleReference,
		Baseline:            request.Baseline,
		OraclePolicySHA256:  request.Oracle.PolicySHA256,
		SubjectPolicySHA256: request.SubjectPolicySHA256,
		Mutations:           entries,
	}
	if problems := Validate(plan); len(problems) > 0 {
		return Plan{}, fmt.Errorf("invalid campaign plan: %s", strings.Join(problems, "; "))
	}
	return plan, nil
}

// Validate returns structural and internal identity errors in a plan.
func Validate(plan Plan) []string {
	problems := make([]string, 0)
	if plan.Schema != Schema {
		problems = append(problems, fmt.Sprintf("plan.schema must be %s", Schema))
	}
	if plan.Status != "ready" {
		problems = append(problems, "plan.status must be ready")
	}
	if strings.TrimSpace(plan.Catalogue.Path) == "" {
		problems = append(problems, "plan.catalogue.path must be non-empty")
	}
	if strings.TrimSpace(plan.Catalogue.ID) == "" {
		problems = append(problems, "plan.catalogue.id must be non-empty")
	}
	if plan.Catalogue.Version < 1 {
		problems = append(problems, "plan.catalogue.version must be positive")
	}
	if !digestPattern.MatchString(plan.Catalogue.SHA256) {
		problems = append(problems, "plan.catalogue.sha256 must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(plan.Contract.ID) == "" {
		problems = append(problems, "plan.contract.id must be non-empty")
	}
	if plan.Contract.Version < 1 {
		problems = append(problems, "plan.contract.version must be positive")
	}
	if !digestPattern.MatchString(plan.Contract.SHA256) {
		problems = append(problems, "plan.contract.sha256 must be a lowercase SHA-256 digest")
	}
	if plan.Oracle.Schema != oracle.Schema {
		problems = append(problems, fmt.Sprintf("plan.oracle.schema must be %s", oracle.Schema))
	}
	if !digestPattern.MatchString(plan.Oracle.SHA256) {
		problems = append(problems, "plan.oracle.sha256 must be a lowercase SHA-256 digest")
	}
	if plan.Baseline.Contract != plan.Contract {
		problems = append(problems, "plan.baseline.contract must match plan.contract")
	}
	if plan.Baseline.Oracle == nil || *plan.Baseline.Oracle != plan.Oracle {
		problems = append(problems, "plan.baseline.oracle must match plan.oracle")
	}
	if strings.TrimSpace(plan.Baseline.EvidencePath) == "" {
		problems = append(problems, "plan.baseline.evidence_path must be non-empty")
	}
	if strings.TrimSpace(plan.Baseline.RunID) == "" {
		problems = append(problems, "plan.baseline.run_id must be non-empty")
	}
	if !digestPattern.MatchString(plan.OraclePolicySHA256) {
		problems = append(problems, "plan.oracle_policy_sha256 must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(plan.SubjectPolicySHA256) != "" && !digestPattern.MatchString(plan.SubjectPolicySHA256) {
		problems = append(problems, "plan.subject_policy_sha256 must be a lowercase SHA-256 digest")
	}
	if len(plan.Mutations) == 0 {
		problems = append(problems, "plan.mutations must contain at least one mutation")
	}
	specs := make([]mutation.Spec, 0, len(plan.Mutations))
	for index, entry := range plan.Mutations {
		if entry.Sequence != index+1 {
			problems = append(problems, fmt.Sprintf("plan.mutations[%d].sequence must be %d", index, index+1))
		}
		specs = append(specs, entry.Spec)
	}
	catalogue := mutation.Catalogue{
		Schema:          mutation.Schema,
		ID:              plan.Catalogue.ID,
		Version:         plan.Catalogue.Version,
		ContractID:      plan.Contract.ID,
		ContractVersion: plan.Contract.Version,
		Mutations:       specs,
	}
	for _, problem := range mutation.Validate(catalogue) {
		problems = append(problems, "plan."+strings.TrimPrefix(problem, "mutation_catalogue."))
	}
	return problems
}

// WriteFile writes a canonical plan and returns the exact SHA-256 hash of the
// bytes written. Use SemanticHash when comparing plan meaning across baseline
// executions.
func WriteFile(path string, plan Plan) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("campaign plan output path must not be empty")
	}
	contents, err := CanonicalJSON(plan)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return "", err
	}
	return HashBytes(contents), nil
}

// SemanticHash returns the stable identity of a plan's reviewable meaning.
// The selected baseline run ID is intentionally excluded because it is
// generated for each execution. The exact plan hash still includes that ID,
// and the serialized plan retains it for run-bound provenance.
func SemanticHash(plan Plan) (string, error) {
	if problems := Validate(plan); len(problems) > 0 {
		return "", fmt.Errorf("invalid campaign plan: %s", strings.Join(problems, "; "))
	}
	semantic := plan
	semantic.Baseline.RunID = ""
	contents, err := encodeCanonicalJSON(semantic)
	if err != nil {
		return "", err
	}
	return HashBytes(contents), nil
}

// LoadFile loads and verifies a canonical JSON campaign plan.
func LoadFile(path string) (Plan, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, err
	}
	return LoadBytes(path, contents)
}

// LoadBytes loads and verifies a canonical JSON campaign plan from already
// captured bytes. Callers that need provenance can use this to avoid a
// time-of-check/time-of-use gap between validation and execution.
func LoadBytes(path string, contents []byte) (Plan, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var plan Plan
	if err := decoder.Decode(&plan); err != nil {
		return Plan{}, fmt.Errorf("parse campaign plan %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Plan{}, fmt.Errorf("parse campaign plan %s: multiple JSON values are not supported", path)
		}
		return Plan{}, fmt.Errorf("parse campaign plan %s: %w", path, err)
	}
	if problems := Validate(plan); len(problems) > 0 {
		return Plan{}, fmt.Errorf("invalid campaign plan: %s", strings.Join(problems, "; "))
	}
	canonical, err := CanonicalJSON(plan)
	if err != nil {
		return Plan{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return Plan{}, fmt.Errorf("campaign plan %s is not canonical JSON", path)
	}
	return plan, nil
}

// CanonicalJSON returns deterministic JSON bytes for a valid plan.
func CanonicalJSON(plan Plan) ([]byte, error) {
	if problems := Validate(plan); len(problems) > 0 {
		return nil, fmt.Errorf("invalid campaign plan: %s", strings.Join(problems, "; "))
	}
	return encodeCanonicalJSON(plan)
}

func encodeCanonicalJSON(plan Plan) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(plan); err != nil {
		return nil, fmt.Errorf("canonicalize campaign plan: %w", err)
	}
	return buffer.Bytes(), nil
}

// HashFile returns the SHA-256 digest of an input file's exact bytes.
func HashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashBytes(contents), nil
}

// HashBytes returns the SHA-256 digest of exact input bytes.
func HashBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func integer(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		converted := int64(value)
		return converted, float64(converted) == value
	default:
		return 0, false
	}
}
