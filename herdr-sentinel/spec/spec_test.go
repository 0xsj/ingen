package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHerdrEventSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "herdr-event-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ID                   string                     `json:"$id"`
		Draft                string                     `json:"$schema"`
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(contents, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.ID == "" || schema.Draft == "" || schema.Type != "object" || schema.AdditionalProperties {
		t.Fatalf("incomplete or open Herdr event schema: %+v", schema)
	}
	for _, field := range []string{"schema", "event_id", "run_id", "workspace_id", "workspace_version", "type", "at"} {
		if !contains(schema.Required, field) {
			t.Fatalf("Herdr event schema is missing required field %q", field)
		}
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("Herdr event schema is missing property %q", field)
		}
	}
	var discriminator struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
		t.Fatal(err)
	}
	if discriminator.Const != "ingen.herdr-event/v1" {
		t.Fatalf("Herdr event discriminator = %q, want ingen.herdr-event/v1", discriminator.Const)
	}
}

func TestSentinelCIExplanationSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "ci-explanation-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ID                   string                     `json:"$id"`
		Draft                string                     `json:"$schema"`
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(contents, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.ID == "" || schema.Draft == "" || schema.Type != "object" || schema.AdditionalProperties {
		t.Fatalf("incomplete or open Sentinel CI explanation schema: %+v", schema)
	}
	for _, field := range []string{"schema", "receipt_status", "outcome", "artifact_ids", "audit_status", "audit_checks"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("Sentinel CI explanation schema is missing property %q", field)
		}
	}
	for _, field := range []string{"schema", "receipt_status", "outcome", "artifact_ids"} {
		if !contains(schema.Required, field) {
			t.Fatalf("Sentinel CI explanation schema is missing required field %q", field)
		}
	}
	var discriminator struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
		t.Fatal(err)
	}
	if discriminator.Const != "ingen.sentinel-ci-explanation/v1" {
		t.Fatalf("Sentinel CI explanation discriminator = %q, want ingen.sentinel-ci-explanation/v1", discriminator.Const)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
