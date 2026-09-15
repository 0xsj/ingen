package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCLIEndToEnd(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte(`{"schema":"ingen.ci-result/v1","status":"passed"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", "lockwood-cli-0001",
		"--media-type", "application/json",
		"--producer", "sorna",
		"--kind", "behavioral-verification",
		"--run-id", "run-0001",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	digest := record["artifact"].(map[string]any)["digest"].(string)

	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect", "--root", root, "lockwood-cli-0001"}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect exit code = %d", code)
	}
	if !strings.Contains(inspectOutput.String(), digest) {
		t.Fatalf("inspect output does not contain digest: %s", inspectOutput.String())
	}

	var findOutput bytes.Buffer
	if code := run([]string{"find", "--root", root, "--producer", "sorna"}, strings.NewReader(""), &findOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find exit code = %d", code)
	}
	if !strings.Contains(findOutput.String(), "lockwood-cli-0001") {
		t.Fatalf("find output does not contain custody ID: %s", findOutput.String())
	}

	var getOutput bytes.Buffer
	if code := run([]string{"get", "--root", root, digest}, strings.NewReader(""), &getOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("get exit code = %d", code)
	}
	if getOutput.String() != `{"schema":"ingen.ci-result/v1","status":"passed"}` {
		t.Fatalf("get output = %q", getOutput.String())
	}

	var verifyOutput bytes.Buffer
	if code := run([]string{"verify", "--root", root, digest}, strings.NewReader(""), &verifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify exit code = %d", code)
	}
	if strings.TrimSpace(verifyOutput.String()) != digest {
		t.Fatalf("verify output = %q", verifyOutput.String())
	}

	var custodyVerifyOutput bytes.Buffer
	if code := run([]string{"verify", "--root", root, "--id", "lockwood-cli-0001"}, strings.NewReader(""), &custodyVerifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("custody verify exit code = %d", code)
	}
	if !strings.Contains(custodyVerifyOutput.String(), "lockwood-cli-0001") {
		t.Fatalf("custody verify output = %s", custodyVerifyOutput.String())
	}

	var reconcileOutput bytes.Buffer
	if code := run([]string{"reconcile", "--root", root}, strings.NewReader(""), &reconcileOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile exit code = %d", code)
	}
	var report struct {
		Orphans            []any `json:"orphans"`
		DanglingReferences []any `json:"dangling_references"`
		CorruptBlobs       []any `json:"corrupt_blobs"`
	}
	if err := json.Unmarshal(reconcileOutput.Bytes(), &report); err != nil {
		t.Fatalf("decode reconcile output: %v", err)
	}
	if len(report.Orphans) != 0 || len(report.DanglingReferences) != 0 || len(report.CorruptBlobs) != 0 {
		t.Fatalf("reconcile report = %+v, want clean root", report)
	}
}

func TestCLIGetRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(input, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var putOutput bytes.Buffer
	if code := run([]string{"put", "--root", root, "--id", "lockwood-cli-overwrite", "--media-type", "text/plain", "--producer", "example", "--kind", "fixture", input}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	digest := record["artifact"].(map[string]any)["digest"].(string)
	output := filepath.Join(t.TempDir(), "output.txt")
	if err := os.WriteFile(output, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{"get", "--root", root, "--output", output, digest}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("get overwrote an existing output")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing output changed to %q", data)
	}
}

func TestCLIPutRemoteV2Source(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "remote-result.json")
	if err := os.WriteFile(input, []byte("remote bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--schema", "lockwood.custody/v2",
		"--id", "lockwood-cli-remote-v2",
		"--media-type", "application/json",
		"--producer", "sorna",
		"--kind", "remote-result",
		"--source-uri", "s3://evidence.example/runs/run-0001/result.json",
		"--source-version", "version-0001",
		input,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put remote v2 exit code = %d", code)
	}
	var record struct {
		Schema string `json:"schema"`
		Source struct {
			URI     string `json:"uri"`
			Version string `json:"version"`
		} `json:"source"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Schema != "lockwood.custody/v2" || record.Source.URI == "" || record.Source.Version == "" {
		t.Fatalf("remote v2 record = %+v", record)
	}
}

func TestCLIImportSorna(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "sorna-run")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"manifest.json": []byte("{\"schema\":\"sorna.evidence/v1\",\"run_id\":\"run-cli-0001\"}\n"),
		"run.json":      []byte("{\"schema\":\"ingen.run/v1\",\"run_id\":\"run-cli-0001\"}\n"),
	}
	checksums := make([]string, 0, len(files))
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(bundle, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		checksums = append(checksums, hex.EncodeToString(digest[:])+"  "+name)
	}
	sort.Strings(checksums)
	if err := os.WriteFile(filepath.Join(bundle, "checksums.sha256"), []byte(strings.Join(checksums, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := run([]string{"import-sorna", "--root", root, "--id", "lockwood-cli-sorna-0001", bundle}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-sorna exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["producer"].(map[string]any)["kind"] != "evidence-bundle" {
		t.Fatalf("producer = %+v", record["producer"])
	}
}
