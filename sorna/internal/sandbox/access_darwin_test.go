//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseAccessOutputScopesStructuredEventsToProcess(t *testing.T) {
	output := []byte(fmt.Sprintf(`{"timestamp":"2026-09-14 08:56:34.966769+0300","eventMessage":"Sandbox: sorna(%d) deny(1) file-read-data /project/implementation.go"}
{"timestamp":"2026-09-14 08:56:34.966770+0300","eventMessage":"Sandbox: other(%d) deny(1) file-read-data /other/secret.go"}
{"count":2}
`, 1234, 5678))
	events, parseErrors := parseAccessOutput(output, 1234)
	if parseErrors != 0 || len(events) != 1 {
		t.Fatalf("events = %+v, parse errors = %d; want one scoped event", events, parseErrors)
	}
	if events[0].PID != 1234 || events[0].Decision != "deny" {
		t.Fatalf("event = %+v, want PID 1234 deny decision", events[0])
	}
}

func TestParseAccessOutputScopesStructuredEventsToProcessTree(t *testing.T) {
	output := []byte(fmt.Sprintf(`{"timestamp":"2026-09-14 08:56:34.966769+0300","eventMessage":"Sandbox: parent(%d) deny(1) file-read-data /project/contract.json"}
{"timestamp":"2026-09-14 08:56:34.966770+0300","eventMessage":"Sandbox: child(%d) deny(1) file-read-data /project/implementation.go"}
{"timestamp":"2026-09-14 08:56:34.966771+0300","eventMessage":"Sandbox: unrelated(%d) deny(1) file-read-data /other/secret.go"}
`, 1234, 1235, 5678))
	events, parseErrors := parseAccessOutputForProcesses(output, map[int]struct{}{1234: {}, 1235: {}})
	if parseErrors != 0 || len(events) != 2 {
		t.Fatalf("events = %+v, parse errors = %d; want two process-tree events", events, parseErrors)
	}
	if events[0].PID != 1234 || events[1].PID != 1235 {
		t.Fatalf("events = %+v, want parent and child PIDs", events)
	}
}

func TestAccessCaptureSamplesDescendantProcess(t *testing.T) {
	capture := &darwinAccessCapture{
		ctx:        context.Background(),
		started:    time.Now(),
		processIDs: make(map[int]struct{}),
	}
	command := exec.Command("/bin/sh", "-c", "sleep 0.25; exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
		}
	}()
	rootPID := command.Process.Pid
	if err := capture.Attach(rootPID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(75 * time.Millisecond)
	capture.extendProcessTree(rootPID)
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	capture.stopSampler()
	processIDs := capture.snapshotProcessIDs()
	if len(processIDs) < 2 {
		t.Fatalf("observed process IDs = %v, want root and a descendant", processIDs)
	}
	if processIDs[0] != rootPID {
		t.Fatalf("observed process IDs = %v, want root PID %d included first", processIDs, rootPID)
	}
	if observations := capture.snapshotExecutableHistory(); len(observations) == 0 {
		t.Fatal("executable observations are empty; want a root process identity")
	}
}

func TestAccessCaptureRecordsExecutableTransition(t *testing.T) {
	capture := &darwinAccessCapture{
		ctx:        context.Background(),
		started:    time.Now(),
		processIDs: make(map[int]struct{}),
	}
	command := exec.Command("/bin/sh", "-c", "sleep 0.15; exec /bin/sleep 0.25")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	rootPID := command.Process.Pid
	if err := capture.Attach(rootPID); err != nil {
		t.Fatal(err)
	}
	capture.extendProcessTree(rootPID)
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	capture.stopSampler()

	observations := capture.snapshotExecutableHistory()
	identitiesByPID := make(map[int]map[string]bool)
	for _, observation := range observations {
		if identitiesByPID[observation.PID] == nil {
			identitiesByPID[observation.PID] = make(map[string]bool)
		}
		if strings.HasSuffix(observation.Path, "/sh") {
			identitiesByPID[observation.PID]["shell"] = true
		}
		if strings.HasSuffix(observation.Path, "/sleep") {
			identitiesByPID[observation.PID]["sleep"] = true
		}
	}
	if !identitiesByPID[rootPID]["shell"] || !identitiesByPID[rootPID]["sleep"] {
		t.Fatalf("executable observations = %+v; want shell and later sleep identities", observations)
	}
}

func TestAccessCaptureCanMissShortLivedTransitionBetweenSamples(t *testing.T) {
	capture := &darwinAccessCapture{
		ctx:              context.Background(),
		started:          time.Now(),
		processIDs:       make(map[int]struct{}),
		samplingInterval: 500 * time.Millisecond,
	}
	command := exec.Command("/bin/sh", "-c", "sleep 0.05; exec /bin/sleep 0.05")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	rootPID := command.Process.Pid
	if err := capture.Attach(rootPID); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	capture.stopSampler()

	rootObservations := make([]ExecutableObservation, 0)
	for _, observation := range capture.snapshotExecutableHistory() {
		if observation.PID == rootPID {
			rootObservations = append(rootObservations, observation)
		}
	}
	if len(rootObservations) != 1 || !strings.HasSuffix(rootObservations[0].Path, "/sh") {
		t.Fatalf("root executable observations = %+v; want only the initial shell identity", rootObservations)
	}
	if capture.executableSamples < 2 {
		t.Fatalf("executable sample count = %d; want initial synchronous and sampler attempts", capture.executableSamples)
	}
}

func TestParseAccessEventNormalizesSeatbeltMessage(t *testing.T) {
	event, err := parseAccessEvent(macOSLogEvent{
		Timestamp:    "2026-09-14 08:56:34.966769+0300",
		EventMessage: "Sandbox: sorna(1234) deny(1) file-read-data /project/implementation.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Timestamp.Equal(time.Time{}) || event.Process != "sorna" || event.PID != 1234 || event.Decision != "deny" || event.Operation != "file-read-data" || event.Resource != "/project/implementation.go" {
		t.Fatalf("event = %+v, want normalized Seatbelt fields", event)
	}
}

func TestParseAccessEventRejectsDuplicateReportEnvelope(t *testing.T) {
	_, err := parseAccessEvent(macOSLogEvent{
		Timestamp:    "2026-09-14T08:56:34.966769+03:00",
		EventMessage: "1 duplicate report for Sandbox: sorna(1234) deny(1) file-read-data /project/implementation.go",
	})
	if err == nil || !strings.Contains(err.Error(), "unrecognized sandbox event") {
		t.Fatalf("parseAccessEvent() = %v, want duplicate envelope rejection", err)
	}
}
