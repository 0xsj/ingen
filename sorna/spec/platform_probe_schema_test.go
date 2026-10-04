package spec

import (
	"encoding/json"
	"os"
	"testing"

	"ingen/sorna/internal/platformprobe"
)

func TestLinuxPlatformProbeSchema(t *testing.T) {
	schema := compileSchema(t, "linux-platform-probe-v1.schema.json")
	data, err := json.Marshal(platformprobe.Probe(platformprobe.Options{}))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("produced platform diagnostic violates its schema: %v", err)
	}
	document["ready"] = true
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted a product readiness claim")
	}
	delete(document, "ready")
	document["landlock"] = map[string]any{"status": "available", "required_access_bits": []any{}}
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted available Landlock without an ABI or access-bit observations")
	}
}

func TestPublishedLinuxPlatformProbeReport(t *testing.T) {
	path := os.Getenv("INGEN_LINUX_PROBE_REPORT")
	if path == "" {
		t.Skip("set INGEN_LINUX_PROBE_REPORT to validate a kernel diagnostic artifact")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t, "linux-platform-probe-v1.schema.json").Validate(document); err != nil {
		t.Fatal(err)
	}
}
