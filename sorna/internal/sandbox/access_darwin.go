//go:build darwin

package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var accessMessagePattern = regexp.MustCompile(`^Sandbox: ([^\s(]+)\(([0-9]+)\) (allow|deny)\([0-9]+\) ([^\s]+)(?: (.*))?$`)

var accessProcessIDPattern = regexp.MustCompile(`^Sandbox: [^\s(]+\(([0-9]+)\)`)

var accessProcessIDSearchPattern = regexp.MustCompile(`Sandbox: [^\s(]+\(([0-9]+)\)`)

type darwinAccessCapture struct {
	ctx      context.Context
	started  time.Time
	stopOnce sync.Once
	result   AccessReport
	err      error
}

type macOSLogEvent struct {
	Timestamp    string `json:"timestamp"`
	EventMessage string `json:"eventMessage"`
}

func startAccessCapture(ctx context.Context) (AccessCapture, error) {
	if _, err := exec.LookPath("/usr/bin/log"); err != nil {
		return nil, fmt.Errorf("resolve macOS log collector: %w", err)
	}
	return &darwinAccessCapture{ctx: ctx, started: time.Now()}, nil
}

func (capture *darwinAccessCapture) Stop(processID int) (AccessReport, error) {
	capture.stopOnce.Do(func() {
		ended := time.Now()
		start := capture.started.Truncate(time.Second).Add(-time.Second)
		end := ended.Truncate(time.Second).Add(time.Second)
		predicate := fmt.Sprintf(`eventMessage CONTAINS[c] "Sandbox:" AND eventMessage CONTAINS[c] "(%d)"`, processID)
		command := exec.CommandContext(capture.ctx, "/usr/bin/log", "show",
			"--style", "ndjson",
			"--debug",
			"--info",
			"--color", "none",
			"--start", formatLogTime(start),
			"--end", formatLogTime(end),
			"--predicate", predicate,
		)
		output, err := command.Output()
		if err != nil {
			capture.err = fmt.Errorf("query macOS sandbox log: %w", err)
			return
		}
		events, parseErrors := parseAccessOutput(output, processID)
		capture.result = AccessReport{
			Source:      "macos-unified-log",
			Events:      events,
			ParseErrors: parseErrors,
		}
	})
	return capture.result, capture.err
}

func formatLogTime(value time.Time) string {
	return value.Format("2006-01-02 15:04:05-0700")
}

func parseAccessOutput(output []byte, processID int) ([]AccessEvent, int) {
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	messages := make([]macOSLogEvent, 0)
	unparsedLines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Bytes()
		var logEvent macOSLogEvent
		if err := json.Unmarshal(line, &logEvent); err != nil {
			unparsedLines = append(unparsedLines, string(line))
			continue
		}
		if strings.HasPrefix(logEvent.EventMessage, "Sandbox: ") {
			messages = append(messages, logEvent)
		}
	}
	if scanner.Err() != nil {
		unparsedLines = append(unparsedLines, scanner.Err().Error())
	}
	return normalizeAccessEvents(messages, unparsedLines, processID)
}

func normalizeAccessEvents(messages []macOSLogEvent, unparsedLines []string, processID int) ([]AccessEvent, int) {
	events := make([]AccessEvent, 0)
	parseErrors := 0
	for _, message := range messages {
		matches := accessProcessIDPattern.FindStringSubmatch(message.EventMessage)
		if len(matches) != 2 || matches[1] != strconv.Itoa(processID) {
			continue
		}
		event, err := parseAccessEvent(message)
		if err != nil {
			parseErrors++
			continue
		}
		events = append(events, event)
	}
	for _, line := range unparsedLines {
		matches := accessProcessIDSearchPattern.FindStringSubmatch(line)
		if len(matches) == 2 && matches[1] == strconv.Itoa(processID) {
			parseErrors++
		}
	}
	return events, parseErrors
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
	timestamp, err := parseAccessTimestamp(logEvent.Timestamp)
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

func parseAccessTimestamp(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-0700"} {
		if timestamp, err := time.Parse(layout, raw); err == nil {
			return timestamp, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format %q", raw)
}
