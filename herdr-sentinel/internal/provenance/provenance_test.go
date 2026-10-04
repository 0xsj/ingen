package provenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	amber "github.com/0xsj/ingen/amber"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestStartWritesValidAmberRootContextExclusively(t *testing.T) {
	root := t.TempDir()
	value, expectedHash, err := Start(root, ".ingen/provenance/root.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".ingen/provenance/root.json"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := amber.FromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ExecutionID() != value.ExecutionID() || digest(raw) != expectedHash {
		t.Fatal("root context did not preserve Amber identity or exact digest")
	}
	if _, _, err := Start(root, ".ingen/provenance/root.json"); err == nil {
		t.Fatal("start replaced an existing Amber context")
	}
}

func TestExecuteCapturesCompletedAndFailedArgvWithoutShellExpansion(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		root, parentPath := newParent(t)
		t.Setenv("SENTINEL_PROVENANCE_HELPER", "print")
		request := executeRequest(root, parentPath, "http-document-fetch", helperCommand())
		receipt, err := Execute(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Status != "completed" || receipt.ExitCode == nil || *receipt.ExitCode != 0 || receipt.Enforcement != "not-isolated" || receipt.Assurance != "unverified" {
			t.Fatalf("receipt = %+v", receipt)
		}
		if strings.Join(receipt.Argv, " ") != strings.Join(helperCommand(), " ") {
			t.Fatalf("argv changed: %v", receipt.Argv)
		}
		if got := readFile(t, filepath.Join(root, receipt.StdoutPath)); !strings.HasPrefix(got, "hello\n") {
			t.Fatalf("stdout = %q", got)
		}
		childBytes := readBytes(t, filepath.Join(root, receipt.ContextPath))
		child, err := amber.FromJSON(childBytes)
		if err != nil {
			t.Fatal(err)
		}
		parentBytes := readBytes(t, filepath.Join(root, parentPath))
		parent, _ := amber.FromJSON(parentBytes)
		cause, ok := child.Causation()
		if !ok || cause.ID != parent.ExecutionID() || child.Depth() != parent.Depth()+1 {
			t.Fatalf("child lineage does not use Amber transition: cause=%+v depth=%d", cause, child.Depth())
		}
		receiptBytes := readBytes(t, filepath.Join(root, receipt.ReceiptPath))
		var persisted Receipt
		if err := json.Unmarshal(receiptBytes, &persisted); err != nil || persisted.ContextSHA256 != receipt.ContextSHA256 {
			t.Fatalf("persisted execution receipt mismatch: err=%v receipt=%+v", err, persisted)
		}
		validateReceiptSchema(t, receiptBytes)
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("duplicate execution claim succeeded")
		}
		if got := readBytes(t, filepath.Join(root, receipt.ReceiptPath)); !bytes.Equal(got, receiptBytes) {
			t.Fatal("duplicate execution changed the original receipt")
		}
	})
	t.Run("nonzero", func(t *testing.T) {
		root, parentPath := newParent(t)
		t.Setenv("SENTINEL_PROVENANCE_HELPER", "fail")
		receipt, err := Execute(context.Background(), executeRequest(root, parentPath, "test-failure", helperCommand()))
		if err == nil || receipt.Status != "failed" || receipt.ExitCode == nil || *receipt.ExitCode != 7 {
			t.Fatalf("failed child outcome lost: receipt=%+v err=%v", receipt, err)
		}
		if !strings.Contains(readFile(t, filepath.Join(root, receipt.StderrPath)), "helper failure") {
			t.Fatal("failed child stderr was not captured")
		}
		validateReceiptSchema(t, readBytes(t, filepath.Join(root, receipt.ReceiptPath)))
	})
}

func TestExecuteCanceledChildPreservesActualExit(t *testing.T) {
	root, parentPath := newParent(t)
	t.Setenv("SENTINEL_PROVENANCE_HELPER", "wait")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	request := executeRequest(root, parentPath, "cancel-test", helperCommand())
	type outcome struct {
		receipt Receipt
		err     error
	}
	result := make(chan outcome, 1)
	go func() {
		receipt, err := Execute(ctx, request)
		result <- outcome{receipt: receipt, err: err}
	}()
	stdoutPath := filepath.Join(root, request.ReceiptPath+".stdout")
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(readFileIfExists(stdoutPath), "ready") })
	cancel()
	select {
	case got := <-result:
		if got.err == nil || got.receipt.Status != "canceled" || got.receipt.ExitCode == nil || *got.receipt.ExitCode != 0 {
			t.Fatalf("canceled child outcome = %+v err=%v", got.receipt, got.err)
		}
		validateReceiptSchema(t, readBytes(t, filepath.Join(root, got.receipt.ReceiptPath)))
	case <-time.After(6 * time.Second):
		t.Fatal("canceled child did not settle within bounded wait")
	}
}

func TestExecuteRejectsInvalidParentAndCollisionsBeforeLaunch(t *testing.T) {
	t.Run("digest mismatch", func(t *testing.T) {
		root, parentPath := newParent(t)
		t.Setenv("SENTINEL_PROVENANCE_HELPER", "print")
		request := executeRequest(root, parentPath, "pin-test", helperCommand())
		request.ExpectedParentSHA256 = strings.Repeat("0", 64)
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("accepted mismatched parent digest")
		}
		assertAbsent(t, root, request.OutputPath, request.ReceiptPath, request.ReceiptPath+".stdout", request.ReceiptPath+".stderr")
	})
	t.Run("duplicate receipt", func(t *testing.T) {
		root, parentPath := newParent(t)
		request := executeRequest(root, parentPath, "collision-test", helperCommand())
		write(t, filepath.Join(root, request.ReceiptPath), []byte("existing"))
		t.Setenv("SENTINEL_PROVENANCE_HELPER", "print")
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("accepted existing receipt path")
		}
		assertAbsent(t, root, request.OutputPath, request.ReceiptPath+".stdout", request.ReceiptPath+".stderr")
	})
	t.Run("malformed and duplicate parent JSON", func(t *testing.T) {
		root, parentPath := newParent(t)
		parentFile := filepath.Join(root, parentPath)
		raw := readBytes(t, parentFile)
		duplicate := append([]byte("{\"version\":1,"), raw[1:]...)
		write(t, parentFile, duplicate)
		request := executeRequest(root, parentPath, "bad-parent", helperCommand())
		t.Setenv("SENTINEL_PROVENANCE_HELPER", "print")
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("accepted duplicate-key parent")
		}
		assertAbsent(t, root, request.OutputPath, request.ReceiptPath, request.ReceiptPath+".stdout", request.ReceiptPath+".stderr")
	})
	t.Run("unknown nested Amber field and null depth", func(t *testing.T) {
		root, parentPath := newParent(t)
		parentFile := filepath.Join(root, parentPath)
		raw := string(readBytes(t, parentFile))
		changed := strings.Replace(raw, "\"mode\":{\"kind\":\"normal\"}", "\"mode\":{\"kind\":\"normal\",\"unexpected\":true}", 1)
		if changed == raw {
			t.Fatalf("parent mode field layout changed: %s", raw)
		}
		write(t, parentFile, []byte(changed))
		request := executeRequest(root, parentPath, "bad-mode", helperCommand())
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("accepted unknown nested Amber field")
		}
		changed = strings.Replace(raw, "\"depth\":0", "\"depth\":null", 1)
		write(t, parentFile, []byte(changed))
		request = executeRequest(root, parentPath, "bad-depth", helperCommand())
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("accepted null Amber depth")
		}
	})
	t.Run("trailing parent JSON", func(t *testing.T) {
		root, parentPath := newParent(t)
		parentFile := filepath.Join(root, parentPath)
		write(t, parentFile, append(readBytes(t, parentFile), []byte(" {}")...))
		request := executeRequest(root, parentPath, "trailing-parent", helperCommand())
		if _, err := Execute(context.Background(), request); err == nil {
			t.Fatal("accepted trailing JSON after Amber parent")
		}
		assertAbsent(t, root, request.OutputPath, request.ReceiptPath, request.ReceiptPath+".stdout", request.ReceiptPath+".stderr")
	})
}

func TestExecuteRejectsPathEscapeAndAlreadyCanceledContext(t *testing.T) {
	root, parentPath := newParent(t)
	request := executeRequest(root, parentPath, "escape-test", helperCommand())
	request.OutputPath = "../outside.json"
	t.Setenv("SENTINEL_PROVENANCE_HELPER", "print")
	if _, err := Execute(context.Background(), request); err == nil {
		t.Fatal("accepted output escaping project root")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request = executeRequest(root, parentPath, "pre-canceled", helperCommand())
	receipt, err := Execute(ctx, request)
	if err == nil || receipt.Status != "canceled" || receipt.ExitCode != nil {
		t.Fatalf("pre-canceled command did not record no-launch cancellation: %+v err=%v", receipt, err)
	}
}

func TestExecuteResolvesSlashRelativeExecutableAgainstProjectRoot(t *testing.T) {
	root, parentPath := newParent(t)
	toolPath := filepath.Join(root, "relative-tool")
	write(t, toolPath, []byte("#!/bin/sh\nprintf 'from-project-root\\n'\n"))
	if err := os.Chmod(toolPath, 0o700); err != nil {
		t.Fatal(err)
	}
	request := executeRequest(root, parentPath, "relative-tool", []string{"./relative-tool"})
	receipt, err := Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, receipt.StdoutPath)); !strings.Contains(got, "from-project-root") {
		t.Fatalf("project-relative executable ran with unexpected stdout %q", got)
	}
}

func TestExecutePassesShellMetacharactersAsLiteralArgv(t *testing.T) {
	root, parentPath := newParent(t)
	marker := filepath.Join(root, "must-not-exist")
	argument := "$(touch " + marker + ")"
	request := executeRequest(root, parentPath, "literal-argv", []string{"/bin/echo", argument})
	receipt, err := Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shell metacharacters were interpreted (stat error %v)", err)
	}
	if output := readFile(t, filepath.Join(root, receipt.StdoutPath)); strings.TrimSpace(output) != argument {
		t.Fatalf("argv was changed or interpreted: %q", output)
	}
}

func TestExecuteReceiptReservationTamperIsIndeterminate(t *testing.T) {
	root, parentPath := newParent(t)
	request := executeRequest(root, parentPath, "tamper-receipt", helperCommand())
	t.Setenv("SENTINEL_PROVENANCE_HELPER", "tamper-receipt")
	t.Setenv("SENTINEL_PROVENANCE_RECEIPT", filepath.Join(root, request.ReceiptPath))
	receipt, err := Execute(context.Background(), request)
	if err == nil || receipt.Status != "indeterminate" || receipt.ExitCode == nil || *receipt.ExitCode != 0 {
		t.Fatalf("receipt tampering not preserved as indeterminate with actual child exit: %+v err=%v", receipt, err)
	}
}

func TestExecuteDetectsContextTamperingAfterChildExits(t *testing.T) {
	for _, test := range []struct {
		name string
		env  string
		path func(root, parent string, receipt Receipt) string
	}{
		{
			name: "child context",
			env:  "SENTINEL_PROVENANCE_CHILD_CONTEXT",
			path: func(root, _ string, receipt Receipt) string { return filepath.Join(root, receipt.ContextPath) },
		},
		{
			name: "parent context",
			env:  "SENTINEL_PROVENANCE_PARENT_CONTEXT",
			path: func(root, parent string, _ Receipt) string { return filepath.Join(root, parent) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, parentPath := newParent(t)
			t.Setenv("SENTINEL_PROVENANCE_HELPER", "tamper-context")
			request := executeRequest(root, parentPath, "context-integrity", helperCommand())
			t.Setenv(test.env, test.path(root, parentPath, Receipt{ContextPath: request.OutputPath}))
			receipt, err := Execute(context.Background(), request)
			if err == nil || receipt.Status != "indeterminate" || receipt.ExitCode == nil || *receipt.ExitCode != 0 {
				t.Fatalf("context tampering was not detected while preserving actual child exit: receipt=%+v err=%v", receipt, err)
			}
			if !strings.Contains(receipt.Reason, "context lineage changed") {
				t.Fatalf("unexpected integrity reason: %q", receipt.Reason)
			}
			validateReceiptSchema(t, readBytes(t, filepath.Join(root, receipt.ReceiptPath)))
		})
	}
}

func TestProvenanceProcessHelper(t *testing.T) {
	switch os.Getenv("SENTINEL_PROVENANCE_HELPER") {
	case "print":
		_, _ = fmt.Fprintln(os.Stdout, "hello")
	case "fail":
		_, _ = fmt.Fprintln(os.Stderr, "helper failure")
		os.Exit(7)
	case "wait":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		_, _ = fmt.Fprintln(os.Stdout, "ready")
		<-signals
		return
	case "tamper-receipt":
		if err := os.Remove(os.Getenv("SENTINEL_PROVENANCE_RECEIPT")); err != nil {
			os.Exit(9)
		}
	case "tamper-context":
		path := os.Getenv("SENTINEL_PROVENANCE_CHILD_CONTEXT")
		if path == "" {
			path = os.Getenv("SENTINEL_PROVENANCE_PARENT_CONTEXT")
		}
		if path == "" {
			os.Exit(10)
		}
		if err := os.WriteFile(path, []byte("tampered after launch\n"), 0o600); err != nil {
			os.Exit(11)
		}
	}
}

func helperCommand() []string {
	return []string{os.Args[0], "-test.run=^TestProvenanceProcessHelper$"}
}

func newParent(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	_, _, err := Start(root, ".ingen/provenance/root.json")
	if err != nil {
		t.Fatal(err)
	}
	return root, ".ingen/provenance/root.json"
}

func executeRequest(root, parent, operation string, command []string) Request {
	return Request{Root: root, ParentPath: parent, OutputPath: ".ingen/provenance/child.json", ReceiptPath: ".ingen/provenance/execution.json", Operation: operation, Command: command}
}

func waitFor(t *testing.T, duration time.Duration, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for helper child readiness")
}

func assertAbsent(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected artifact %s (err %v)", path, err)
		}
	}
}

func write(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func readFile(t *testing.T, path string) string { return string(readBytes(t, path)) }
func readFileIfExists(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestProvenanceReceiptSchemaContract(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "../../spec/provenance-execution-v1.schema.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ID                   string                     `json:"$id"`
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(contents, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.ID == "" || schema.Type != "object" || schema.AdditionalProperties {
		t.Fatalf("incomplete or open provenance receipt schema: %+v", schema)
	}
	for _, name := range []string{"schema", "parent_sha256", "context_sha256", "operation", "argv", "status", "enforcement", "assurance", "limitations"} {
		if _, ok := schema.Properties[name]; !ok || !containsString(schema.Required, name) {
			t.Fatalf("schema missing required field %q", name)
		}
	}
}

func validateReceiptSchema(t *testing.T, raw []byte) {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "../../spec/provenance-execution-v1.schema.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	const schemaURL = "https://ingen.dev/schemas/sentinel/provenance-execution-v1.schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(contents)); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("published receipt does not satisfy v1 schema: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
