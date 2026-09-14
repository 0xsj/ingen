package amber

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

type conformanceCase struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type conformanceFixture struct {
	Version     int                       `json:"version"`
	Valid       []conformanceCase         `json:"valid"`
	Invalid     []conformanceCase         `json:"invalid"`
	Incoming    []incomingConformanceCase `json:"incoming"`
	Transitions []conformanceTransition   `json:"transitions"`
}

type incomingConformanceCase struct {
	Name            string          `json:"name"`
	InputRef        string          `json:"input_ref"`
	InputValue      json.RawMessage `json:"input_value"`
	Policy          IncomingPolicy  `json:"policy"`
	ExpectedPresent bool            `json:"expected_present"`
	ExpectedRef     string          `json:"expected_ref"`
	ExpectError     bool            `json:"expect_error"`
}

type conformanceTransition struct {
	Name         string          `json:"name"`
	Operation    string          `json:"operation"`
	Input        string          `json:"input"`
	GeneratedIDs []ID            `json:"generated_ids"`
	Expected     json.RawMessage `json:"expected"`
}

func TestV1ConformanceFixtures(t *testing.T) {
	data, err := os.ReadFile("../conformance/v1.json")
	if err != nil {
		t.Fatalf("read conformance fixture: %v", err)
	}
	var fixture conformanceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode conformance fixture: %v", err)
	}
	if fixture.Version != Version {
		t.Fatalf("fixture version = %d, want %d", fixture.Version, Version)
	}

	for _, testCase := range fixture.Valid {
		t.Run("valid/"+testCase.Name, func(t *testing.T) {
			if _, err := FromJSON(testCase.Value); err != nil {
				t.Fatalf("valid fixture rejected: %v", err)
			}
		})
	}
	for _, testCase := range fixture.Invalid {
		t.Run("invalid/"+testCase.Name, func(t *testing.T) {
			if _, err := FromJSON(testCase.Value); err == nil {
				t.Fatal("invalid fixture was accepted")
			}
		})
	}

	validValues := make(map[string]json.RawMessage, len(fixture.Valid))
	for _, testCase := range fixture.Valid {
		validValues[testCase.Name] = testCase.Value
	}
	for _, testCase := range fixture.Incoming {
		t.Run("incoming/"+testCase.Name, func(t *testing.T) {
			var input json.RawMessage
			if testCase.InputRef != "" {
				var ok bool
				input, ok = validValues[testCase.InputRef]
				if !ok {
					t.Fatalf("input fixture %q not found", testCase.InputRef)
				}
			} else if testCase.InputValue != nil {
				input = testCase.InputValue
			}
			provenance, present, err := InspectIncomingJSON(input, testCase.Policy)
			if testCase.ExpectError {
				if err == nil || present {
					t.Fatalf("expected incoming error: present=%v err=%v", present, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected incoming error: %v", err)
			}
			if present != testCase.ExpectedPresent {
				t.Fatalf("present=%v, want %v", present, testCase.ExpectedPresent)
			}
			if testCase.ExpectedRef != "" {
				expected, ok := validValues[testCase.ExpectedRef]
				if !ok {
					t.Fatalf("expected fixture %q not found", testCase.ExpectedRef)
				}
				expectedProvenance, err := FromJSON(expected)
				if err != nil {
					t.Fatal(err)
				}
				if provenance.ExecutionID() != expectedProvenance.ExecutionID() {
					t.Fatalf("execution_id=%s, want %s", provenance.ExecutionID(), expectedProvenance.ExecutionID())
				}
			}
		})
	}
	for _, testCase := range fixture.Transitions {
		t.Run("transition/"+testCase.Name, func(t *testing.T) {
			input, ok := validValues[testCase.Input]
			if !ok {
				t.Fatalf("input fixture %q not found", testCase.Input)
			}
			ids := append([]ID(nil), testCase.GeneratedIDs...)
			factory := func() (ID, error) {
				if len(ids) == 0 {
					return "", fmt.Errorf("deterministic ID sequence exhausted")
				}
				id := ids[0]
				ids = ids[1:]
				return id, nil
			}
			provenance, err := FromJSONWithIDFactory(input, factory)
			if err != nil {
				t.Fatalf("decode input: %v", err)
			}

			var got Provenance
			switch testCase.Operation {
			case "child":
				got, err = provenance.Child()
			case "retry":
				got, err = provenance.Retry()
			case "replay":
				got, err = provenance.Replay()
			default:
				t.Fatalf("unsupported operation %q", testCase.Operation)
			}
			if err != nil {
				t.Fatalf("%s transition: %v", testCase.Operation, err)
			}
			if len(ids) != 0 {
				t.Fatalf("transition left %d generated IDs unused", len(ids))
			}

			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			var gotValue, expectedValue any
			if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if err := json.Unmarshal(testCase.Expected, &expectedValue); err != nil {
				t.Fatalf("decode expected result: %v", err)
			}
			if !reflect.DeepEqual(gotValue, expectedValue) {
				t.Fatalf("result mismatch:\ngot:  %s\nwant: %s", gotJSON, testCase.Expected)
			}
		})
	}
}
