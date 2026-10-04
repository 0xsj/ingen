package main

import (
	"os"
	"path/filepath"
	"testing"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/evidence"
)

func TestRoleEvidenceExitCodeUsesProducerLifecycleMapping(t *testing.T) {
	for _, test := range []struct {
		status string
		exit   *int
		want   int
	}{
		{status: "completed", want: 0},
		{status: "failed", exit: intPtr(7), want: 1},
		{status: "canceled", want: 2},
		{status: "indeterminate", want: 2},
		{status: "unknown", want: 2},
	} {
		if got := roleEvidenceExitCode(test.status, test.exit); got != test.want {
			t.Errorf("roleEvidenceExitCode(%q, %v) = %d, want %d", test.status, test.exit, got, test.want)
		}
	}
}

func TestRoleExecuteErrorNeverReportsSuccessForCanceledOrIndeterminateZero(t *testing.T) {
	zero, failed, negative := 0, 7, -1
	for _, test := range []struct {
		name string
		code *int
		want int
	}{
		{name: "canceled with zero", code: &zero, want: 1},
		{name: "indeterminate with zero", code: &zero, want: 1},
		{name: "failed with positive exit", code: &failed, want: 7},
		{name: "missing exit", want: 1},
		{name: "unknown exit", code: &negative, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := roleExecuteErrorExitCode(test.code); got != test.want {
				t.Fatalf("roleExecuteErrorExitCode(%v) = %d, want %d", test.code, got, test.want)
			}
		})
	}
}

func TestRejectRoleCIResultOverlapIncludesAbsentCapturePath(t *testing.T) {
	verified := evidence.Verified{Report: evidence.Report{
		WorkspaceManifestPath: ".ingen/workspace.yaml",
		PolicyPath:            ".ingen/artifacts/role-executions/x.policy.json",
		StdoutPath:            ".ingen/artifacts/role-executions/x.stdout",
		StderrPath:            ".ingen/artifacts/role-executions/x.stderr",
	}}
	if err := rejectRoleCIResultOverlap(".ingen/artifacts/role-executions/x.stdout", ".ingen/artifacts/role-executions/x.json", verified); err == nil {
		t.Fatal("CI output overlapping an absent capture path was accepted")
	}
	if err := rejectRoleCIResultOverlap(".ingen/artifacts/role-executions", ".ingen/artifacts/role-executions/x.json", verified); err == nil {
		t.Fatal("CI output directory overlapping report inputs was accepted")
	}
}

func TestWriteRoleCIResultPublishesWithoutReplacement(t *testing.T) {
	root := t.TempDir()
	artifact := ciresult.Artifact{Schema: ciresult.Schema, Tool: "test", Kind: "role-execution"}
	if err := writeRoleCIResult(root, "results/role.json", artifact); err != nil {
		t.Fatalf("writeRoleCIResult() error = %v", err)
	}
	path := filepath.Join(root, "results", "role.json")
	first, err := os.ReadFile(path)
	if err != nil || len(first) == 0 {
		t.Fatalf("published CI result = %q, %v", first, err)
	}
	if err := writeRoleCIResult(root, "results/role.json", artifact); err == nil {
		t.Fatal("writeRoleCIResult replaced an existing result")
	}
	second, err := os.ReadFile(path)
	if err != nil || string(second) != string(first) {
		t.Fatalf("existing CI result changed after collision: %v", err)
	}
}

func intPtr(value int) *int { return &value }
