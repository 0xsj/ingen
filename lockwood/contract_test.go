package lockwood

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var sha256Digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func TestDraftSchemasHaveExpectedBoundary(t *testing.T) {
	for _, fixture := range []struct {
		name           string
		path           string
		constValue     string
		requiredFields []string
	}{
		{
			name:           "artifact",
			path:           filepath.Join("spec", "lockwood.artifact-v1.schema.json"),
			constValue:     "lockwood.artifact/v1",
			requiredFields: []string{"schema", "digest", "size_bytes", "media_type"},
		},
		{
			name:           "custody",
			path:           filepath.Join("spec", "lockwood.custody-v1.schema.json"),
			constValue:     "lockwood.custody/v1",
			requiredFields: []string{"schema", "custody_id", "status", "artifact", "received_at", "producer", "custodian", "source", "integrity", "parents", "handling"},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}

			var schema struct {
				ID         string                     `json:"$id"`
				Draft      string                     `json:"$schema"`
				Type       string                     `json:"type"`
				Closed     bool                       `json:"additionalProperties"`
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatalf("decode schema: %v", err)
			}
			if schema.ID == "" || schema.Draft == "" || schema.Type != "object" || schema.Closed {
				t.Fatalf("incomplete or open schema: %+v", schema)
			}
			for _, required := range fixture.requiredFields {
				if !contains(schema.Required, required) {
					t.Fatalf("schema is missing required field %q", required)
				}
				if _, ok := schema.Properties[required]; !ok {
					t.Fatalf("schema is missing property %q", required)
				}
			}

			var schemaProperty struct {
				Const string `json:"const"`
			}
			if err := json.Unmarshal(schema.Properties["schema"], &schemaProperty); err != nil {
				t.Fatalf("decode schema discriminator: %v", err)
			}
			if schemaProperty.Const != fixture.constValue {
				t.Fatalf("schema discriminator = %q, want %q", schemaProperty.Const, fixture.constValue)
			}
		})
	}
}

func TestValidCustodyFixture(t *testing.T) {
	record := loadFixture(t, "valid-custody-record.json")
	if err := validateCustodyFixture(record); err != nil {
		t.Fatalf("valid custody fixture rejected: %v", err)
	}
}

func TestExampleCustodyRecordMatchesDraftSchema(t *testing.T) {
	schema := compileLockwoodSchema(t, custodySchemaURL, "spec/lockwood.custody-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "examples/custody-record.json")); err != nil {
		t.Fatalf("example custody record rejected by draft schema: %v", err)
	}
}

func TestInvalidCustodyFixtures(t *testing.T) {
	for _, name := range []string{
		"invalid-digest.json",
		"invalid-missing-required.json",
		"invalid-lineage.json",
	} {
		t.Run(name, func(t *testing.T) {
			record := loadFixture(t, name)
			if err := validateCustodyFixture(record); err == nil {
				t.Fatal("invalid custody fixture was accepted")
			}
		})
	}
}

func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return record
}

func validateCustodyFixture(record map[string]any) error {
	for _, field := range []string{"schema", "custody_id", "artifact", "received_at", "producer", "source", "integrity", "parents", "handling"} {
		if _, ok := record[field]; !ok {
			return fmt.Errorf("missing required field %q", field)
		}
	}
	if record["schema"] != "lockwood.custody/v1" {
		return fmt.Errorf("unexpected custody schema %v", record["schema"])
	}
	if record["status"] != "accepted" {
		return fmt.Errorf("unexpected custody status %v", record["status"])
	}

	artifact, ok := record["artifact"].(map[string]any)
	if !ok {
		return fmt.Errorf("artifact is not an object")
	}
	if artifact["schema"] != "lockwood.artifact/v1" {
		return fmt.Errorf("unexpected artifact schema %v", artifact["schema"])
	}
	digest, ok := artifact["digest"].(string)
	if !ok || !sha256Digest.MatchString(digest) {
		return fmt.Errorf("invalid artifact digest %v", artifact["digest"])
	}

	integrity, ok := record["integrity"].(map[string]any)
	if !ok {
		return fmt.Errorf("integrity is not an object")
	}
	if integrity["status"] == "verified" {
		if _, ok := integrity["verified_at"]; !ok {
			return fmt.Errorf("verified integrity is missing verified_at")
		}
	}
	if integrity["status"] != "verified" {
		return fmt.Errorf("accepted custody has non-verified integrity status %v", integrity["status"])
	}

	parents, ok := record["parents"].([]any)
	if !ok {
		return fmt.Errorf("parents is not an array")
	}
	for _, rawParent := range parents {
		parent, ok := rawParent.(map[string]any)
		if !ok {
			return fmt.Errorf("lineage parent is not an object")
		}
		switch parent["relation"] {
		case "references", "derived-from", "contains", "verifies":
		default:
			return fmt.Errorf("unsupported lineage relation %v", parent["relation"])
		}
		parentDigest, ok := parent["digest"].(string)
		if !ok || !sha256Digest.MatchString(parentDigest) {
			return fmt.Errorf("invalid lineage digest %v", parent["digest"])
		}
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
