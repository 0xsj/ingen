package spec

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"ingen/herdr-sentinel/internal/provenance"
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

func TestSentinelRoleExecutionExplanationSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "role-execution-explanation-v1.schema.json"))
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
		t.Fatalf("incomplete or open Sentinel role execution explanation schema: %+v", schema)
	}
	for _, field := range []string{"schema", "execution_id", "workspace_id", "role_id", "role_kind", "outcome", "report_status", "report_sha256", "enforcement", "assurance", "meaning"} {
		if _, ok := schema.Properties[field]; !ok || !contains(schema.Required, field) {
			t.Fatalf("Sentinel role execution explanation schema is missing required property %q", field)
		}
	}
	var discriminator struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
		t.Fatal(err)
	}
	if discriminator.Const != "ingen.sentinel-role-execution-explanation/v1" {
		t.Fatalf("role execution explanation discriminator = %q", discriminator.Const)
	}
}

func TestSentinelProvenancePublishedReceiptsMatchSchema(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	schemaPath := filepath.Join(filepath.Dir(sourcePath), "provenance-execution-v1.schema.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	const schemaURL = "https://ingen.dev/schemas/sentinel/provenance-execution-v1.schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(schemaBytes)); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, _, err := provenance.Start(root, ".ingen/provenance/root.json"); err != nil {
		t.Fatal(err)
	}
	falseCommand, err := exec.LookPath("false")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		command []string
		cancel  bool
	}{
		{name: "completed", command: []string{"/bin/echo", "complete"}},
		{name: "failed", command: []string{falseCommand}},
		{name: "canceled", command: []string{"/bin/sleep", "2"}, cancel: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancel {
				time.AfterFunc(100*time.Millisecond, cancel)
			}
			receipt, execErr := provenance.Execute(ctx, provenance.Request{
				Root: root, ParentPath: ".ingen/provenance/root.json",
				OutputPath:  ".ingen/provenance/" + test.name + ".context.json",
				ReceiptPath: ".ingen/provenance/" + test.name + ".receipt.json",
				Operation:   "schema-check", Command: test.command,
			})
			if receipt.Schema != provenance.ReceiptSchema || test.name == "completed" && execErr != nil || test.name != "completed" && execErr == nil {
				t.Fatalf("Execute %s returned receipt %+v and err %v", test.name, receipt, execErr)
			}
			contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(receipt.ReceiptPath)))
			if err != nil {
				t.Fatalf("read %s receipt (status %s): %v", test.name, receipt.Status, err)
			}
			var document any
			if err := json.Unmarshal(contents, &document); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(document); err != nil {
				t.Fatalf("generated %s receipt does not satisfy schema: %v", test.name, err)
			}
			if receipt.Status != test.name {
				t.Fatalf("receipt status = %s, want %s", receipt.Status, test.name)
			}
		})
	}
}

func TestSentinelSessionSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "session-v1.schema.json"))
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
		t.Fatalf("incomplete or open Sentinel session schema: %+v", schema)
	}
	for _, field := range []string{"schema", "session_id", "run_id", "role_id", "status", "enforcement", "assurance"} {
		if !contains(schema.Required, field) {
			t.Fatalf("Sentinel session schema is missing required field %q", field)
		}
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("Sentinel session schema is missing property %q", field)
		}
	}
	var discriminator struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
		t.Fatal(err)
	}
	if discriminator.Const != "ingen.sentinel-session/v1" {
		t.Fatalf("Sentinel session discriminator = %q, want ingen.sentinel-session/v1", discriminator.Const)
	}
}

func TestSentinelRoleExecutionSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "role-execution-v1.schema.json"))
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
		t.Fatalf("incomplete or open role execution schema: %+v", schema)
	}
	for _, field := range []string{"schema", "execution_id", "manifest_sha256", "policy_sha256", "workspace_manifest_path", "policy_path", "executable_sha256", "backend", "enforcement", "assurance", "network_mode", "declared_deny_roots", "stdout_path", "stderr_path", "status"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("role execution schema is missing property %q", field)
		}
		if !contains(schema.Required, field) {
			t.Fatalf("role execution schema is missing required field %q", field)
		}
	}
	var discriminator struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
		t.Fatal(err)
	}
	if discriminator.Const != "ingen.sentinel-role-execution/v1" {
		t.Fatalf("role execution discriminator = %q, want ingen.sentinel-role-execution/v1", discriminator.Const)
	}
}

func TestNativeSessionJournalSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "native-session-v1.schema.json"))
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
		t.Fatalf("incomplete or open native session journal schema: %+v", schema)
	}
	for _, field := range []string{"schema", "origin", "enforcement", "assurance", "intent", "state", "events"} {
		if !contains(schema.Required, field) {
			t.Fatalf("native session journal schema is missing required field %q", field)
		}
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("native session journal schema is missing property %q", field)
		}
	}
	for field, want := range map[string]string{
		"schema":      "ingen.sentinel-native-session/v1",
		"origin":      "sentinel-launch-journal",
		"enforcement": "declaration-only",
		"assurance":   "unverified",
	} {
		var discriminator struct {
			Const string `json:"const"`
		}
		if err := json.Unmarshal(schema.Properties[field], &discriminator); err != nil {
			t.Fatal(err)
		}
		if discriminator.Const != want {
			t.Fatalf("native session journal %s discriminator = %q, want %q", field, discriminator.Const, want)
		}
	}
}

func TestSentinelPreflightSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "preflight-v1.schema.json"))
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
		t.Fatalf("incomplete or open Sentinel preflight schema: %+v", schema)
	}
	for _, field := range []string{"schema", "workspace_id", "workspace", "status", "assurance", "checks"} {
		if !contains(schema.Required, field) {
			t.Fatalf("Sentinel preflight schema is missing required field %q", field)
		}
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("Sentinel preflight schema is missing property %q", field)
		}
	}
	var discriminator struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
		t.Fatal(err)
	}
	if discriminator.Const != "ingen.sentinel-preflight/v1" {
		t.Fatalf("Sentinel preflight discriminator = %q, want ingen.sentinel-preflight/v1", discriminator.Const)
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
