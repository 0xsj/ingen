package malcolm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/mutation"
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

func TestTranslateAcceptsCaptureInterpolationInLaterRequestBody(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Request = &Request{Body: map[string]any{
			"name":    "{document_name}",
			"content": "copied",
		}}
		s.Setups = []Setup{{
			Name: "create-seed",
			Request: &Request{Body: map[string]any{
				"name": "seed copy.txt",
			}},
			When: When{
				Method: "POST",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.status == 202"},
			},
			Captures: []Capture{{
				Name:     "document_name",
				Selector: "body.name",
			}},
		}}
	})

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated interpolation contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	body := rule["given"].(map[string]any)["body"].(map[string]any)
	if body["name"] != "{document_name}" {
		t.Fatalf("target body name = %#v, want capture placeholder", body["name"])
	}
}

func TestTranslateRejectsInvalidCaptureInterpolationReferences(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing capture",
			body: "{document_name}",
			want: "not available from an earlier setup",
		},
		{
			name: "invalid capture name",
			body: "{document-name}",
			want: "placeholder name must be an identifier",
		},
		{
			name: "unclosed placeholder",
			body: "{document_name",
			want: "unclosed capture placeholder",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ir := validIR(func(s *Scenario) {
				s.Request = &Request{Body: map[string]any{"name": test.body}}
			})
			_, err := Translate(ir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text containing %q", err, test.want)
			}
		})
	}
}

func TestValidateIRRejectsSetupBodyReferenceBeforeCapture(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Setups = []Setup{{
			Name: "create-seed",
			Request: &Request{Body: map[string]any{
				"name": "{document_name}",
			}},
			When: When{
				Method: "POST",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.status == 202"},
			},
			Captures: []Capture{{
				Name:     "document_name",
				Selector: "body.name",
			}},
		}}
	})

	problems := ValidateIR(ir)
	for _, problem := range problems {
		if strings.Contains(problem, "not available from an earlier setup") {
			return
		}
	}
	t.Fatalf("problems = %v, want forward capture-reference error", problems)
}

func TestTranslateLowersNestedResponsePaths(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Requirements[0].Expression = "response.body.metadata.owner.id exists"
	})

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated nested-selector contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	body := rule["expect"].(map[string]any)["body"].(map[string]any)
	metadata := body["properties"].(map[string]any)["metadata"].(map[string]any)
	owner := metadata["properties"].(map[string]any)["owner"].(map[string]any)
	if owner["required"].([]any)[0] != "id" {
		t.Fatalf("nested existence shape = %#v, want required id", body)
	}
}

func TestTranslateMergesNestedResponseExpectations(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Setups = []Setup{{
			Name: "inspect-document",
			When: When{
				Method: "GET",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.body.metadata.owner exists"},
				{Kind: "must", Expression: "response.body.metadata.status == \"ready\""},
			},
		}}
	})

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	setup := rule["given"].(map[string]any)["setup"].([]any)[0].(map[string]any)
	body := setup["expect"].(map[string]any)["body"].(map[string]any)
	metadata := body["properties"].(map[string]any)["metadata"].(map[string]any)
	if len(metadata["required"].([]any)) != 1 || metadata["required"].([]any)[0] != "owner" {
		t.Fatalf("merged nested required shape = %#v", metadata)
	}
	status := metadata["properties"].(map[string]any)["status"].(map[string]any)
	if status["equals"] != "ready" {
		t.Fatalf("merged nested equality shape = %#v", metadata)
	}
}

func TestTranslateLowersNestedCaptureSelector(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Setups = []Setup{{
			Name: "inspect-document",
			When: When{
				Method: "GET",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.body.metadata.owner.id exists"},
			},
			Captures: []Capture{{
				Name:     "owner_id",
				Selector: "body.metadata.owner.id",
			}},
		}}
	})

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	setup := rule["given"].(map[string]any)["setup"].([]any)[0].(map[string]any)
	if got := setup["capture"].(map[string]any)["owner_id"]; got != "body.metadata.owner.id" {
		t.Fatalf("nested capture selector = %v, want body.metadata.owner.id", got)
	}
}

func TestValidateIRRejectsMalformedNestedCaptureSelector(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Setups = []Setup{{
			Name: "inspect-document",
			When: When{
				Method: "GET",
				Path:   "/documents",
			},
			Requirements: []Requirement{
				{Kind: "must", Expression: "response.status == 200"},
			},
			Captures: []Capture{{
				Name:     "owner_id",
				Selector: "body.metadata..owner",
			}},
		}}
	})

	problems := ValidateIR(ir)
	if len(problems) == 0 || !strings.Contains(strings.Join(problems, "; "), "body.FIELD[.FIELD...]") {
		t.Fatalf("problems = %v, want malformed nested-selector error", problems)
	}
}

func TestTranslateLowersNestedRequestBodyValues(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Request = &Request{Body: map[string]any{
			"metadata": map[string]any{
				"source":   "import",
				"priority": int64(2),
				"reviewed": true,
			},
			"tags": []any{"docs", "contract"},
		}}
	})
	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated nested contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	body := rule["given"].(map[string]any)["body"].(map[string]any)
	metadata := body["metadata"].(map[string]any)
	if metadata["source"] != "import" || metadata["priority"] != int64(2) || metadata["reviewed"] != true {
		t.Fatalf("nested metadata = %#v", metadata)
	}
	tags := body["tags"].([]any)
	if len(tags) != 2 || tags[0] != "docs" || tags[1] != "contract" {
		t.Fatalf("nested tags = %#v", tags)
	}
}

func TestTranslateMutationCatalogueBindsToSornaContract(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Mutations = []Mutation{{
		ID:       "status-201",
		Scenario: "health",
		Change: MutationChange{
			Field: "response.status",
			From:  200,
			To:    201,
		},
		ExpectedRule: "health.requirement.1",
	}}

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := TranslateMutationCatalogue(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}
	if problems := mutation.ValidateAgainstContract(catalogue, document); len(problems) > 0 {
		t.Fatalf("translated mutation catalogue is not bound to the contract: %v", problems)
	}
	if catalogue.ContractID != "demo" || catalogue.ContractVersion != 1 {
		t.Fatalf("catalogue contract binding = %#v", catalogue)
	}
	if len(catalogue.Mutations) != 1 {
		t.Fatalf("catalogue mutations = %#v, want one mutation", catalogue.Mutations)
	}
	spec := catalogue.Mutations[0]
	if spec.Plane != "implementation" || spec.Operator != "response.status.replace" {
		t.Fatalf("catalogue mutation metadata = %#v", spec)
	}
	if spec.Target != "GET /healthz" || spec.ExpectedRuleIDs[0] != "health.requirement.1" {
		t.Fatalf("catalogue mutation target = %#v", spec)
	}
	if spec.Change["from"] != int64(200) || spec.Change["to"] != int64(201) {
		t.Fatalf("catalogue mutation change = %#v", spec.Change)
	}
}

func TestTranslateLowersExecutionIDProvenanceRequirement(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Provenance = &Provenance{
		Requirements: []ProvenanceRequirement{{
			Kind:  "create",
			Field: "execution_id",
		}},
	}

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	expect := rule["expect"].(map[string]any)
	provenance := expect["provenance"].(map[string]any)
	required := provenance["required"].([]any)
	if len(required) != 1 || required[0] != "execution_id" {
		t.Fatalf("provenance expectation = %#v, want execution_id", provenance)
	}
}

func TestTranslateLowersPerScenarioIsolationReset(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Isolation = &Isolation{
		Scope: "scenario",
		Reset: Reset{Method: "POST", Path: "/__malcolm/reset"},
	}

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated isolation contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	isolation := rule["given"].(map[string]any)["isolation"].(map[string]any)
	if isolation["scope"] != "scenario" {
		t.Fatalf("isolation scope = %#v, want scenario", isolation["scope"])
	}
	reset := isolation["reset"].(map[string]any)
	if reset["method"] != "POST" || reset["path"] != "/__malcolm/reset" {
		t.Fatalf("isolation reset = %#v, want POST /__malcolm/reset", reset)
	}
}

func TestTranslateLowersDigestPinnedOracleFixture(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Fixtures = []Fixture{{
		ID:      "welcome-document",
		Owner:   "oracle",
		Purpose: "canonical document input",
		SHA256:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}}

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated fixture contract is invalid: %v", problems)
	}
	fixtures := document.Contract["fixtures"].([]any)
	if len(fixtures) != 1 {
		t.Fatalf("translated fixtures = %#v, want one fixture", fixtures)
	}
	fixture := fixtures[0].(map[string]any)
	if fixture["id"] != "welcome-document" || fixture["owner"] != "oracle" || fixture["purpose"] != "canonical document input" || fixture["sha256"] != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("translated fixture = %#v", fixture)
	}
}

func TestValidateIRRejectsUnpinnedOrSubjectOwnedFixture(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Fixtures = []Fixture{{
		ID:      "subject-fixture",
		Owner:   "subject",
		Purpose: "subject-owned implementation fixture",
		SHA256:  "not-a-digest",
	}}

	problems := strings.Join(ValidateIR(ir), "; ")
	for _, want := range []string{
		"owner must be oracle",
		"sha256 must be a lowercase SHA-256 digest",
	} {
		if !strings.Contains(problems, want) {
			t.Fatalf("problems = %s, want %q", problems, want)
		}
	}
}

func TestValidateIRRejectsUnsupportedIsolationReset(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Isolation = &Isolation{
		Scope: "run",
		Reset: Reset{Method: "DELETE", Path: "reset"},
	}

	problems := strings.Join(ValidateIR(ir), "; ")
	for _, want := range []string{
		"scope must be scenario",
		"reset.method must be POST",
		"reset.path must start with /",
	} {
		if !strings.Contains(problems, want) {
			t.Fatalf("problems = %s, want %q", problems, want)
		}
	}
}

func TestValidateIRRejectsMalformedMutationDeclaration(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Mutations = []Mutation{{
		ID:       "status-noop",
		Scenario: "missing",
		Change: MutationChange{
			Field: "response.body",
			From:  200,
			To:    200,
		},
		ExpectedRule: "health.requirement.2",
	}}

	problems := ValidateIR(ir)
	joined := strings.Join(problems, "; ")
	for _, want := range []string{
		"scenario must name an existing scenario",
		"change.field must be response.status",
		"change must change the response status",
		"expected_rule must name an existing scenario requirement",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("problems = %v, want %q", problems, want)
		}
	}
}

func TestValidateIRRejectsUnsupportedProvenanceRequirement(t *testing.T) {
	ir := validIR(nil)
	ir.Specification.Provenance = &Provenance{
		Requirements: []ProvenanceRequirement{{
			Kind:  "preserve",
			Field: "tenant_id",
		}},
	}

	problems := strings.Join(ValidateIR(ir), "; ")
	for _, want := range []string{
		"kind must be create",
		"field must be execution_id",
	} {
		if !strings.Contains(problems, want) {
			t.Fatalf("problems = %s, want %q", problems, want)
		}
	}
}

func TestTranslateLowersRepeatGeneratedRequestValue(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Request = &Request{Body: map[string]any{
			"content": map[string]any{
				"generated": map[string]any{
					"kind":  "repeat",
					"value": "a",
					"count": int64(4097),
				},
			},
		}}
	})

	document, err := Translate(ir)
	if err != nil {
		t.Fatal(err)
	}
	if problems := contract.Validate(document); len(problems) > 0 {
		t.Fatalf("translated generated-value contract is invalid: %v", problems)
	}
	rule := document.Contract["rules"].([]any)[0].(map[string]any)
	body := rule["given"].(map[string]any)["body"].(map[string]any)
	content := body["content"].(map[string]any)
	generated := content["generated"].(map[string]any)
	if generated["kind"] != "repeat" || generated["value"] != "a" || generated["count"] != int64(4097) {
		t.Fatalf("generated request value = %#v", generated)
	}
}

func TestTranslateRejectsMalformedRepeatGeneratedRequestValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "unsupported kind",
			value: map[string]any{
				"generated": map[string]any{"kind": "random", "value": "a", "count": int64(1)},
			},
			want: "kind must be repeat",
		},
		{
			name: "non-string value",
			value: map[string]any{
				"generated": map[string]any{"kind": "repeat", "value": int64(1), "count": int64(1)},
			},
			want: "value must be a string",
		},
		{
			name: "zero count",
			value: map[string]any{
				"generated": map[string]any{"kind": "repeat", "value": "a", "count": int64(0)},
			},
			want: "count must be between 1 and 1000000",
		},
		{
			name: "count over limit",
			value: map[string]any{
				"generated": map[string]any{"kind": "repeat", "value": "a", "count": int64(1_000_001)},
			},
			want: "count must be between 1 and 1000000",
		},
		{
			name: "unknown generator field",
			value: map[string]any{
				"generated": map[string]any{
					"kind": "repeat", "value": "a", "count": int64(1), "seed": int64(7),
				},
			},
			want: "seed is not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ir := validIR(func(s *Scenario) {
				s.Request = &Request{Body: map[string]any{"content": test.value}}
			})
			_, err := Translate(ir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text containing %q", err, test.want)
			}
		})
	}
}

func TestTranslateRejectsNonIntegerNestedBodyNumbers(t *testing.T) {
	ir := validIR(func(s *Scenario) {
		s.Request = &Request{Body: map[string]any{
			"metadata": map[string]any{
				"ratio": json.Number("1.5"),
			},
		}}
	})

	_, err := Translate(ir)
	if err == nil || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("error = %v, want nested integer validation error", err)
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
		{
			name: "malformed nested response selector",
			ir: validIR(func(s *Scenario) {
				s.Requirements[0].Expression = "response.body.metadata..owner exists"
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
