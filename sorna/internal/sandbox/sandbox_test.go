package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	if prepared.Enforcement != "host-enforced" || prepared.PolicySHA256 != sealed.SHA256 {
		t.Fatalf("prepared = %+v, want host enforcement and policy hash", prepared)
	}
	if len(prepared.Command) < 4 || prepared.Command[0] != "/usr/bin/sandbox-exec" {
		t.Fatalf("wrapped command = %q, want sandbox-exec prefix", prepared.Command)
	}
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
		"process": map[string]any{"can_invoke_subject": false},
	}}
}
