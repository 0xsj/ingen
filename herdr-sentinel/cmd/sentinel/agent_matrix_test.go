package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
)

func TestAgentCompareCommandEmitsJSONAndMapsAggregateExit(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   int
	}{{"supported", 0}, {"unsupported", 1}, {"indeterminate", 2}} {
		t.Run(tc.status, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			calls := 0
			code := agentCompareCommandWith([]string{
				"--agent-executable", "/opt/codex-a",
				"--agent-executable", "/opt/codex-b",
				"--timeout-seconds", "17",
			}, &stdout, &stderr, func(_ context.Context, request agentprobe.MatrixRequest) (agentprobe.MatrixReport, error) {
				calls++
				if len(request.ExecutablePaths) != 2 || request.Timeout != 17*time.Second {
					t.Fatalf("Compare request = %+v", request)
				}
				return agentprobe.MatrixReport{Schema: agentprobe.MatrixSchema, Status: tc.status}, nil
			})
			if code != tc.want || calls != 1 || stdout.Len() == 0 || stderr.Len() != 0 {
				t.Fatalf("exit/calls/stdout/stderr = %d/%d/%q/%q", code, calls, stdout.String(), stderr.String())
			}
			if !strings.HasPrefix(stdout.String(), "{\n") || strings.Contains(stdout.String(), "codex child stdout") {
				t.Fatalf("command output is not safe JSON: %q", stdout.String())
			}
		})
	}
}

func TestAgentCompareCommandRejectsInvalidInvocationBeforeProbe(t *testing.T) {
	cases := [][]string{
		{},
		{"--agent-executable", "/one", "--timeout-seconds", "0"},
		{"--agent-executable", "/one", "--unknown"},
		{"--agent-executable", "/one", "--agent-executable", "/two", "--agent-executable", "/three", "--agent-executable", "/four", "--agent-executable", "/five"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		calls := 0
		code := agentCompareCommandWith(args, &stdout, &stderr, func(context.Context, agentprobe.MatrixRequest) (agentprobe.MatrixReport, error) {
			calls++
			return agentprobe.MatrixReport{}, nil
		})
		if code != 2 || calls != 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%q exit/calls/stdout/stderr = %d/%d/%q/%q", args, code, calls, stdout.String(), stderr.String())
		}
	}
}
