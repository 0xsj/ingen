package spec

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"ingen/core/cliversion"
)

func TestVersionSchemaPreservesUnknownSourceIdentity(t *testing.T) {
	schema, err := jsonschema.Compile("tool-version-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cliversion.Current("sentinel", cliversion.Legacy{}))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
	value["source_inputs_sha256"] = "dirty"
	if err := schema.Validate(value); err == nil {
		t.Fatal("accepted a dirty marker as a source-input digest")
	}
	value["source_inputs_sha256"] = "unknown"
	value["credential"] = "undeclared"
	if err := schema.Validate(value); err == nil {
		t.Fatal("accepted undeclared metadata")
	}
}

func TestPublishedBundleVersionReports(t *testing.T) {
	path := os.Getenv("INGEN_BUNDLE_VERSION_REPORTS")
	if path == "" {
		t.Skip("set INGEN_BUNDLE_VERSION_REPORTS to validate built CLI metadata")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reports []any
	if err := json.Unmarshal(contents, &reports); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 9 {
		t.Fatalf("expected nine command version reports, got %d", len(reports))
	}
	schema, err := jsonschema.Compile("tool-version-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, report := range reports {
		if err := schema.Validate(report); err != nil {
			t.Fatal(err)
		}
	}
}
