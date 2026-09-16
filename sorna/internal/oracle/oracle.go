// Package oracle creates the deterministic, frozen input/expectation artifact
// that a later Sorna runner can consume without reopening the contract source.
package oracle

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
	"strconv"
	"strings"

	"ingen/sorna/internal/contract"
)

const Schema = "ingen.oracle/v1"

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ContractReference binds a frozen oracle to the exact sealed contract that
// produced it.
type ContractReference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

// Case is a materialized contract rule. The oracle stores the concrete given
// values so a later evaluator does not need to reinterpret generators.
type Case struct {
	CaseID   string         `json:"case_id"`
	RuleID   string         `json:"rule_id"`
	Strength string         `json:"strength"`
	Subject  string         `json:"subject,omitempty"`
	Given    map[string]any `json:"given,omitempty"`
	Expect   map[string]any `json:"expect,omitempty"`
}

// Artifact is the immutable JSON artifact produced by an oracle generation
// process. It intentionally contains no subject observations.
type Artifact struct {
	Schema       string            `json:"schema"`
	Status       string            `json:"status"`
	Contract     ContractReference `json:"contract"`
	PolicySHA256 string            `json:"policy_sha256"`
	Cases        []Case            `json:"cases"`
}

// Generate materializes every contract rule into a deterministic oracle
// artifact. The policy hash is supplied by the orchestrator so the result is
// bound to the capability policy used to generate it.
func Generate(sealed contract.Sealed, policySHA256 string) (Artifact, error) {
	if sealed.SHA256 == "" || !digestPattern.MatchString(sealed.SHA256) {
		return Artifact{}, fmt.Errorf("sealed contract must contain a valid SHA-256 hash")
	}
	if !digestPattern.MatchString(policySHA256) {
		return Artifact{}, fmt.Errorf("oracle policy hash must be a lowercase SHA-256 digest")
	}
	if problems := contract.Validate(sealed.Document); len(problems) > 0 {
		return Artifact{}, fmt.Errorf("oracle contract is invalid: %s", strings.Join(problems, "; "))
	}

	contractID, _ := sealed.Document.Contract["id"].(string)
	version, ok := integer(sealed.Document.Contract["version"])
	if !ok {
		return Artifact{}, fmt.Errorf("oracle contract version must be an integer")
	}
	rules, ok := sealed.Document.Contract["rules"].([]any)
	if !ok {
		return Artifact{}, fmt.Errorf("oracle contract rules must be a list")
	}

	cases := make([]Case, 0, len(rules))
	for index, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return Artifact{}, fmt.Errorf("oracle contract rule %d must be an object", index)
		}
		ruleID, _ := rule["id"].(string)
		strength, _ := rule["strength"].(string)
		subject, _ := rule["subject"].(string)
		generatedGiven, err := materializeGiven(rule["given"])
		if err != nil {
			return Artifact{}, fmt.Errorf("materialize rule %q: %w", ruleID, err)
		}
		expect, _ := rule["expect"].(map[string]any)
		cases = append(cases, Case{
			CaseID:   fmt.Sprintf("case-%04d", index+1),
			RuleID:   ruleID,
			Strength: strength,
			Subject:  subject,
			Given:    generatedGiven,
			Expect:   expect,
		})
	}

	artifact := Artifact{
		Schema: Schema,
		Status: "frozen",
		Contract: ContractReference{
			ID:      contractID,
			Version: version,
			SHA256:  sealed.SHA256,
		},
		PolicySHA256: policySHA256,
		Cases:        cases,
	}
	if problems := Validate(artifact); len(problems) > 0 {
		return Artifact{}, fmt.Errorf("generated oracle is invalid: %s", strings.Join(problems, "; "))
	}
	return artifact, nil
}

// Validate returns structural errors in the frozen oracle shape.
func Validate(artifact Artifact) []string {
	problems := make([]string, 0)
	if artifact.Schema != Schema {
		problems = append(problems, fmt.Sprintf("oracle.schema must be %s", Schema))
	}
	if artifact.Status != "frozen" {
		problems = append(problems, "oracle.status must be frozen")
	}
	if strings.TrimSpace(artifact.Contract.ID) == "" {
		problems = append(problems, "oracle.contract.id must be non-empty")
	}
	if artifact.Contract.Version < 1 {
		problems = append(problems, "oracle.contract.version must be positive")
	}
	if !digestPattern.MatchString(artifact.Contract.SHA256) {
		problems = append(problems, "oracle.contract.sha256 must be a lowercase SHA-256 digest")
	}
	if !digestPattern.MatchString(artifact.PolicySHA256) {
		problems = append(problems, "oracle.policy_sha256 must be a lowercase SHA-256 digest")
	}
	if len(artifact.Cases) == 0 {
		problems = append(problems, "oracle.cases must contain at least one case")
	}
	seenCaseIDs := make(map[string]bool, len(artifact.Cases))
	seenRuleIDs := make(map[string]bool, len(artifact.Cases))
	allowedStrengths := map[string]bool{
		"must": true, "must_not": true, "may": true, "should": true, "unspecified": true,
	}
	for index, item := range artifact.Cases {
		path := fmt.Sprintf("oracle.cases[%d]", index)
		if strings.TrimSpace(item.CaseID) == "" {
			problems = append(problems, path+".case_id must be non-empty")
		} else if seenCaseIDs[item.CaseID] {
			problems = append(problems, fmt.Sprintf("%s.case_id duplicates %q", path, item.CaseID))
		} else {
			seenCaseIDs[item.CaseID] = true
		}
		if strings.TrimSpace(item.RuleID) == "" {
			problems = append(problems, path+".rule_id must be non-empty")
		} else if seenRuleIDs[item.RuleID] {
			problems = append(problems, fmt.Sprintf("%s.rule_id duplicates %q", path, item.RuleID))
		} else {
			seenRuleIDs[item.RuleID] = true
		}
		if strings.TrimSpace(item.Strength) == "" {
			problems = append(problems, path+".strength must be non-empty")
		} else if !allowedStrengths[item.Strength] {
			problems = append(problems, path+".strength is invalid")
		}
	}
	return problems
}

// CanonicalJSON returns deterministic bytes for a valid oracle artifact.
func CanonicalJSON(artifact Artifact) ([]byte, error) {
	if problems := Validate(artifact); len(problems) > 0 {
		return nil, fmt.Errorf("invalid oracle: %s", strings.Join(problems, "; "))
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(artifact); err != nil {
		return nil, fmt.Errorf("canonicalize oracle: %w", err)
	}
	return buffer.Bytes(), nil
}

// WriteFile writes a canonical frozen oracle artifact and returns its SHA-256
// hash. The parent directory is created before the artifact is written.
func WriteFile(path string, artifact Artifact) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("oracle output path must not be empty")
	}
	contents, err := CanonicalJSON(artifact)
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

// LoadFile loads and validates a canonical frozen oracle artifact.
func LoadFile(path string) (Artifact, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var artifact Artifact
	if err := decoder.Decode(&artifact); err != nil {
		return Artifact{}, fmt.Errorf("parse oracle %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Artifact{}, fmt.Errorf("parse oracle %s: multiple JSON values are not supported", path)
		}
		return Artifact{}, fmt.Errorf("parse oracle %s: %w", path, err)
	}
	if problems := Validate(artifact); len(problems) > 0 {
		return Artifact{}, fmt.Errorf("invalid oracle: %s", strings.Join(problems, "; "))
	}
	canonical, err := CanonicalJSON(artifact)
	if err != nil {
		return Artifact{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return Artifact{}, fmt.Errorf("oracle %s is not canonical JSON", path)
	}
	return artifact, nil
}

// Hash returns the SHA-256 digest of an artifact's canonical bytes.
func Hash(artifact Artifact) (string, error) {
	contents, err := CanonicalJSON(artifact)
	if err != nil {
		return "", err
	}
	return hashBytes(contents), nil
}

// HashFile returns the SHA-256 digest of an oracle file's bytes.
func HashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(contents), nil
}

func materializeGiven(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	materialized, err := contract.Materialize(value)
	if err != nil {
		return nil, err
	}
	given, ok := materialized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("given must be an object")
	}
	return given, nil
}

func hashBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func integer(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case json.Number:
		parsed, err := strconv.ParseInt(string(value), 10, 64)
		return parsed, err == nil
	case float64:
		parsed := int64(value)
		return parsed, value == float64(parsed)
	default:
		return 0, false
	}
}
