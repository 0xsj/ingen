//go:build darwin

package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var accessMessagePattern = regexp.MustCompile(`^Sandbox: ([^\s(]+)\(([0-9]+)\) (allow|deny)\([0-9]+\) ([^\s]+)(?: (.*))?$`)

var accessProcessIDPattern = regexp.MustCompile(`^Sandbox: [^\s(]+\(([0-9]+)\)`)

var accessProcessIDSearchPattern = regexp.MustCompile(`Sandbox: [^\s(]+\(([0-9]+)\)`)

const executableSamplingInterval = 25 * time.Millisecond

type darwinAccessCapture struct {
	ctx               context.Context
	started           time.Time
	mu                sync.Mutex
	processIDs        map[int]struct{}
	lastExecutables   map[int]ExecutableIdentity
	executableHistory []ExecutableObservation
	executableErrors  int
	executableSamples int
	samplingInterval  time.Duration
	samplingStartedAt time.Time
	samplingStoppedAt time.Time
	samplerStop       chan struct{}
	samplerDone       chan struct{}
	samplerStarted    bool
	processTreeErrors int
	stopOnce          sync.Once
	result            AccessReport
	err               error
}

type macOSLogEvent struct {
	Timestamp    string `json:"timestamp"`
	EventMessage string `json:"eventMessage"`
}

func startAccessCapture(ctx context.Context) (AccessCapture, error) {
	if _, err := exec.LookPath("/usr/bin/log"); err != nil {
		return nil, fmt.Errorf("resolve macOS log collector: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &darwinAccessCapture{
		ctx:              ctx,
		started:          time.Now(),
		processIDs:       make(map[int]struct{}),
		lastExecutables:  make(map[int]ExecutableIdentity),
		samplingInterval: executableSamplingInterval,
	}, nil
}

func (capture *darwinAccessCapture) Attach(processID int) error {
	if processID <= 0 {
		return fmt.Errorf("access capture process ID must be positive")
	}
	var initialSampleDone chan struct{}
	capture.mu.Lock()
	capture.processIDs[processID] = struct{}{}
	if capture.samplingInterval <= 0 {
		capture.samplingInterval = executableSamplingInterval
	}
	if capture.samplingStartedAt.IsZero() {
		capture.samplingStartedAt = time.Now().UTC()
	}
	if !capture.samplerStarted {
		capture.samplerStarted = true
		capture.samplerStop = make(chan struct{})
		capture.samplerDone = make(chan struct{})
		initialSampleDone = make(chan struct{})
	}
	capture.mu.Unlock()
	if initialSampleDone != nil {
		go capture.sampleProcessTree(processID, initialSampleDone)
	}
	// Take one synchronous sample before returning so a short-lived process
	// cannot finish before the asynchronous sampler gets its first turn.
	capture.extendProcessTree(processID)
	if initialSampleDone != nil {
		close(initialSampleDone)
	}
	return nil
}

func (capture *darwinAccessCapture) Stop(processID int) (AccessReport, error) {
	capture.stopOnce.Do(func() {
		if processID > 0 {
			capture.mu.Lock()
			capture.processIDs[processID] = struct{}{}
			capture.mu.Unlock()
		}
		capture.stopSampler()
		capture.mu.Lock()
		if capture.executableSamples > 0 && capture.samplingStoppedAt.IsZero() {
			capture.samplingStoppedAt = time.Now().UTC()
		}
		capture.mu.Unlock()
		processIDs := capture.snapshotProcessIDs()
		processTreeErrors := capture.snapshotProcessTreeErrors()
		executableSamples, samplingInterval, samplingStartedAt, samplingStoppedAt := capture.snapshotExecutableSampling()
		executableHistory := capture.snapshotExecutableHistory()
		executableErrors := capture.snapshotExecutableErrors()
		if len(processIDs) == 0 {
			capture.result = AccessReport{
				Source:                      "macos-unified-log",
				ProcessTreeErrors:           processTreeErrors,
				ExecutableSampleCount:       executableSamples,
				ExecutableSamplingInterval:  samplingInterval,
				ExecutableSamplingStartedAt: samplingStartedAt,
				ExecutableSamplingStoppedAt: samplingStoppedAt,
				ExecutableObservations:      executableHistory,
				ExecutableObservationErrors: executableErrors,
			}
			return
		}
		ended := time.Now()
		start := capture.started.Truncate(time.Second).Add(-time.Second)
		end := ended.Truncate(time.Second).Add(time.Second)
		predicate := `eventMessage CONTAINS[c] "Sandbox:"`
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
		processSet := make(map[int]struct{}, len(processIDs))
		for _, processID := range processIDs {
			processSet[processID] = struct{}{}
		}
		events, parseErrors := parseAccessOutputForProcesses(output, processSet)
		capture.result = AccessReport{
			Source:                      "macos-unified-log",
			Events:                      events,
			ProcessIDs:                  processIDs,
			ParseErrors:                 parseErrors,
			ProcessTreeErrors:           processTreeErrors,
			ExecutableSampleCount:       executableSamples,
			ExecutableSamplingInterval:  samplingInterval,
			ExecutableSamplingStartedAt: samplingStartedAt,
			ExecutableSamplingStoppedAt: samplingStoppedAt,
			ExecutableObservations:      executableHistory,
			ExecutableObservationErrors: executableErrors,
		}
	})
	return capture.result, capture.err
}

func (capture *darwinAccessCapture) stopSampler() {
	capture.mu.Lock()
	if !capture.samplerStarted {
		capture.mu.Unlock()
		return
	}
	select {
	case <-capture.samplerStop:
	default:
		close(capture.samplerStop)
	}
	done := capture.samplerDone
	capture.mu.Unlock()
	<-done
}

func (capture *darwinAccessCapture) snapshotProcessIDs() []int {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	processIDs := make([]int, 0, len(capture.processIDs))
	for processID := range capture.processIDs {
		processIDs = append(processIDs, processID)
	}
	sort.Ints(processIDs)
	return processIDs
}

func (capture *darwinAccessCapture) snapshotProcessTreeErrors() int {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.processTreeErrors
}

func (capture *darwinAccessCapture) snapshotExecutableHistory() []ExecutableObservation {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]ExecutableObservation(nil), capture.executableHistory...)
}

func (capture *darwinAccessCapture) snapshotExecutableErrors() int {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.executableErrors
}

func (capture *darwinAccessCapture) snapshotExecutableSampling() (int, time.Duration, time.Time, time.Time) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.executableSamples, capture.samplingInterval, capture.samplingStartedAt, capture.samplingStoppedAt
}

func (capture *darwinAccessCapture) sampleProcessTree(rootProcessID int, initialSampleDone <-chan struct{}) {
	defer close(capture.samplerDone)
	capture.mu.Lock()
	interval := capture.samplingInterval
	if interval <= 0 {
		interval = executableSamplingInterval
	}
	capture.mu.Unlock()
	select {
	case <-initialSampleDone:
	case <-capture.samplerStop:
		return
	case <-capture.ctx.Done():
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		capture.extendProcessTree(rootProcessID)
		select {
		case <-capture.samplerStop:
			return
		case <-capture.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (capture *darwinAccessCapture) extendProcessTree(rootProcessID int) {
	capture.mu.Lock()
	if capture.samplingStartedAt.IsZero() {
		capture.samplingStartedAt = time.Now().UTC()
	}
	capture.executableSamples++
	capture.mu.Unlock()
	entries, err := processTable()
	if err != nil {
		capture.mu.Lock()
		capture.processTreeErrors++
		capture.mu.Unlock()
		return
	}
	children := make(map[int][]int)
	for _, entry := range entries {
		children[entry.PPID] = append(children[entry.PPID], entry.PID)
	}
	for _, childIDs := range children {
		sort.Ints(childIDs)
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()
	queue := []int{rootProcessID}
	seen := make(map[int]struct{}, len(queue))
	now := time.Now().UTC()
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]
		if _, ok := seen[parentID]; ok {
			continue
		}
		seen[parentID] = struct{}{}
		if entry, ok := entries[parentID]; ok {
			capture.observeExecutableLocked(now, entry.PID, entry.Path)
		}
		for _, childID := range children[parentID] {
			capture.processIDs[childID] = struct{}{}
			queue = append(queue, childID)
		}
	}
}

func (capture *darwinAccessCapture) observeExecutableLocked(timestamp time.Time, processID int, path string) {
	if strings.HasPrefix(path, "<") {
		return
	}
	if capture.lastExecutables == nil {
		capture.lastExecutables = make(map[int]ExecutableIdentity)
	}
	path = canonicalizeExistingParent(path)
	identity, err := identityForPath(processID, path)
	if err != nil {
		capture.executableErrors++
		return
	}
	if previous, ok := capture.lastExecutables[processID]; ok && previous == identity {
		return
	}
	capture.lastExecutables[processID] = identity
	capture.executableHistory = append(capture.executableHistory, ExecutableObservation{
		Timestamp: timestamp,
		PID:       processID,
		Path:      identity.Path,
		SHA256:    identity.SHA256,
	})
}

func formatLogTime(value time.Time) string {
	return value.Format("2006-01-02 15:04:05-0700")
}

func parseAccessOutput(output []byte, processID int) ([]AccessEvent, int) {
	return parseAccessOutputForProcesses(output, map[int]struct{}{processID: {}})
}

func parseAccessOutputForProcesses(output []byte, processIDs map[int]struct{}) ([]AccessEvent, int) {
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
	return normalizeAccessEvents(messages, unparsedLines, processIDs)
}

func normalizeAccessEvents(messages []macOSLogEvent, unparsedLines []string, processIDs map[int]struct{}) ([]AccessEvent, int) {
	events := make([]AccessEvent, 0)
	parseErrors := 0
	for _, message := range messages {
		matches := accessProcessIDPattern.FindStringSubmatch(message.EventMessage)
		if len(matches) != 2 {
			continue
		}
		processID, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		if _, ok := processIDs[processID]; !ok {
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
		if len(matches) != 2 {
			continue
		}
		processID, err := strconv.Atoi(matches[1])
		if err == nil {
			if _, ok := processIDs[processID]; ok {
				parseErrors++
			}
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
