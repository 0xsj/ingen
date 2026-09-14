package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ingen/sorna/internal/policy"
)

func TestPrepareResolvesPolicyRootsAndWrapsCommand(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	root := t.TempDir()
	sealed, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{os.Args[0], "-test.run=TestSandboxHelperProcess"}, root, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Enforcement != "host-enforced" || prepared.PolicySHA256 != sealed.SHA256 || prepared.SubjectID != "sandbox-test" || prepared.ExecutablePath == "" || len(prepared.ExecutableSHA256) != 64 {
		t.Fatalf("prepared = %+v, want host enforcement, policy hash, subject identity, and executable digest", prepared)
	}
	if len(prepared.Command) < 4 || prepared.Command[0] != "/usr/bin/sandbox-exec" {
		t.Fatalf("wrapped command = %q, want sandbox-exec prefix", prepared.Command)
	}
	if !filepath.IsAbs(prepared.Command[3]) {
		t.Fatalf("prepared executable argument = %q, want an absolute path", prepared.Command[3])
	}
}

func TestPrepareRestrictsProcessExecToCommandAndAllowedTools(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	document := testPolicy()
	document.Policy["process"] = map[string]any{
		"subject_id":         "sandbox-test",
		"can_invoke_subject": false,
		"allowed_tools": []any{map[string]any{
			"name":    "cat",
			"purpose": "test helper",
		}},
	}
	sealed, err := policy.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{"/bin/echo"}, t.TempDir(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	profile := prepared.Command[2]
	if !strings.Contains(profile, `(allow process-exec (literal "/bin/echo"))`) || !strings.Contains(profile, `(allow process-exec (literal "/bin/cat"))`) {
		t.Fatalf("Seatbelt profile = %s, want command and declared tool exec rules", profile)
	}
	if strings.Contains(profile, "(allow process-exec)\n") || prepared.CanInvokeSubject {
		t.Fatalf("prepared = %+v; want restricted process execution and no subject invocation", prepared)
	}
}

func TestSandboxDeniesUnlistedExecHelperProcess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	sealed, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{os.Args[0], "-test.run=TestSandboxUnlistedExecHelperProcess", "--"}, t.TempDir(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	command.Env = append(os.Environ(), "INGEN_SANDBOX_UNLISTED_EXEC=1")
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "unlisted exec denied") {
		t.Fatalf("sandbox helper = %v; output=%s, want denied unlisted exec", err, output)
	}
}

func TestSandboxUnlistedExecHelperProcess(t *testing.T) {
	if os.Getenv("INGEN_SANDBOX_UNLISTED_EXEC") != "1" {
		return
	}
	if output, err := exec.Command("/bin/echo", "unexpected").CombinedOutput(); err == nil {
		fmt.Fprintf(os.Stderr, "unlisted exec unexpectedly succeeded: %s", output)
		os.Exit(1)
	}
	fmt.Println("unlisted exec denied")
}

func TestSandboxDeniesLateUnlistedExec(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	root := t.TempDir()
	sealed, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{os.Args[0], "-test.run=TestSandboxLateUnlistedExecHelper", "--"}, root, sealed)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	command.Dir = root
	command.Env = append(os.Environ(), "INGEN_SANDBOX_LATE_EXEC=1")
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyProcessExecutableEventually(context.Background(), command.Process.Pid, prepared.ExecutablePath, prepared.ExecutableSHA256, time.Second); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("initial process identity verification = %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("late unlisted exec helper = %v; output=%s", err, output.String())
	}
	if !strings.Contains(output.String(), "late unlisted exec denied") {
		t.Fatalf("late unlisted exec output = %q, want denial", output.String())
	}
}

func TestSandboxLateUnlistedExecHelper(t *testing.T) {
	if os.Getenv("INGEN_SANDBOX_LATE_EXEC") != "1" {
		return
	}
	time.Sleep(100 * time.Millisecond)
	if output, err := exec.Command("/bin/echo", "unexpected").CombinedOutput(); err == nil {
		fmt.Fprintf(os.Stderr, "late unlisted exec unexpectedly succeeded: %s", output)
		os.Exit(1)
	}
	fmt.Println("late unlisted exec denied")
}

func TestSandboxEnforcesFilesystemProbe(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	root := t.TempDir()
	inputDir := filepath.Join(root, "contract")
	secretDir := filepath.Join(root, "implementation")
	for _, directory := range []string{inputDir, secretDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	allowedPath := filepath.Join(inputDir, "public.txt")
	deniedPath := filepath.Join(secretDir, "private.txt")
	if err := os.WriteFile(allowedPath, []byte("public"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deniedPath, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	sealed, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{"/bin/cat", allowedPath}, root, sealed)
	if err != nil {
		t.Fatal(err)
	}
	allowedCommand := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	allowedCommand.Dir = root
	allowedOutput, err := allowedCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("allowed sandbox read = %v; output=%s; command=%q", err, allowedOutput, prepared.Command)
	}
	if string(allowedOutput) != "public" {
		t.Fatalf("allowed sandbox output = %q, want public", allowedOutput)
	}

	prepared, err = Prepare([]string{"/bin/cat", deniedPath}, root, sealed)
	if err != nil {
		t.Fatal(err)
	}
	deniedCommand := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	deniedCommand.Dir = root
	deniedOutput, err := deniedCommand.CombinedOutput()
	if err == nil || (!strings.Contains(string(deniedOutput), "Operation not permitted") && !strings.Contains(string(deniedOutput), "Permission denied")) {
		t.Fatalf("denied sandbox read = %v; output=%s, want OS denial", err, deniedOutput)
	}
}

func TestSandboxRejectsExecutableReplacementAfterPrepare(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	root := t.TempDir()
	target := filepath.Join(root, "candidate-sleep")
	original, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	sealed, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{target, "-test.run=TestSandboxReplacementHelper"}, root, sealed)
	if err != nil {
		t.Fatal(err)
	}
	mutated := append(append([]byte(nil), original...), []byte("\nreplacement")...)
	if err := os.WriteFile(target, mutated, 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	command.Dir = root
	command.Env = append(os.Environ(), "INGEN_SANDBOX_REPLACEMENT=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()
	_, verifyErr := VerifyProcessExecutableEventually(context.Background(), command.Process.Pid, prepared.ExecutablePath, prepared.ExecutableSHA256, 500*time.Millisecond)
	if verifyErr == nil || !strings.Contains(verifyErr.Error(), "identity mismatch") {
		t.Fatalf("VerifyProcessExecutableEventually() = %v, want replacement identity mismatch", verifyErr)
	}
	_ = command.Process.Kill()
}

func TestSandboxReplacementHelper(t *testing.T) {
	if os.Getenv("INGEN_SANDBOX_REPLACEMENT") != "1" {
		return
	}
	time.Sleep(2 * time.Second)
}

func TestPrepareRejectsUnrestrictedNetworkMode(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	document := testPolicy()
	document.Policy["network"] = map[string]any{
		"mode": "unrestricted",
	}
	sealed, err := policy.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare([]string{"/bin/cat"}, t.TempDir(), sealed); err == nil || !strings.Contains(err.Error(), "network.mode=unrestricted") {
		t.Fatalf("Prepare error = %v, want unsupported network mode", err)
	}
}

func TestPrepareAppliesDirectionalNetworkAllowlist(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host enforcement probe is macOS-specific")
	}
	document := testPolicy()
	document.Policy["network"] = map[string]any{
		"mode": "allowlist",
		"allow": []any{map[string]any{
			"host":      "localhost",
			"ports":     []any{int64(8080)},
			"direction": "inbound",
			"purpose":   "test listener",
		}},
	}
	sealed, err := policy.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare([]string{"/bin/cat"}, t.TempDir(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	profile := prepared.Command[2]
	for _, expected := range []string{
		`(allow network-bind (local tcp "localhost:8080"))`,
		`(allow network-inbound (local tcp "localhost:8080"))`,
	} {
		if !strings.Contains(profile, expected) {
			t.Fatalf("Seatbelt profile = %s, want %q", profile, expected)
		}
	}
	if strings.Contains(profile, "network-outbound") || prepared.NetworkMode != "allowlist" {
		t.Fatalf("prepared = %+v; want inbound-only allowlist", prepared)
	}
}

func TestPathCoveredUsesCapabilityRoots(t *testing.T) {
	root := t.TempDir()
	if !PathCovered([]string{filepath.Join(root, "contract")}, filepath.Join(root, "contract", "case.json")) {
		t.Fatal("PathCovered rejected a path inside the capability root")
	}
	if PathCovered([]string{filepath.Join(root, "contract")}, filepath.Join(root, "implementation", "source.go")) {
		t.Fatal("PathCovered accepted a path outside the capability root")
	}
}

func testPolicy() policy.Document {
	return policy.Document{Policy: map[string]any{
		"schema":      "ingen.policy/v1",
		"id":          "sandbox-test",
		"version":     int64(1),
		"status":      "draft",
		"purpose":     "sandbox-test",
		"enforcement": "declared-only",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": ".", "reason": "public input root"}},
			"write": []any{map[string]any{"path": "output", "reason": "probe output"}},
			"deny":  []any{map[string]any{"path": "implementation", "reason": "private implementation"}},
		},
		"network": map[string]any{"mode": "disabled"},
		"process": map[string]any{"subject_id": "sandbox-test", "can_invoke_subject": false},
	}}
}
