package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/nativejournal"
)

func compileNativeBoundarySchema(t *testing.T, filename string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func boundaryDocument(t *testing.T, value any) map[string]any {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestPublishedCodexContextMatchesSchema(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, ".ingen", "artifacts", "role-executions", ".runtime", "fixture-launch")
	context, _, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: "fixture-launch", Root: root, WorkingDir: working,
		HomePath: filepath.Join(working, "rw", "home"), CodexHome: filepath.Join(working, "rw", "codex"),
		PromptSourcePath: "contract/prompt.txt", PromptSourceSHA256: strings.Repeat("a", 64),
		PromptSnapshotPath:   ".ingen/artifacts/role-executions/fixture-launch.prompt",
		PromptSnapshotSHA256: strings.Repeat("a", 64), PromptBytes: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	schema := compileNativeBoundarySchema(t, "agent-context-v1.schema.json")
	document := boundaryDocument(t, context)
	if err := schema.Validate(document); err != nil {
		t.Fatalf("published Codex context rejected by its schema: %v", err)
	}
	// Keep the self-contained report schema and the standalone context schema
	// identical without requiring a network resolver to validate a report.
	reportBytes, err := os.ReadFile("role-execution-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	contextBytes, err := os.ReadFile("agent-context-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var reportSchema, contextSchema map[string]any
	if err := json.Unmarshal(reportBytes, &reportSchema); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contextBytes, &contextSchema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reportSchema["$defs"].(map[string]any)["agentContext"], contextSchema) {
		t.Fatal("embedded agent context schema differs from published standalone schema")
	}
	compileNativeBoundarySchema(t, "role-execution-v1.schema.json")
	for _, field := range []string{"prompt_snapshot_sha256", "codex_home_path", "args", "stdin"} {
		invalid := boundaryDocument(t, context)
		delete(invalid, field)
		if err := schema.Validate(invalid); err == nil {
			t.Fatalf("context schema accepted missing %s", field)
		}
	}
	invalid := boundaryDocument(t, context)
	invalid["network_mode"] = "unrestricted"
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("context schema accepted unrestricted networking")
	}
}

func TestPublishedNativeJournalsWithAndWithoutLeaseMatchSchema(t *testing.T) {
	schema := compileNativeBoundarySchema(t, "native-session-v1.schema.json")
	for _, lease := range []string{"", ".ingen/artifacts/native-sessions/native-fixture.lease"} {
		intent := nativejournal.Intent{
			SessionID: "native-fixture", RunID: "fixture-run", WorkspaceID: "fixture-workspace", WorkspaceVersion: 1,
			WorkspaceManifestSHA256: strings.Repeat("b", 64), RoleID: "fixture-role", RoleKind: "bootstrap",
			Workdir: "role", ReceiptPath: ".ingen/receipt.json", Argv: []string{"/usr/bin/true"},
			StdoutPath:         ".ingen/artifacts/native-sessions/native-fixture.stdout.log",
			StderrPath:         ".ingen/artifacts/native-sessions/native-fixture.stderr.log",
			ExecutionLeasePath: lease, HerdrSocket: "/private/tmp/fixture-herdr.sock",
			SentinelExecutable: "/usr/bin/true", SentinelExecutableSHA256: strings.Repeat("c", 64),
		}
		record, err := nativejournal.New(intent, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := nativejournal.WriteJSON(&output, record); err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(output.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(document); err != nil {
			t.Fatalf("published native journal (lease %q) rejected by its schema: %v", lease, err)
		}
	}
}

func TestPublishedBrokerContextAndStatisticsRejectMixedProfilesAndSecrets(t *testing.T) {
	root := t.TempDir()
	context, _, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: "broker-schema-fixture", Root: root, WorkingDir: root,
		HomePath: filepath.Join(root, "home"), CodexHome: filepath.Join(root, "codex"),
		PromptSourcePath: "prompt.txt", PromptSourceSHA256: strings.Repeat("a", 64),
		PromptSnapshotPath: "snapshot.txt", PromptSnapshotSHA256: strings.Repeat("a", 64), PromptBytes: 12,
		Model: "fixture-model", Broker: &agentlaunch.BrokerContext{
			Provider: "openai-responses", Endpoint: "http://127.0.0.1:54321/v1", CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY",
			MaxRequests: 1, MaxOutputTokens: 64, TimeoutSeconds: 30,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	schema := compileNativeBoundarySchema(t, "agent-context-v1.schema.json")
	if err := schema.Validate(boundaryDocument(t, context)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(d map[string]any) { d["network_mode"] = "disabled" },
		func(d map[string]any) { delete(d, "broker") },
		func(d map[string]any) { delete(d, "model") },
		func(d map[string]any) { d["broker"].(map[string]any)["api_key"] = "synthetic-secret" },
		func(d map[string]any) { d["broker"].(map[string]any)["endpoint"] = "https://api.openai.com/v1" },
	} {
		d := boundaryDocument(t, context)
		mutate(d)
		if err := schema.Validate(d); err == nil {
			t.Fatal("broker context schema accepted a mixed profile or unsafe field")
		}
	}
	// The public statistics contract forbids credential/body fields and cannot
	// represent an unsettled server as closed. Counter relationships and exact
	// endpoint/policy matching are additionally checked by the evidence verifier.
	compiler := jsonschema.NewCompiler()
	statsSchema, err := compiler.Compile("role-execution-v1.schema.json#/$defs/brokerExecution")
	if err != nil {
		t.Fatal(err)
	}
	execution := map[string]any{
		"status": "closed", "provider": "openai-responses", "endpoint": "http://127.0.0.1:54321/v1", "model": "fixture-model",
		"max_requests": 1, "max_output_tokens": 64, "timeout_seconds": 30,
		"stats": map[string]any{"started_at": "2026-10-04T00:00:00Z", "closed_at": "2026-10-04T00:00:01Z", "requests_received": 1, "requests_forwarded": 1, "requests_rejected": 0, "upstream_failures": 0, "in_flight": 0, "last_status": 200},
	}
	if err := statsSchema.Validate(execution); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(d map[string]any) { d["token"] = "synthetic-secret" },
		func(d map[string]any) { d["stats"].(map[string]any)["request_body"] = "synthetic-prompt" },
		func(d map[string]any) { d["stats"].(map[string]any)["in_flight"] = 1 },
		func(d map[string]any) { delete(d["stats"].(map[string]any), "closed_at") },
	} {
		d := boundaryDocument(t, execution)
		mutate(d)
		if err := statsSchema.Validate(d); err == nil {
			t.Fatal("broker statistics schema accepted unsafe fields or unsettled closed execution")
		}
	}
}
