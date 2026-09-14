//go:build darwin

package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var accessMessagePattern = regexp.MustCompile(`^Sandbox: ([^\s(]+)\(([0-9]+)\) (allow|deny)\([0-9]+\) ([^\s]+)(?: (.*))?$`)

type darwinAccessCapture struct {
	cancel   context.CancelFunc
	command  *exec.Cmd
	readDone chan accessReadResult
	stopOnce sync.Once
	result   AccessReport
	err      error
}

type accessReadResult struct {
	events      []AccessEvent
	parseErrors int
	err         error
}

type macOSLogEvent struct {
	Timestamp    string `json:"timestamp"`
	EventMessage string `json:"eventMessage"`
}

func startAccessCapture(ctx context.Context) (AccessCapture, error) {
	if _, err := exec.LookPath("/usr/bin/log"); err != nil {
		return nil, fmt.Errorf("resolve macOS log collector: %w", err)
	}
	captureContext, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(captureContext, "/usr/bin/log", "stream",
		"--style", "ndjson",
		"--level", "debug",
		"--color", "none",
		"--ignore-dropped",
		"--predicate", `eventMessage CONTAINS[c] "Sandbox:"`,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("prepare macOS log collector: %w", err)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start macOS log collector: %w", err)
	}
	capture := &darwinAccessCapture{
		cancel:   cancel,
		command:  command,
		readDone: make(chan accessReadResult, 1),
	}
	go capture.read(stdout)
	return capture, nil
}

func (capture *darwinAccessCapture) read(stdout io.ReadCloser) {
	defer stdout.Close()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	result := accessReadResult{events: make([]AccessEvent, 0)}
	for scanner.Scan() {
		var logEvent macOSLogEvent
		if err := json.Unmarshal(scanner.Bytes(), &logEvent); err != nil {
			result.parseErrors++
			continue
		}
		if !strings.HasPrefix(logEvent.EventMessage, "Sandbox: ") {
			continue
		}
		event, err := parseAccessEvent(logEvent)
		if err != nil {
			result.parseErrors++
			continue
		}
		result.events = append(result.events, event)
	}
	if err := scanner.Err(); err != nil {
		result.err = err
	}
	capture.readDone <- result
}

func (capture *darwinAccessCapture) Stop(processID int) (AccessReport, error) {
	capture.stopOnce.Do(func() {
		capture.cancel()
		waitErr := capture.command.Wait()
		readResult := <-capture.readDone
		capture.result = AccessReport{
			Source:      "macos-unified-log",
			Events:      filterAccessEvents(readResult.events, processID),
			ParseErrors: readResult.parseErrors,
		}
		capture.err = readResult.err
		if capture.err == nil && waitErr != nil && !errors.Is(waitErr, context.Canceled) {
			// CommandContext reports a signal after cancellation. It is the
			// expected shutdown path, not a telemetry failure.
			if !strings.Contains(waitErr.Error(), "signal: killed") {
				capture.err = fmt.Errorf("macOS log collector: %w", waitErr)
			}
		}
	})
	return capture.result, capture.err
}

func filterAccessEvents(events []AccessEvent, processID int) []AccessEvent {
	filtered := make([]AccessEvent, 0)
	for _, event := range events {
		if event.PID == processID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func parseAccessEvent(logEvent macOSLogEvent) (AccessEvent, error) {
	matches := accessMessagePattern.FindStringSubmatch(logEvent.EventMessage)
	if len(matches) == 0 {
		return AccessEvent{}, fmt.Errorf("unrecognized sandbox event: %q", logEvent.EventMessage)
	}
	pid, err := strconv.Atoi(matches[2])
	if err != nil {
		return AccessEvent{}, fmt.Errorf("parse sandbox process ID: %w", err)
	}
	timestamp, err := time.Parse(time.RFC3339Nano, logEvent.Timestamp)
	if err != nil {
		return AccessEvent{}, fmt.Errorf("parse sandbox event timestamp: %w", err)
	}
	return AccessEvent{
		Timestamp: timestamp.UTC(),
		Process:   matches[1],
		PID:       pid,
		Decision:  matches[3],
		Operation: matches[4],
		Resource:  matches[5],
	}, nil
}
