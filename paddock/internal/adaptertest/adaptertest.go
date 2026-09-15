package adaptertest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/paddock/internal/graph"
)

const (
	ManifestSchema    = "paddock.adapter-tests/v1"
	DocumentSchema    = "paddock.adapter-test-result/v1"
	ExplanationSchema = "paddock.adapter-test-explanation/v1"
)

type Manifest struct {
	Schema  string  `yaml:"schema" json:"schema"`
	Adapter Adapter `yaml:"adapter" json:"adapter"`
	Cases   []Case  `yaml:"cases" json:"cases"`
}

type Adapter struct {
	Executable string   `yaml:"executable" json:"executable"`
	Args       []string `yaml:"args,omitempty" json:"args,omitempty"`
}

type Case struct {
	Name              string   `yaml:"name" json:"name"`
	Root              string   `yaml:"root" json:"root"`
	Language          string   `yaml:"language" json:"language"`
	SourceUnit        string   `yaml:"source_unit,omitempty" json:"source_unit,omitempty"`
	Roots             []string `yaml:"roots,omitempty" json:"roots,omitempty"`
	RequiredEdgeKinds []string `yaml:"required_edge_kinds,omitempty" json:"required_edge_kinds,omitempty"`
	Args              []string `yaml:"args,omitempty" json:"args,omitempty"`
	Expect            string   `yaml:"expect" json:"expect"`
	ErrorContains     string   `yaml:"error_contains,omitempty" json:"error_contains,omitempty"`
	PackageCount      *int     `yaml:"package_count,omitempty" json:"package_count,omitempty"`
	EdgeCount         *int     `yaml:"edge_count,omitempty" json:"edge_count,omitempty"`
}

type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Document struct {
	Schema   string       `json:"schema"`
	Manifest FileRef      `json:"manifest"`
	Adapter  Adapter      `json:"adapter"`
	Status   string       `json:"status"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	Cases    []CaseResult `json:"cases"`
}

type CaseResult struct {
	Name             string `json:"name"`
	Root             string `json:"root"`
	Language         string `json:"language"`
	SourceUnit       string `json:"source_unit,omitempty"`
	Expected         string `json:"expected"`
	Actual           string `json:"actual"`
	Status           string `json:"status"`
	AssertionsPassed bool   `json:"assertions_passed"`
	PackageCount     int    `json:"package_count,omitempty"`
	EdgeCount        int    `json:"edge_count,omitempty"`
	Error            string `json:"error,omitempty"`
}

type Explanation struct {
	Schema       string `json:"schema"`
	SourceSchema string `json:"source_schema"`
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	Passed       int    `json:"passed"`
	Failed       int    `json:"failed"`
}

func (d Document) Explain() Explanation {
	status := "PASS"
	if d.Status != "PASS" {
		status = "FAIL"
	}
	return Explanation{
		Schema:       ExplanationSchema,
		SourceSchema: DocumentSchema,
		Status:       status,
		Summary:      fmt.Sprintf("%d of %d adapter conformance cases passed.", d.Passed, len(d.Cases)),
		Passed:       d.Passed,
		Failed:       d.Failed,
	}
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read adapter test manifest: %w", err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse adapter test manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.Schema != ManifestSchema {
		return fmt.Errorf("adapter test manifest schema must be %s, got %q", ManifestSchema, m.Schema)
	}
	if strings.TrimSpace(m.Adapter.Executable) == "" {
		return fmt.Errorf("adapter test manifest adapter executable is required")
	}
	if len(m.Cases) == 0 {
		return fmt.Errorf("adapter test manifest cases must not be empty")
	}
	seen := make(map[string]struct{}, len(m.Cases))
	for index, testCase := range m.Cases {
		if strings.TrimSpace(testCase.Name) == "" {
			return fmt.Errorf("adapter test case %d name is required", index+1)
		}
		if _, exists := seen[testCase.Name]; exists {
			return fmt.Errorf("duplicate adapter test case %q", testCase.Name)
		}
		seen[testCase.Name] = struct{}{}
		if strings.TrimSpace(testCase.Root) == "" {
			return fmt.Errorf("adapter test case %q root is required", testCase.Name)
		}
		if strings.TrimSpace(testCase.Language) == "" {
			return fmt.Errorf("adapter test case %q language is required", testCase.Name)
		}
		switch testCase.Expect {
		case "pass", "error":
		default:
			return fmt.Errorf("adapter test case %q expect must be pass or error", testCase.Name)
		}
		if testCase.ErrorContains != "" && testCase.Expect != "error" {
			return fmt.Errorf("adapter test case %q error_contains requires expect error", testCase.Name)
		}
		if err := validateStringList(testCase.Name, "roots", testCase.Roots); err != nil {
			return err
		}
		if err := validateStringList(testCase.Name, "required_edge_kinds", testCase.RequiredEdgeKinds); err != nil {
			return err
		}
		if testCase.PackageCount != nil && *testCase.PackageCount < 0 {
			return fmt.Errorf("adapter test case %q package_count must not be negative", testCase.Name)
		}
		if testCase.EdgeCount != nil && *testCase.EdgeCount < 0 {
			return fmt.Errorf("adapter test case %q edge_count must not be negative", testCase.Name)
		}
	}
	return nil
}

func validateStringList(caseName, field string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("adapter test case %q %s cannot contain empty values", caseName, field)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("adapter test case %q %s contains duplicate value %q", caseName, field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func Run(manifestPath string) (Document, error) {
	absoluteManifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return Document{}, fmt.Errorf("resolve adapter test manifest: %w", err)
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
		Manifest: manifestRef,
		Adapter:  manifest.Adapter,
		Status:   "PASS",
		Cases:    make([]CaseResult, 0, len(manifest.Cases)),
	}
	for _, testCase := range manifest.Cases {
		root := testCase.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(base, root)
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return Document{}, fmt.Errorf("resolve adapter test case %q root: %w", testCase.Name, err)
		}
		args := manifest.Adapter.Args
		if len(testCase.Args) > 0 {
			args = testCase.Args
		}
		args = expandArgs(args, root, base)
		executable := resolveExecutable(manifest.Adapter.Executable, base)
		requiredCapabilities := graph.Capabilities{
			EdgeKinds: append([]string(nil), testCase.RequiredEdgeKinds...),
		}
		if testCase.SourceUnit != "" {
			requiredCapabilities.SourceUnits = []string{testCase.SourceUnit}
		}
		request := graph.Request{
			Schema:               graph.RequestSchema,
			Language:             testCase.Language,
			Unit:                 testCase.SourceUnit,
			Root:                 root,
			Roots:                append([]string(nil), testCase.Roots...),
			RequiredCapabilities: requiredCapabilities,
		}
		_, response, checkErr := graph.LoadExternal(context.Background(), executable, args, request)
		caseResult := CaseResult{
			Name:             testCase.Name,
			Root:             root,
			Language:         testCase.Language,
			SourceUnit:       testCase.SourceUnit,
			Expected:         testCase.Expect,
			AssertionsPassed: true,
		}
		if checkErr != nil {
			caseResult.Actual = "error"
			caseResult.Error = checkErr.Error()
			if testCase.ErrorContains != "" && !strings.Contains(caseResult.Error, testCase.ErrorContains) {
				caseResult.AssertionsPassed = false
			}
		} else {
			caseResult.Actual = "pass"
			caseResult.PackageCount = response.PackageCount
			caseResult.EdgeCount = response.EdgeCount
			if testCase.PackageCount != nil && response.PackageCount != *testCase.PackageCount {
				caseResult.AssertionsPassed = false
				caseResult.Error = fmt.Sprintf("package_count = %d, want %d", response.PackageCount, *testCase.PackageCount)
			}
			if testCase.EdgeCount != nil && response.EdgeCount != *testCase.EdgeCount {
				caseResult.AssertionsPassed = false
				caseResult.Error = appendError(caseResult.Error, fmt.Sprintf("edge_count = %d, want %d", response.EdgeCount, *testCase.EdgeCount))
			}
		}
		if caseResult.Actual == testCase.Expect && caseResult.AssertionsPassed {
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

func appendError(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

func expandArgs(args []string, root, manifestDir string) []string {
	expanded := make([]string, len(args))
	for index, arg := range args {
		expanded[index] = strings.ReplaceAll(arg, "{{root}}", root)
		expanded[index] = strings.ReplaceAll(expanded[index], "{{manifest_dir}}", manifestDir)
	}
	return expanded
}

func resolveExecutable(executable, base string) string {
	if filepath.IsAbs(executable) || !strings.ContainsAny(executable, `/\\`) {
		return executable
	}
	return filepath.Join(base, executable)
}

func fileRef(path string) (FileRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileRef{Path: path}, fmt.Errorf("hash adapter test manifest: %w", err)
	}
	digest := sha256.Sum256(data)
	return FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}, nil
}

func Save(path string, document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode adapter test result: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write adapter test result: %w", err)
	}
	return nil
}

func LoadResult(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read adapter test result: %w", err)
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return Document{}, fmt.Errorf("parse adapter test result: %w", err)
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func VerifyFiles(document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	data, err := os.ReadFile(document.Manifest.Path)
	if err != nil {
		return fmt.Errorf("read adapter test manifest %q: %w", document.Manifest.Path, err)
	}
	digest := sha256.Sum256(data)
	actual := hex.EncodeToString(digest[:])
	if actual != document.Manifest.SHA256 {
		return fmt.Errorf("adapter test manifest %q hash mismatch: expected %s, got %s", document.Manifest.Path, document.Manifest.SHA256, actual)
	}
	return nil
}

func (d Document) Validate() error {
	if d.Schema != DocumentSchema {
		return fmt.Errorf("adapter test result schema must be %s, got %q", DocumentSchema, d.Schema)
	}
	if d.Manifest.Path == "" || !isSHA256(d.Manifest.SHA256) {
		return fmt.Errorf("adapter test result manifest must include a path and SHA-256 digest")
	}
	if strings.TrimSpace(d.Adapter.Executable) == "" {
		return fmt.Errorf("adapter test result adapter executable is required")
	}
	if d.Status != "PASS" && d.Status != "FAIL" {
		return fmt.Errorf("adapter test result has unsupported status %q", d.Status)
	}
	if d.Passed < 0 || d.Failed < 0 || d.Passed+d.Failed != len(d.Cases) || len(d.Cases) == 0 {
		return fmt.Errorf("adapter test result counts do not match cases")
	}
	if (d.Failed == 0 && d.Status != "PASS") || (d.Failed > 0 && d.Status != "FAIL") {
		return fmt.Errorf("adapter test result status does not match failed case count")
	}
	seen := make(map[string]struct{}, len(d.Cases))
	for _, testCase := range d.Cases {
		if testCase.Name == "" || testCase.Root == "" || testCase.Language == "" {
			return fmt.Errorf("adapter test result cases require name, root, and language")
		}
		if _, exists := seen[testCase.Name]; exists {
			return fmt.Errorf("adapter test result has duplicate case %q", testCase.Name)
		}
		seen[testCase.Name] = struct{}{}
		if !validOutcome(testCase.Expected) || !validOutcome(testCase.Actual) {
			return fmt.Errorf("adapter test result case %q has an unsupported outcome", testCase.Name)
		}
		if testCase.Status != "PASS" && testCase.Status != "FAIL" {
			return fmt.Errorf("adapter test result case %q has an unsupported status %q", testCase.Name, testCase.Status)
		}
		if testCase.PackageCount < 0 || testCase.EdgeCount < 0 {
			return fmt.Errorf("adapter test result case %q has negative graph counts", testCase.Name)
		}
		wantPass := testCase.Expected == testCase.Actual && testCase.AssertionsPassed
		if (testCase.Status == "PASS") != wantPass {
			return fmt.Errorf("adapter test result case %q status does not match its outcome", testCase.Name)
		}
		if testCase.Actual == "error" && testCase.Error == "" {
			return fmt.Errorf("adapter test result case %q error outcome needs an error", testCase.Name)
		}
	}
	return nil
}

func validOutcome(value string) bool {
	return value == "pass" || value == "error"
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func Text(w io.Writer, document Document) error {
	if _, err := fmt.Fprintf(w, "ADAPTER-TEST %s (%d/%d passed)\n", document.Status, document.Passed, len(document.Cases)); err != nil {
		return err
	}
	for _, testCase := range document.Cases {
		detail := fmt.Sprintf("%s/%s, expected %s, got %s", testCase.Language, testCase.SourceUnit, testCase.Expected, testCase.Actual)
		if testCase.PackageCount > 0 || testCase.EdgeCount > 0 {
			detail += fmt.Sprintf(", %d packages, %d edges", testCase.PackageCount, testCase.EdgeCount)
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
	if err := document.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
