package malcolm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
)

func TestTranslateHealthcheckIRToValidSornaContract(t *testing.T) {
	ir := IR{
		Schema: IRSchema,
		Specification: Specification{
			Name:    "document_health",
			Version: stringPointer("v1"),
			Subject: stringPointer("document-pipeline"),
			Scenarios: []Scenario{{
				Name: "healthcheck",
				When: When{
					Method: "GET",
					Path:   "/healthz",
				},
				Requirements: []Requirement{
					{Kind: "must", Expression: "response.status == 200"},
					{Kind: "must", Expression: "response.body.status == \"ok\""},
				},
			}},
		},
	}

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}

	rules := document.Contract["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("translated rule count = %d, want 2", len(rules))
	}
	first := rules[0].(map[string]any)
	if first["subject"] != "GET /healthz" {
		t.Fatalf("translated subject = %v, want GET /healthz", first["subject"])
	}
	second := rules[1].(map[string]any)
	secondExpect := second["expect"].(map[string]any)
	secondBody := secondExpect["body"].(map[string]any)
	if secondBody["properties"].(map[string]any)["status"].(map[string]any)["equals"] != "ok" {
		t.Fatalf("translated body equality = %#v", secondBody)
	}
}

func TestTranslateLowersMustNotRequirement(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Requirements[0].Kind = "must_not"
	})
	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	if rule["strength"] != "must_not" {
		t.Fatalf("strength = %v, want must_not", rule["strength"])
	}
}

func TestTranslateLowersEventRequirement(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Requirements[0].Expression = "emit \"document.accepted\""
	})
	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	expect := rule["expect"].(map[string]any)
	events := expect["events"].(map[string]any)
	if got := events["required"].([]any); len(got) != 1 || got[0] != "document.accepted" {
		t.Fatalf("event expectation = %#v, want document.accepted", events)
	}
}

func TestTranslateLowersOrderedEventRequirement(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Requirements[0].Expression = "emit in order [\"document.accepted\", \"document.queued\"]"
	})
	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	expect := rule["expect"].(map[string]any)
	events := expect["events"].(map[string]any)
	ordered := events["ordered"].([]any)
	if len(ordered) != 2 || ordered[0] != "document.accepted" || ordered[1] != "document.queued" {
		t.Fatalf("ordered event expectation = %#v, want accepted then queued", events)
	}
}

func TestTranslateLowersNegativeSetupRequirement(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Setups = []Setup{{
			Name: "prepare",
			Request: &Request{Body: map[string]any{
				"name": "welcome.md",
			}},
			When: When{
				Method: "POST",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.status == 202"},
				{Kind: "must_not", Expression: "response.body.error exists"},
				{Kind: "must_not", Expression: "response.body.rejected exists"},
			},
		}}
	})
	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	setup := rule["given"].(map[string]any)["setup"].([]any)[0].(map[string]any)
	if _, present := setup["expect"].(map[string]any)["status"]; !present {
		t.Fatalf("positive setup expectation = %#v, want status", setup["expect"])
	}
	expectNot := setup["expect_not"].([]any)
	if len(expectNot) != 2 {
		t.Fatalf("negative setup expectations = %#v, want two independent expectations", setup["expect_not"])
	}
	expectNotBody := expectNot[0].(map[string]any)
	body := expectNotBody["body"].(map[string]any)
	if len(body["required"].([]any)) != 1 || body["required"].([]any)[0] != "error" {
		t.Fatalf("negative setup expectation = %#v, want missing error", expectNot)
	}
}

func TestTranslateLowersRequestBodyAndStatefulSetup(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.State = stringPointer("document_accepted")
		s.Request = &Request{Body: map[string]any{
			"name":      "welcome.md",
			"published": true,
			"retries":   int64(2),
		}}
		s.Setups = []Setup{{
			Name: "accept-document",
			Request: &Request{Body: map[string]any{
				"name": "welcome.md",
			}},
			When: When{
				Method: "POST",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.status == 202"},
				{Kind: "must", Expression: "response.body.id exists"},
			},
			Captures: []Capture{{
				Name:     "document_id",
				Selector: "body.id",
			}},
		}}
	})

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated stateful contract is invalid: %v", problems)
	}

	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	given := rule["given"].(map[string]any)
	if given["state"] != "document_accepted" {
		t.Fatalf("state = %v, want document_accepted", given["state"])
	}
	body := given["body"].(map[string]any)
	if body["name"] != "welcome.md" || body["published"] != true || body["retries"] != int64(2) {
		t.Fatalf("request body = %#v", body)
	}
	setup := given["setup"].([]any)[0].(map[string]any)
	setupRequest := setup["request"].(map[string]any)
	if setupRequest["method"] != "POST" || setupRequest["path"] != "/documents" {
		t.Fatalf("setup request = %#v", setupRequest)
	}
	setupExpect := setup["expect"].(map[string]any)
	if setupExpect["status"] != int64(202) {
		t.Fatalf("setup status = %v, want 202", setupExpect["status"])
	}
	setupBody := setupExpect["body"].(map[string]any)
	if len(setupBody["required"].([]any)) != 1 {
		t.Fatalf("setup required fields = %#v", setupBody["required"])
	}
	if setup["capture"].(map[string]any)["document_id"] != "body.id" {
		t.Fatalf("setup capture = %#v", setup["capture"])
	}
}

func TestTranslateRejectsMeaningItCannotLower(t *testing.T) {
	tests := []struct {
		name string
		ir   IR
		want string
	}{
		{
			name: "free-form setup",
			ir:   validIR(func(s *Scenario) { s.Given = []string{"document is ready"} }),
			want: "free-form given",
		},
		{
			name: "stateful setup unsupported requirement kind",
			ir: validIR(func(s *Scenario) {
				s.Setups = []Setup{{
					Name: "prepare",
					When: When{
						Method: "POST",
						Path:   "/documents",
					},
					Requirements: []Requirement{{
						Kind:       "should",
						Expression: "response.status == 500",
					}},
				}}
			}),
			want: "kind must be must or must_not",
		},
		{
			name: "unknown expression",
			ir: validIR(func(s *Scenario) {
				s.Requirements[0].Expression = "response.header.X-InGen-Event exists"
			}),
			want: "not lowerable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Translate(test.ir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text containing %q", err, test.want)
			}
		})
	}
}

func TestLoadFileRejectsUnknownIRFields(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ir.json")
	contents := map[string]any{
		"schema": "malcolm.ir/v1",
		"specification": map[string]any{
			"name":    "demo",
			"version": "v1",
			"subject": nil,
			"scenarios": []any{map[string]any{
				"name":         "health",
				"given":        []any{},
				"when":         map[string]any{"method": "GET", "path": "/healthz"},
				"requirements": []any{map[string]any{"kind": "must", "expression": "response.status == 200"}},
			}},
		},
		"future_field": true,
	}
	bytes, err := json.Marshal(contents)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown-field error", err)
	}
}

func TestWriteFileProducesLoadableSornaContract(t *testing.T) {
	document, err := Translate(validIR(nil))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nested", "contract.json")
	if err := WriteFile(path, document); err != nil {
		t.Fatal(err)
	}
	loaded, err := contract.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Contract["schema"] != SornaSchema {
		t.Fatalf("loaded schema = %v, want %s", loaded.Contract["schema"], SornaSchema)
	}
}

func validIR(change func(*Scenario)) IR {
	scenario := Scenario{
		Name: "health",
		When: When{
			Method: "GET",
			Path:   "/healthz",
		},
		Requirements: []Requirement{{
			Kind:       "must",
			Expression: "response.status == 200",
		}},
	}
	if change != nil {
		change(&scenario)
	}
	return IR{
		Schema: IRSchema,
		Specification: Specification{
			Name:      "demo",
			Version:   stringPointer("v1"),
			Scenarios: []Scenario{scenario},
		},
	}
}

func stringPointer(value string) *string {
	return &value
}
