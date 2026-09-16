package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/capability"
)

func TestPrepareOracleDelegatesToSornaWithBoundPolicy(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "workspace.yaml", "manifest")
	writeFile(t, "oracle-policy.yaml", "oracle policy")
	writeFile(t, "subject-policy.yaml", "subject policy")
	plan := capability.Plan{
		Schema: capability.Schema,
		Workspace: capability.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			Manifest: ciresult.FileRef{
				Path:   "workspace.yaml",
				SHA256: digest("manifest"),
			},
			OraclePolicy: ciresult.FileRef{
				Path:   "oracle-policy.yaml",
				SHA256: digest("oracle policy"),
			},
			SubjectPolicy: ciresult.FileRef{
				Path:   "subject-policy.yaml",
				SHA256: digest("subject policy"),
			},
		},
		ImplementationRoots: []string{"subject", "defects"},
		Enforcement:         "declaration-only",
		Assurance:           "unverified",
		Roles: []capability.Role{
			{
				ID:         "oracle-writer",
				Kind:       "oracle-writer",
				Workspace:  ".sentinel/oracle",
				ReadRoots:  []string{"contract.yaml"},
				WriteRoots: []string{".artifacts/oracle"},
				DenyRoots:  []string{"subject", "defects", ".git"},
			},
		},
	}

	prepared, err := PrepareOracle(plan, ".", []string{"/bin/cat", "contract.yaml"}, []string{"go", "run", "./sorna/cmd/sorna"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if prepared.Backend != "sorna-cli" || prepared.Enforcement != "pending-host-enforcement" {
		t.Fatalf("prepared = %+v, want explicit delegated enforcement state", prepared)
	}
	if !filepath.IsAbs(prepared.AppliedPolicyPath) {
		t.Fatalf("applied policy path = %q, want temporary absolute snapshot", prepared.AppliedPolicyPath)
	}
	want := []string{"go", "run", "./sorna/cmd/sorna", "sandbox", "exec", "--policy", prepared.AppliedPolicyPath, "--root", ".", "--", "/bin/cat", "contract.yaml"}
	if strings.Join(prepared.Command, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("command = %q, want %q", prepared.Command, want)
	}
	snapshot, err := os.ReadFile(prepared.AppliedPolicyPath)
	if err != nil || string(snapshot) != "oracle policy" {
		t.Fatalf("policy snapshot = %q, %v; want verified policy bytes", snapshot, err)
	}
	if prepared.Policy.SHA256 != digest("oracle policy") {
		t.Fatalf("policy = %+v, want bound oracle policy", prepared.Policy)
	}
}

func TestPrepareOracleRejectsPolicyDrift(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "workspace.yaml", "manifest")
	writeFile(t, "oracle-policy.yaml", "oracle policy")
	writeFile(t, "subject-policy.yaml", "subject policy")
	plan := capability.Plan{
		Schema: capability.Schema,
		Workspace: capability.WorkspaceRef{
			ID:       "webhook-validation",
			Version:  1,
			Manifest: ciresult.FileRef{Path: "workspace.yaml", SHA256: digest("manifest")},
			OraclePolicy: ciresult.FileRef{
				Path:   "oracle-policy.yaml",
				SHA256: digest("oracle policy"),
			},
			SubjectPolicy: ciresult.FileRef{
				Path:   "subject-policy.yaml",
				SHA256: digest("subject policy"),
			},
		},
		ImplementationRoots: []string{"subject"},
		Enforcement:         "declaration-only",
		Assurance:           "unverified",
		Roles: []capability.Role{{
			ID:         "oracle-writer",
			Kind:       "oracle-writer",
			Workspace:  ".sentinel/oracle",
			ReadRoots:  []string{"contract.yaml"},
			WriteRoots: []string{".artifacts/oracle"},
			DenyRoots:  []string{"subject"},
		}},
	}
	if err := os.WriteFile("oracle-policy.yaml", []byte("changed policy"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareOracle(plan, ".", []string{"/bin/cat"}, []string{"sorna"})
	if err == nil || !strings.Contains(err.Error(), "changed after capability planning") {
		t.Fatalf("PrepareOracle() = %v, want policy drift error", err)
	}
}

func TestPrepareOracleRejectsInvalidPlanBeforeDelegation(t *testing.T) {
	plan := capability.Plan{Schema: capability.Schema, Enforcement: "declaration-only", Assurance: "unverified"}
	_, err := PrepareOracle(plan, ".", []string{"/bin/cat"}, []string{"sorna"})
	if err == nil || !strings.Contains(err.Error(), "workspace id is required") {
		t.Fatalf("PrepareOracle() = %v, want plan validation error", err)
	}
}

func TestPrepareVerifierBindsOracleAndSeparatePolicies(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "workspace.yaml", "manifest")
	writeFile(t, "oracle-policy.yaml", "oracle policy")
	writeFile(t, "subject-policy.yaml", "subject policy")
	writeFile(t, "oracle.json", "frozen oracle")
	plan := capability.Plan{
		Schema: capability.Schema,
		Workspace: capability.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			Manifest: ciresult.FileRef{
				Path:   "workspace.yaml",
				SHA256: digest("manifest"),
			},
			OraclePolicy: ciresult.FileRef{
				Path:   "oracle-policy.yaml",
				SHA256: digest("oracle policy"),
			},
			SubjectPolicy: ciresult.FileRef{
				Path:   "subject-policy.yaml",
				SHA256: digest("subject policy"),
			},
		},
		ImplementationRoots: []string{"subject", "defects"},
		Enforcement:         "declaration-only",
		Assurance:           "unverified",
		Roles: []capability.Role{
			{
				ID:         "verifier",
				Kind:       "verifier",
				Workspace:  ".sentinel/verifier",
				ReadRoots:  []string{"contract.yaml", ".artifacts/oracle", "subject"},
				WriteRoots: []string{".artifacts/run"},
				DenyRoots:  []string{".git"},
			},
		},
	}

	prepared, err := PrepareVerifier(
		plan,
		".",
		"oracle.json",
		"http://127.0.0.1:8090",
		".",
		"./subject",
		"/healthz",
		"clean-baseline",
		".artifacts/run",
		[]string{"-addr", "127.0.0.1:8090"},
		[]string{"go", "run", "./sorna/cmd/sorna"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if prepared.RoleID != "verifier" || prepared.Backend != "sorna-cli" {
		t.Fatalf("prepared = %+v, want verifier Sorna delegation", prepared)
	}
	if prepared.Oracle.SHA256 != digest("frozen oracle") {
		t.Fatalf("oracle = %+v, want frozen oracle reference", prepared.Oracle)
	}
	if prepared.Policy.SHA256 != digest("oracle policy") || prepared.SubjectPolicy.SHA256 != digest("subject policy") {
		t.Fatalf("policy refs = %+v / %+v, want separate policy bindings", prepared.Policy, prepared.SubjectPolicy)
	}
	if prepared.AppliedPolicyPath == prepared.AppliedSubjectPolicyPath || prepared.AppliedOraclePath == prepared.AppliedPolicyPath {
		t.Fatalf("snapshot paths = %q, %q, %q; want distinct snapshots", prepared.AppliedOraclePath, prepared.AppliedPolicyPath, prepared.AppliedSubjectPolicyPath)
	}
	want := []string{
		"go", "run", "./sorna/cmd/sorna", "run",
		"--oracle", prepared.AppliedOraclePath,
		"--policy", prepared.AppliedPolicyPath,
		"--subject-policy", prepared.AppliedSubjectPolicyPath,
		"--subject-root", ".",
		"--base-url", "http://127.0.0.1:8090",
		"--subject-command", "./subject",
		"--subject-arg", "-addr",
		"--subject-arg", "127.0.0.1:8090",
		"--ready-path", "/healthz",
		"--subject-variant", "clean-baseline",
		"--output-dir", ".artifacts/run",
	}
	if strings.Join(prepared.Command, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("command = %q, want %q", prepared.Command, want)
	}
	for path, expected := range map[string]string{
		prepared.AppliedOraclePath:        "frozen oracle",
		prepared.AppliedPolicyPath:        "oracle policy",
		prepared.AppliedSubjectPolicyPath: "subject policy",
	} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != expected {
			t.Fatalf("snapshot %s = %q, %v; want %q", path, contents, err, expected)
		}
	}
}

func TestPrepareVerifierRejectsInvalidBaseURL(t *testing.T) {
	plan := capability.Plan{
		Schema: capability.Schema,
		Workspace: capability.WorkspaceRef{
			ID:            "webhook-validation",
			Version:       1,
			Manifest:      ciresult.FileRef{Path: "workspace.yaml", SHA256: digest("manifest")},
			OraclePolicy:  ciresult.FileRef{Path: "oracle-policy.yaml", SHA256: digest("oracle policy")},
			SubjectPolicy: ciresult.FileRef{Path: "subject-policy.yaml", SHA256: digest("subject policy")},
		},
		ImplementationRoots: []string{"subject"},
		Enforcement:         "declaration-only",
		Assurance:           "unverified",
		Roles: []capability.Role{{
			ID:        "verifier",
			Kind:      "verifier",
			Workspace: ".sentinel/verifier",
		}},
	}
	_, err := PrepareVerifier(plan, ".", "oracle.json", "not-a-url", ".", "subject", "/healthz", "clean", ".artifacts/run", nil, []string{"sorna"})
	if err == nil || !strings.Contains(err.Error(), "base URL must be an absolute") {
		t.Fatalf("PrepareVerifier() = %v, want base URL validation error", err)
	}
}

func TestPrepareVerifierRejectsSubjectRootEscape(t *testing.T) {
	plan := capability.Plan{
		Schema: capability.Schema,
		Workspace: capability.WorkspaceRef{
			ID:            "webhook-validation",
			Version:       1,
			Manifest:      ciresult.FileRef{Path: "workspace.yaml", SHA256: digest("manifest")},
			OraclePolicy:  ciresult.FileRef{Path: "oracle-policy.yaml", SHA256: digest("oracle policy")},
			SubjectPolicy: ciresult.FileRef{Path: "subject-policy.yaml", SHA256: digest("subject policy")},
		},
		ImplementationRoots: []string{"subject"},
		Enforcement:         "declaration-only",
		Assurance:           "unverified",
		Roles: []capability.Role{{
			ID:        "verifier",
			Kind:      "verifier",
			Workspace: ".sentinel/verifier",
		}},
	}
	_, err := PrepareVerifier(plan, ".", "oracle.json", "http://127.0.0.1:8090", "../outside", "subject", "/healthz", "clean", ".artifacts/run", nil, []string{"sorna"})
	if err == nil || !strings.Contains(err.Error(), "must stay inside the project root") {
		t.Fatalf("PrepareVerifier() = %v, want subject-root containment error", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func digest(contents string) string {
	digest := sha256.Sum256([]byte(contents))
	return hex.EncodeToString(digest[:])
}
