package amberlogging

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func TestToFieldsAndMergeDoNotExposeOptionalDomainData(t *testing.T) {
	root, err := amber.Start(amber.StartOptions{
		Attribution: &amber.Attribution{
			InitiatedBy: &amber.Actor{ID: "user-42", Type: "user"},
			TenantID:    "tenant-7",
		},
		References: []amber.Reference{{Type: "invoice", ID: "invoice-123"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields, err := ToFields(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["amber.work_id"]; !ok {
		t.Fatal("identity fields are missing")
	}
	if _, ok := fields["amber.tenant_id"]; ok {
		t.Fatal("tenant attribution must be opt-in")
	}
	if _, ok := fields["amber.reference_count"]; ok {
		t.Fatal("typed references must be omitted by default")
	}

	existing := map[string]any{"message": "created", "amber.work_id": "stale"}
	merged, err := MergeFields(existing, root)
	if err != nil {
		t.Fatal(err)
	}
	if existing["amber.work_id"] != "stale" || merged["amber.work_id"] == "stale" {
		t.Fatal("MergeFields must clone and replace fields without mutating input")
	}
}

type loggingConformanceFixture struct {
	Version  int             `json:"version"`
	Value    json.RawMessage `json:"value"`
	Expected map[string]any  `json:"expected"`
}

func TestLoggingConformanceFixture(t *testing.T) {
	data, err := os.ReadFile("../../../conformance/logging-v1.json")
	if err != nil {
		t.Fatalf("read logging fixture: %v", err)
	}
	var fixture loggingConformanceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode logging fixture: %v", err)
	}
	if fixture.Version != amber.Version {
		t.Fatalf("fixture version = %d, want %d", fixture.Version, amber.Version)
	}
	provenance, err := amber.FromJSON(fixture.Value)
	if err != nil {
		t.Fatalf("decode provenance fixture: %v", err)
	}
	actual, err := ToFields(provenance)
	if err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	var normalizedActual map[string]any
	if err := json.Unmarshal(actualJSON, &normalizedActual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizedActual, fixture.Expected) {
		t.Fatalf("fields mismatch:\ngot:  %#v\nwant: %#v", actual, fixture.Expected)
	}
}
