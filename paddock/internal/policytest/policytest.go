package policytest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"ingen/paddock/internal/checker"
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
	Name   string `yaml:"name" json:"name"`
	Root   string `yaml:"root" json:"root"`
	Expect string `yaml:"expect" json:"expect"`
}

type Document struct {
	Schema string       `json:"schema"`
	Policy string       `json:"policy"`
	Status string       `json:"status"`
	Passed int          `json:"passed"`
	Failed int          `json:"failed"`
	Cases  []CaseResult `json:"cases"`
}

type CaseResult struct {
	Name     string `json:"name"`
	Root     string `json:"root"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Status   string `json:"status"`
	Findings int    `json:"findings,omitempty"`
	Error    string `json:"error,omitempty"`
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
	}
	return nil
}

func Run(manifestPath, policyPath string, config policy.Policy) (Document, error) {
	manifest, err := Load(manifestPath)
	if err != nil {
		return Document{}, err
	}
	base := filepath.Dir(manifestPath)
	document := Document{
		Schema: DocumentSchema,
		Policy: policyPath,
		Status: "PASS",
		Cases:  make([]CaseResult, 0, len(manifest.Cases)),
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
		result, checkErr := checker.CheckPolicy(root, policyPath, config)
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
		if caseResult.Actual == testCase.Expect {
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
		if _, err := fmt.Fprintf(w, "  %s %s (%s)\n", testCase.Status, testCase.Name, detail); err != nil {
			return err
		}
	}
	return nil
}

func JSON(w io.Writer, document Document) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
