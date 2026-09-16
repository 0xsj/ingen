package policytest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/paddock/internal/checker"
	"ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
	"ingen/paddock/internal/policy"
)

const (
	ManifestSchema = "paddock.policy-tests/v1"
	DocumentSchema = "paddock.policy-test-result/v1"
)

type Manifest struct {
	Schema string `yaml:"schema" json:"schema"`
	Cases  []Case `yaml:"cases" json:"cases"`
}

type Case struct {
	Name         string   `yaml:"name" json:"name"`
	Root         string   `yaml:"root" json:"root"`
	Expect       string   `yaml:"expect" json:"expect"`
	RequireRules []string `yaml:"require_rules,omitempty" json:"require_rules,omitempty"`
}

type Document struct {
	Schema   string       `json:"schema"`
	Policy   string       `json:"policy"`
	Manifest FileRef      `json:"manifest"`
	Adapter  *Adapter     `json:"adapter,omitempty"`
	Status   string       `json:"status"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	Cases    []CaseResult `json:"cases"`
}

type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Adapter struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

type CaseResult struct {
	Name         string   `json:"name"`
	Root         string   `json:"root"`
	Expected     string   `json:"expected"`
	Actual       string   `json:"actual"`
	Status       string   `json:"status"`
	Findings     int      `json:"findings,omitempty"`
	FindingRules []string `json:"finding_rules,omitempty"`
	Error        string   `json:"error,omitempty"`
	MissingRules []string `json:"missing_rules,omitempty"`
}

type Options struct {
	AdapterExecutable string
	AdapterArgs       []string
}

func (d Document) Validate() error {
	if d.Schema != DocumentSchema {
		return fmt.Errorf("policy test result schema must be %s, got %q", DocumentSchema, d.Schema)
	}
	if d.Policy == "" {
		return fmt.Errorf("policy test result policy is required")
	}
	if d.Manifest.Path == "" || !isSHA256(d.Manifest.SHA256) {
		return fmt.Errorf("policy test result manifest must include a path and SHA-256 digest")
	}
	if d.Status != "PASS" && d.Status != "FAIL" {
		return fmt.Errorf("policy test result has unsupported status %q", d.Status)
	}
	if d.Passed < 0 || d.Failed < 0 || d.Passed+d.Failed != len(d.Cases) || len(d.Cases) == 0 {
		return fmt.Errorf("policy test result counts do not match cases")
	}
	if (d.Failed == 0 && d.Status != "PASS") || (d.Failed > 0 && d.Status != "FAIL") {
		return fmt.Errorf("policy test result status does not match failed case count")
	}
	if d.Adapter != nil && d.Adapter.Executable == "" {
		return fmt.Errorf("policy test result adapter executable is required")
	}
	for _, testCase := range d.Cases {
		if testCase.Name == "" || testCase.Root == "" {
			return fmt.Errorf("policy test result cases require name and root")
		}
		if !validCaseOutcome(testCase.Expected) || !validCaseOutcome(testCase.Actual) {
			return fmt.Errorf("policy test result case %q has an unsupported outcome", testCase.Name)
		}
		if testCase.Status != "PASS" && testCase.Status != "FAIL" {
			return fmt.Errorf("policy test result case %q has an unsupported status %q", testCase.Name, testCase.Status)
		}
		if testCase.Findings < 0 {
			return fmt.Errorf("policy test result case %q has a negative finding count", testCase.Name)
		}
		wantPass := testCase.Expected == testCase.Actual && len(testCase.MissingRules) == 0
		if (testCase.Status == "PASS") != wantPass {
			return fmt.Errorf("policy test result case %q status does not match its outcome", testCase.Name)
		}
		if err := validateRuleIDs(testCase.Name, "finding_rules", testCase.FindingRules); err != nil {
			return err
		}
		if err := validateRuleIDs(testCase.Name, "missing_rules", testCase.MissingRules); err != nil {
			return err
		}
	}
	return nil
}

func validCaseOutcome(value string) bool {
	switch value {
	case "pass", "fail", "error":
		return true
	default:
		return false
	}
}

func validateRuleIDs(caseName, field string, ruleIDs []string) error {
	seen := make(map[string]struct{}, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		if ruleID == "" {
			return fmt.Errorf("policy test result case %q %s cannot contain empty values", caseName, field)
		}
		if _, ok := seen[ruleID]; ok {
			return fmt.Errorf("policy test result case %q %s contains duplicate rule %q", caseName, field, ruleID)
		}
		seen[ruleID] = struct{}{}
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read policy test manifest: %w", err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse policy test manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.Schema != ManifestSchema {
		return fmt.Errorf("policy test manifest schema must be %s, got %q", ManifestSchema, m.Schema)
	}
	if len(m.Cases) == 0 {
		return fmt.Errorf("policy test manifest cases must not be empty")
	}
	seen := make(map[string]struct{}, len(m.Cases))
	for index, testCase := range m.Cases {
		if testCase.Name == "" {
			return fmt.Errorf("policy test case %d name is required", index+1)
		}
		if _, exists := seen[testCase.Name]; exists {
			return fmt.Errorf("duplicate policy test case %q", testCase.Name)
		}
		seen[testCase.Name] = struct{}{}
		if testCase.Root == "" {
			return fmt.Errorf("policy test case %q root is required", testCase.Name)
		}
		switch testCase.Expect {
		case "pass", "fail", "error":
		default:
			return fmt.Errorf("policy test case %q expect must be pass, fail, or error", testCase.Name)
		}
		requiredRules := make(map[string]struct{}, len(testCase.RequireRules))
		for _, ruleID := range testCase.RequireRules {
			if ruleID == "" {
				return fmt.Errorf("policy test case %q require_rules cannot contain empty values", testCase.Name)
			}
			if _, exists := requiredRules[ruleID]; exists {
				return fmt.Errorf("policy test case %q requires duplicate rule %q", testCase.Name, ruleID)
			}
			requiredRules[ruleID] = struct{}{}
		}
	}
	return nil
}

func Run(manifestPath, policyPath string, config policy.Policy) (Document, error) {
	return RunWithOptions(manifestPath, policyPath, config, Options{})
}

func RunWithOptions(manifestPath, policyPath string, config policy.Policy, options Options) (Document, error) {
	absoluteManifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return Document{}, fmt.Errorf("resolve policy test manifest: %w", err)
	}
	manifest, err := Load(absoluteManifestPath)
	if err != nil {
		return Document{}, err
	}
	manifestRef, err := fileRef(absoluteManifestPath)
	if err != nil {
		return Document{}, err
	}
	base := filepath.Dir(absoluteManifestPath)
	document := Document{
		Schema:   DocumentSchema,
		Policy:   policyPath,
		Manifest: manifestRef,
		Status:   "PASS",
		Cases:    make([]CaseResult, 0, len(manifest.Cases)),
	}
	if options.AdapterExecutable != "" {
		document.Adapter = &Adapter{Executable: options.AdapterExecutable, Args: append([]string(nil), options.AdapterArgs...)}
	}
	for _, testCase := range manifest.Cases {
		root := testCase.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(base, root)
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return Document{}, fmt.Errorf("resolve policy test case %q root: %w", testCase.Name, err)
		}
		caseResult := CaseResult{
			Name:     testCase.Name,
			Root:     root,
			Expected: testCase.Expect,
		}
		result, checkErr := checkCase(root, policyPath, config, options)
		switch {
		case checkErr != nil:
			caseResult.Actual = "error"
			caseResult.Error = checkErr.Error()
		case result.OK():
			caseResult.Actual = "pass"
			caseResult.Findings = len(result.Findings)
		default:
			caseResult.Actual = "fail"
			caseResult.Findings = len(result.Findings)
		}
		if result != nil {
			foundRules := make(map[string]struct{}, len(result.Findings))
			for _, finding := range result.Findings {
				foundRules[finding.RuleID] = struct{}{}
			}
			caseResult.FindingRules = make([]string, 0, len(foundRules))
			for ruleID := range foundRules {
				caseResult.FindingRules = append(caseResult.FindingRules, ruleID)
			}
			sort.Strings(caseResult.FindingRules)
			for _, ruleID := range testCase.RequireRules {
				if _, found := foundRules[ruleID]; !found {
					caseResult.MissingRules = append(caseResult.MissingRules, ruleID)
				}
			}
		}
		if caseResult.Actual == testCase.Expect && len(caseResult.MissingRules) == 0 {
			caseResult.Status = "PASS"
			document.Passed++
		} else {
			caseResult.Status = "FAIL"
			document.Failed++
			document.Status = "FAIL"
		}
		document.Cases = append(document.Cases, caseResult)
	}
	return document, nil
}

func fileRef(path string) (FileRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileRef{Path: path}, fmt.Errorf("hash policy test manifest: %w", err)
	}
	digest := sha256.Sum256(data)
	return FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}, nil
}

func checkCase(root, policyPath string, config policy.Policy, options Options) (*model.Result, error) {
	if options.AdapterExecutable == "" {
		return checker.CheckPolicy(root, policyPath, config)
	}
	requiredCapabilities := graph.Capabilities{}
	if config.Source.Unit != "" {
		requiredCapabilities.SourceUnits = []string{config.Source.Unit}
	}
	dependencyGraph, _, err := graph.LoadExternal(context.Background(), options.AdapterExecutable, options.AdapterArgs, graph.Request{
		Schema:               graph.RequestSchema,
		Language:             config.Source.Language,
		Unit:                 config.Source.Unit,
		Root:                 root,
		Roots:                append([]string(nil), config.Source.Roots...),
		Include:              append([]string(nil), config.Source.Include...),
		Exclude:              append([]string(nil), config.Source.Exclude...),
		RequiredCapabilities: requiredCapabilities,
	})
	if err != nil {
		return nil, err
	}
	return checker.CheckGraph(root, policyPath, config, dependencyGraph)
}

func Text(w io.Writer, document Document) error {
	if _, err := fmt.Fprintf(w, "POLICY-TEST %s (%d/%d passed)\n", document.Status, document.Passed, len(document.Cases)); err != nil {
		return err
	}
	for _, testCase := range document.Cases {
		detail := fmt.Sprintf("expected %s, got %s", testCase.Expected, testCase.Actual)
		if testCase.Findings > 0 {
			detail += fmt.Sprintf(", %d findings", testCase.Findings)
		}
		if testCase.Error != "" {
			detail += ": " + testCase.Error
		}
		if len(testCase.MissingRules) > 0 {
			detail += ": missing required rules " + strings.Join(testCase.MissingRules, ", ")
		}
		if _, err := fmt.Fprintf(w, "  %s %s (%s)\n", testCase.Status, testCase.Name, detail); err != nil {
			return err
		}
	}
	return nil
}

func JSON(w io.Writer, document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
