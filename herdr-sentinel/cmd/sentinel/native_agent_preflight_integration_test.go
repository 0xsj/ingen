package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/roleexec"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestNativeBrokerPreflightFailureHasNoHerdrOrProjectSideEffects(t *testing.T) {
	fixture := newNativePreflightCLIFixture(t)

	// /usr/bin/false is a real executable used only as a deterministic failing
	// Codex fixture. The production diagnostic still invokes it through Sorna.
	if runtime.GOOS == "darwin" {
		if info, err := os.Stat("/usr/bin/false"); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			t.Skip("host lacks the deterministic executable fixture")
		}
	}
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("SENTINEL_PREFLIGHT_TEST_KEY", "synthetic-test-only-not-a-provider-key")
	stdout, stderr, exitCode := captureSentinelStreams(t, func() int {
		return run(nativeBrokerSpawnArgs(fixture, "/usr/bin/false"))
	})
	if stdout != "" {
		t.Fatalf("preflight diagnostic wrote to stdout: %q", stdout)
	}
	fixture.assertUnchanged(t)

	var report agentprobe.Report
	if err := json.Unmarshal([]byte(stderr), &report); err != nil {
		t.Fatalf("stderr did not contain the typed preflight JSON report: %v; stderr=%q", err, stderr)
	}
	if report.Scope != agentprobe.Scope || report.Schema != agentprobe.ReportSchema || report.Status == "supported" {
		t.Fatalf("unexpected preflight report: %#v", report)
	}
	if strings.Contains(stderr, "synthetic-test-only-not-a-provider-key") || strings.Contains(stderr, "INGEN_CODEX_BROKER_TOKEN") {
		t.Fatal("preflight report exposed credential material")
	}
	if runtime.GOOS == "darwin" {
		if exitCode != 1 || report.Status != "unsupported" || report.ExitCode == nil || *report.ExitCode != 1 || report.ReasonCode != "codex-startup-failed" {
			t.Fatalf("failing Codex fixture did not stop as unsupported before dispatch: exit=%d report=%#v", exitCode, report)
		}
	} else if exitCode != 2 || report.Status != "indeterminate" {
		t.Fatalf("unsupported host did not fail closed as indeterminate: exit=%d report=%#v", exitCode, report)
	}
}

func TestNativeBrokerIndeterminatePreflightStopsBeforeHerdrAndProjectWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Sorna role preparation requires its macOS host-enforcement backend")
	}
	fixture := newNativePreflightCLIFixture(t)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("SENTINEL_PREFLIGHT_TEST_KEY", "synthetic-test-only-not-a-provider-key")
	executable, digest, err := roleexec.ToolIdentity("/usr/bin/false")
	if err != nil {
		t.Skipf("host lacks the deterministic executable fixture: %v", err)
	}
	diagnose := func(_ context.Context, request agentprobe.Request) (agentprobe.Report, error) {
		report := supportedReadinessFixture(executable, digest)
		report.Status = "indeterminate"
		report.ExitCode = nil
		report.ReplyObserved = false
		report.ReasonCode = "execution-canceled-or-timed-out"
		report.Reason = "diagnostic execution did not finish within its bounded run"
		if request.ExecutablePath != executable {
			t.Fatalf("injected bounded probe received unexpected executable path %q", request.ExecutablePath)
		}
		return report, nil
	}
	preflight := func(ctx context.Context, path string) (agentprobe.Report, error) {
		return checkNativeCodexPreflight(ctx, path, diagnose)
	}
	stdout, stderr, exitCode := captureSentinelStreams(t, func() int {
		return spawnSessionCommandWithPreflight(nativeBrokerSpawnArgs(fixture, executable)[2:], preflight)
	})
	if stdout != "" {
		t.Fatalf("indeterminate diagnostic wrote to stdout: %q", stdout)
	}
	fixture.assertUnchanged(t)
	var report agentprobe.Report
	if err := json.Unmarshal([]byte(stderr), &report); err != nil {
		t.Fatalf("stderr did not contain the typed preflight JSON report: %v; stderr=%q", err, stderr)
	}
	if exitCode != 2 || report.Status != "indeterminate" || report.ReasonCode != "execution-canceled-or-timed-out" {
		t.Fatalf("indeterminate preflight did not fail closed: exit=%d report=%#v", exitCode, report)
	}
	if strings.Contains(stderr, "synthetic-test-only-not-a-provider-key") {
		t.Fatal("indeterminate preflight report exposed credential material")
	}
}

func TestNativeBrokerInvalidPromptFailsBeforeDiagnosticAndSideEffects(t *testing.T) {
	fixture := newNativePreflightCLIFixture(t)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("SENTINEL_PREFLIGHT_TEST_KEY", "synthetic-test-only-not-a-provider-key")
	called := false
	preflight := func(ctx context.Context, path string) (agentprobe.Report, error) {
		called = true
		_, digest, err := roleexec.ToolIdentity(path)
		if err != nil {
			t.Fatalf("unexpected executable identity error after invalid prompt: %v", err)
		}
		return checkNativeCodexPreflight(ctx, path, func(context.Context, agentprobe.Request) (agentprobe.Report, error) {
			return supportedReadinessFixture(path, digest), nil
		})
	}
	args := nativeBrokerSpawnArgs(fixture, "/usr/bin/false")
	for index := range args {
		if args[index] == "docs/preflight.prompt" {
			args[index] = ".ingen/brief.md" // Not in the implementation role's declared read roots.
			break
		}
	}
	stdout, stderr, exitCode := captureSentinelStreams(t, func() int {
		return spawnSessionCommandWithPreflight(args[2:], preflight)
	})
	if called {
		t.Fatal("readiness diagnostic ran before selected prompt capabilities were validated")
	}
	if stdout != "" || exitCode == 0 || strings.Contains(stderr, agentprobe.ReportSchema) {
		t.Fatalf("invalid prompt unexpectedly passed preflight: exit=%d stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	fixture.assertUnchanged(t)
}

type nativePreflightCLIFixture struct {
	root         string
	receiptPath  string
	receiptBytes []byte
	socketPath   string
	listener     net.Listener
	connections  atomic.Int32
	acceptDone   chan struct{}
	before       map[string]string
}

func newNativePreflightCLIFixture(t *testing.T) *nativePreflightCLIFixture {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: "native-readiness-preflight"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "preflight.prompt"), []byte("A synthetic prompt for the failed native readiness gate.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, ".ingen", "artifacts", "sessions", "preflight-receipt.json")
	receipt, err := sentinelrun.NewUnderRoot(root, project.WorkspacePath, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := snapshotProjectTree(root)
	if err != nil {
		t.Fatal(err)
	}
	socketDir, err := os.MkdirTemp("/tmp", "herdr-preflight-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "h.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &nativePreflightCLIFixture{
		root: root, receiptPath: receiptPath, receiptBytes: receiptBytes, socketPath: socketPath,
		listener: listener, acceptDone: make(chan struct{}), before: before,
	}
	go func() {
		defer close(fixture.acceptDone)
		for {
			connection, acceptErr := fixture.listener.Accept()
			if acceptErr != nil {
				return
			}
			fixture.connections.Add(1)
			_ = connection.Close()
		}
	}()
	t.Cleanup(func() {
		_ = fixture.listener.Close()
		<-fixture.acceptDone
	})
	return fixture
}

func (f *nativePreflightCLIFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	if err := f.listener.Close(); err != nil && !strings.Contains(err.Error(), "closed network connection") {
		t.Fatal(err)
	}
	<-f.acceptDone
	if f.connections.Load() != 0 {
		t.Fatalf("preflight failure contacted Herdr %d times", f.connections.Load())
	}
	after, err := snapshotProjectTree(f.root)
	if err != nil {
		t.Fatal(err)
	}
	if !equalTreeSnapshot(f.before, after) {
		t.Fatalf("failed preflight changed project tree\nbefore=%v\nafter=%v", f.before, after)
	}
	receiptAfter, err := os.ReadFile(f.receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(receiptAfter, f.receiptBytes) {
		t.Fatal("failed preflight changed lifecycle receipt bytes")
	}
	for _, path := range []string{
		filepath.Join(f.root, ".ingen", "artifacts", "native-sessions"),
		filepath.Join(f.root, ".ingen", "artifacts", "role-executions"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("failed preflight created output path %s (err=%v)", path, err)
		}
	}
}

func nativeBrokerSpawnArgs(fixture *nativePreflightCLIFixture, executable string) []string {
	return []string{
		"session", "spawn", "--root", fixture.root, "--workspace", project.WorkspacePath,
		"--receipt", filepath.ToSlash(filepath.Join(".ingen", "artifacts", "sessions", "preflight-receipt.json")),
		"--role", "implementation", "--provider", "herdr", "--socket", fixture.socketPath,
		"--isolate", "--agent", "codex", "--agent-executable", executable,
		"--prompt-file", "docs/preflight.prompt", "--agent-provider", "openai-broker",
		"--agent-model", "synthetic-preflight-model", "--agent-credential-env", "SENTINEL_PREFLIGHT_TEST_KEY",
	}
}

func captureSentinelStreams(t *testing.T, runCommand func() int) (string, string, int) {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutWrite, stderrWrite
	code := runCommand()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	if err := stdoutWrite.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrWrite.Close(); err != nil {
		t.Fatal(err)
	}
	stdoutBytes, outErr := io.ReadAll(stdoutRead)
	stderrBytes, errErr := io.ReadAll(stderrRead)
	_ = stdoutRead.Close()
	_ = stderrRead.Close()
	if outErr != nil || errErr != nil {
		t.Fatalf("read captured CLI output: stdout=%v stderr=%v", outErr, errErr)
	}
	return string(stdoutBytes), string(stderrBytes), code
}

func snapshotProjectTree(root string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		if info.IsDir() {
			result[key] = "dir:" + info.Mode().String()
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			result[key] = "symlink:" + target
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		result[key] = "file:" + info.Mode().String() + ":" + hex.EncodeToString(digest[:])
		return nil
	})
	return result, err
}

func equalTreeSnapshot(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	keys := make([]string, 0, len(left))
	for key := range left {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if right[key] != left[key] {
			return false
		}
	}
	return true
}
