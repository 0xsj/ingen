package agentprobe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareValidatesAllCandidatesBeforeRunningAny(t *testing.T) {
	first := matrixExecutable(t, "candidate-one")
	missing := filepath.Join(t.TempDir(), "missing-codex")
	called := 0
	_, err := compare(context.Background(), MatrixRequest{
		ExecutablePaths: []string{first, missing}, Timeout: time.Second,
	}, func(context.Context, Request) (Report, error) {
		called++
		return Report{}, nil
	}, time.Now)
	if err == nil {
		t.Fatal("Compare accepted a missing candidate")
	}
	if called != 0 {
		t.Fatalf("diagnostic calls = %d, want no candidates started before complete preflight", called)
	}
}

func TestCompareRejectsFractionalTimeout(t *testing.T) {
	first := matrixExecutable(t, "candidate-one")
	called := false
	_, err := compare(context.Background(), MatrixRequest{
		ExecutablePaths: []string{first}, Timeout: 1500 * time.Millisecond,
	}, func(context.Context, Request) (Report, error) {
		called = true
		return Report{}, nil
	}, time.Now)
	if err == nil || called {
		t.Fatalf("fractional timeout err=%v, diagnose called=%v", err, called)
	}
}

func TestCompareRejectsCanonicalPathAliasesBeforeRunning(t *testing.T) {
	first := matrixExecutable(t, "candidate-one")
	alias := filepath.Join(t.TempDir(), "candidate-alias")
	if err := os.Symlink(first, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	called := 0
	_, err := compare(context.Background(), MatrixRequest{
		ExecutablePaths: []string{first, alias}, Timeout: time.Second,
	}, func(context.Context, Request) (Report, error) {
		called++
		return Report{}, nil
	}, time.Now)
	if err == nil || !strings.Contains(err.Error(), "distinct canonical") {
		t.Fatalf("Compare error = %v, want duplicate canonical path rejection", err)
	}
	if called != 0 {
		t.Fatalf("diagnostic calls = %d, want no calls", called)
	}
}

func TestComparePreservesPerCandidateUncertaintyAndAggregateRules(t *testing.T) {
	one, two := matrixExecutable(t, "one"), matrixExecutable(t, "two")
	paths := []string{one, two}
	cases := []struct {
		name     string
		statuses []string
		want     string
	}{
		{name: "supported with uncertain peer", statuses: []string{"supported", "indeterminate"}, want: "supported"},
		{name: "all unsupported", statuses: []string{"unsupported", "unsupported"}, want: "unsupported"},
		{name: "unsupported with uncertainty", statuses: []string{"unsupported", "indeterminate"}, want: "indeterminate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := 0
			report, err := compare(context.Background(), MatrixRequest{ExecutablePaths: paths, Timeout: 3 * time.Second}, func(_ context.Context, request Request) (Report, error) {
				index++
				if request.Timeout != 3*time.Second {
					t.Fatalf("per-candidate timeout = %s", request.Timeout)
				}
				sha, err := hashMatrixExecutable(request.ExecutablePath)
				if err != nil {
					t.Fatal(err)
				}
				return Report{Status: tc.statuses[index-1], ExecutablePath: request.ExecutablePath, ExecutableSHA256: sha}, nil
			}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != tc.want || len(report.Candidates) != 2 {
				t.Fatalf("matrix status/candidates = %s/%d, want %s/2", report.Status, len(report.Candidates), tc.want)
			}
			for i, candidate := range report.Candidates {
				if candidate.Status != tc.statuses[i] || candidate.ExecutableSHA256Before == "" || candidate.ExecutableSHA256Before != candidate.ExecutableSHA256After {
					t.Fatalf("candidate %d lost status or digest binding: %+v", i, candidate)
				}
			}
			if tc.statuses[1] == "indeterminate" && !containsString(report.Limitations, "one or more selected candidates remain indeterminate; inspect every candidate result even when another candidate is supported") {
				t.Fatal("matrix did not preserve uncertainty in its limitations")
			}
		})
	}
}

func TestCompareMarksExecutableDriftIndeterminate(t *testing.T) {
	first, second := matrixExecutable(t, "one"), matrixExecutable(t, "two")
	paths := []string{first, second}
	index := 0
	report, err := compare(context.Background(), MatrixRequest{ExecutablePaths: paths, Timeout: time.Second}, func(_ context.Context, request Request) (Report, error) {
		index++
		sha, err := hashMatrixExecutable(request.ExecutablePath)
		if err != nil {
			t.Fatal(err)
		}
		if index == 1 {
			if err := os.WriteFile(request.ExecutablePath, []byte("changed candidate"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		status := "supported"
		if index == 2 {
			status = "unsupported"
		}
		return Report{Status: status, ExecutablePath: request.ExecutablePath, ExecutableSHA256: sha}, nil
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates[0].Status != "indeterminate" || report.Candidates[0].ReasonCode != "executable-changed-during-diagnostic" || report.Candidates[0].ExecutableSHA256Before == report.Candidates[0].ExecutableSHA256After {
		t.Fatalf("changed candidate was not bound as indeterminate: %+v", report.Candidates[0])
	}
	if report.Status != "indeterminate" {
		t.Fatalf("matrix status = %s, want indeterminate when no exact candidate passed", report.Status)
	}
}

func TestCompareDoesNotPublishDiagnosticErrorsOrRawOutput(t *testing.T) {
	first := matrixExecutable(t, "one")
	report, err := compare(context.Background(), MatrixRequest{ExecutablePaths: []string{first}, Timeout: time.Second}, func(context.Context, Request) (Report, error) {
		return Report{}, os.ErrPermission
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "indeterminate" || report.Candidates[0].Status != "indeterminate" || report.Candidates[0].ReasonCode != "diagnostic-failed" || report.Candidates[0].Diagnostic != nil {
		t.Fatalf("diagnostic error was not sanitized: %+v", report)
	}
}

func TestCompareRejectsNonExecutableAndOversizedCandidates(t *testing.T) {
	nonExecutable := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(nonExecutable, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := compare(context.Background(), MatrixRequest{ExecutablePaths: []string{nonExecutable}, Timeout: time.Second}, nil, time.Now); err == nil {
		t.Fatal("Compare accepted a non-executable file")
	}
	oversized := filepath.Join(t.TempDir(), "oversized")
	file, err := os.OpenFile(oversized, os.O_CREATE|os.O_RDWR, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxMatrixExecutableBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := compare(context.Background(), MatrixRequest{ExecutablePaths: []string{oversized}, Timeout: time.Second}, nil, time.Now); err == nil {
		t.Fatal("Compare accepted an oversized candidate")
	}
}

func matrixExecutable(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
