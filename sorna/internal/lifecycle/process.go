// Package lifecycle owns the process boundary for a Sorna subject run.
package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Config describes a subject process that Sorna may start and stop.
type Config struct {
	Command         []string
	RecordCommand   []string
	Dir             string
	Env             []string
	BaseURL         string
	ReadyPath       string
	Client          *http.Client
	StartupTimeout  time.Duration
	ShutdownTimeout time.Duration
	ProbeTimeout    time.Duration
	PollInterval    time.Duration
	Sandbox         *SandboxRecord
	Access          *AccessTelemetry
	Now             func() time.Time
}

// Event is an ordered lifecycle observation. It describes process control,
// not capability enforcement.
type Event struct {
	Sequence  int       `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Detail    string    `json:"detail,omitempty"`
}

// SandboxRecord describes the host boundary used to launch a managed
// subject. It is dependency-free so lifecycle evidence does not need to know
// which platform backend produced it.
type SandboxRecord struct {
	Backend                  string    `json:"backend"`
	Enforcement              string    `json:"enforcement"`
	PolicySHA256             string    `json:"policy_sha256"`
	SubjectID                string    `json:"subject_id"`
	ExecutablePath           string    `json:"executable_path"`
	ExecutableSHA256         string    `json:"executable_sha256"`
	ObservedExecutablePath   string    `json:"observed_executable_path,omitempty"`
	ObservedExecutableSHA256 string    `json:"observed_executable_sha256,omitempty"`
	ExecutableObservedAt     time.Time `json:"executable_observed_at,omitempty"`
	CanInvokeSubject         bool      `json:"can_invoke_subject"`
	AllowedTools             []string  `json:"allowed_tools,omitempty"`
}

// AccessTelemetry describes the quality of a host access observation. A
// captured report with zero events is still only an observation window.
type AccessTelemetry struct {
	Status                       string    `json:"status"`
	Source                       string    `json:"source"`
	ProcessID                    int       `json:"process_id,omitempty"`
	ProcessIDs                   []int     `json:"process_ids,omitempty"`
	EventCount                   int       `json:"event_count"`
	ParseErrors                  int       `json:"parse_errors"`
	ProcessTreeErrors            int       `json:"process_tree_errors"`
	ExecutableSampleCount        int       `json:"executable_sample_count"`
	ExecutableSamplingIntervalMS int       `json:"executable_sampling_interval_ms"`
	ExecutableSamplingStartedAt  time.Time `json:"executable_sampling_started_at,omitempty"`
	ExecutableSamplingStoppedAt  time.Time `json:"executable_sampling_stopped_at,omitempty"`
	ExecutableObservationCount   int       `json:"executable_observation_count"`
	ExecutableObservationErrors  int       `json:"executable_observation_errors"`
	Reason                       string    `json:"reason,omitempty"`
}

// ExecutableObservation is the dependency-free lifecycle form of a host
// process identity observation. The evidence package writes it as a separate
// append-only stream.
type ExecutableObservation struct {
	Timestamp time.Time `json:"timestamp"`
	PID       int       `json:"pid"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
}

// AccessEvent is kept out of run.json and written by the evidence package as
// a separate subject-access JSONL stream.
type AccessEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Process   string    `json:"process"`
	PID       int       `json:"pid"`
	Decision  string    `json:"decision"`
	Operation string    `json:"operation"`
	Resource  string    `json:"resource"`
}

// Record is embedded in a run artifact to show how the subject came to be
// available. A managed process is still not an isolation boundary.
type Record struct {
	Mode                   string                  `json:"mode"`
	Command                []string                `json:"command,omitempty"`
	WorkingDir             string                  `json:"working_dir,omitempty"`
	BaseURL                string                  `json:"base_url,omitempty"`
	ReadyPath              string                  `json:"ready_path,omitempty"`
	StartedAt              *time.Time              `json:"started_at,omitempty"`
	ReadyAt                *time.Time              `json:"ready_at,omitempty"`
	StoppedAt              *time.Time              `json:"stopped_at,omitempty"`
	Outcome                string                  `json:"outcome"`
	ExitCode               *int                    `json:"exit_code,omitempty"`
	Sandbox                *SandboxRecord          `json:"sandbox,omitempty"`
	Access                 *AccessTelemetry        `json:"access_telemetry,omitempty"`
	Events                 []Event                 `json:"events"`
	AccessEvents           []AccessEvent           `json:"-"`
	ExecutableObservations []ExecutableObservation `json:"-"`
}

// External returns a record for a subject that was supplied by another
// process. It makes the weaker lifecycle claim explicit in a run artifact.
func External(baseURL string, now func() time.Time) Record {
	if now == nil {
		now = time.Now
	}
	return Record{
		Mode:    "external-url",
		BaseURL: strings.TrimRight(baseURL, "/"),
		Outcome: "not-managed",
		Events: []Event{{
			Sequence:  1,
			Timestamp: now().UTC(),
			Kind:      "subject.external-url.accepted",
			Detail:    "subject lifecycle was outside Sorna",
		}},
	}
}

// Process is a running managed subject. Close should be called exactly once
// by the owner after the runner finishes.
type Process struct {
	cmd       *exec.Cmd
	done      chan struct{}
	client    *http.Client
	shutdown  time.Duration
	probe     time.Duration
	now       func() time.Time
	stderr    lockedBuffer
	mu        sync.Mutex
	waitErr   error
	closed    bool
	stopAsked bool
	record    Record
}

// lockedBuffer is used for child stderr because os/exec copies the pipe into
// the writer asynchronously while lifecycle startup may read it to explain a
// failed readiness check.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(value)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

// Start launches the subject and waits until its readiness endpoint returns a
// successful HTTP response. A failed startup is torn down before the error is
// returned so a partial launch cannot become an orphaned subject.
func Start(ctx context.Context, config Config) (*Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(config.Command) == 0 || strings.TrimSpace(config.Command[0]) == "" {
		return nil, fmt.Errorf("subject command must contain an executable")
	}
	base, err := parseBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	if config.ReadyPath == "" {
		config.ReadyPath = "/healthz"
	}
	readyURL, err := readinessURL(base, config.ReadyPath)
	if err != nil {
		return nil, err
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = 10 * time.Second
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 5 * time.Second
	}
	if config.ProbeTimeout <= 0 {
		config.ProbeTimeout = time.Second
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 50 * time.Millisecond
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	client := config.Client
	if client == nil {
		client = &http.Client{}
	}

	command := exec.Command(config.Command[0], config.Command[1:]...)
	command.Dir = config.Dir
	if len(config.Env) > 0 {
		command.Env = append(os.Environ(), config.Env...)
	}
	command.Stdout = io.Discard
	process := &Process{
		cmd:      command,
		done:     make(chan struct{}),
		client:   client,
		shutdown: config.ShutdownTimeout,
		probe:    config.ProbeTimeout,
		now:      config.Now,
		record: Record{
			Mode:       "managed-process",
			Command:    append([]string(nil), config.RecordCommand...),
			WorkingDir: config.Dir,
			BaseURL:    strings.TrimRight(base.String(), "/"),
			ReadyPath:  config.ReadyPath,
			Outcome:    "starting",
			Sandbox:    cloneSandboxRecord(config.Sandbox),
			Access:     cloneAccessTelemetry(config.Access),
			Events:     make([]Event, 0, 5),
		},
	}
	if len(process.record.Command) == 0 {
		process.record.Command = append([]string(nil), config.Command...)
	}
	command.Stderr = &process.stderr
	prepareCommand(command)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start subject: %w", err)
	}
	startedAt := config.Now().UTC()
	process.mu.Lock()
	process.record.StartedAt = &startedAt
	process.mu.Unlock()
	process.addEvent("subject.process.started", "command launched")
	go process.wait()

	startupContext, cancel := context.WithTimeout(ctx, config.StartupTimeout)
	defer cancel()
	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()
	var lastProbeErr error
	for {
		select {
		case <-process.done:
			process.mu.Lock()
			process.record.Outcome = "exited-before-ready"
			waitErr := process.waitErr
			stderr := strings.TrimSpace(process.stderr.String())
			process.mu.Unlock()
			return nil, startupError("subject exited before readiness", waitErr, stderr)
		default:
		}

		ready, probeErr := process.probeReady(startupContext, readyURL)
		if ready {
			readyAt := config.Now().UTC()
			process.mu.Lock()
			process.record.ReadyAt = &readyAt
			process.record.Outcome = "ready"
			process.mu.Unlock()
			process.addEvent("subject.ready", "readiness endpoint returned a successful response")
			return process, nil
		}
		if probeErr != nil {
			lastProbeErr = probeErr
		}

		select {
		case <-process.done:
			continue
		case <-startupContext.Done():
			_ = process.Close(context.Background())
			return nil, startupError("subject readiness timeout", lastProbeErr, process.stderrText())
		case <-ticker.C:
		}
	}
}

func (process *Process) wait() {
	err := process.cmd.Wait()
	process.mu.Lock()
	process.waitErr = err
	if process.cmd.ProcessState != nil {
		code := process.cmd.ProcessState.ExitCode()
		process.record.ExitCode = &code
	}
	if !process.stopAsked {
		process.record.Outcome = "exited"
	}
	process.mu.Unlock()
	close(process.done)
}

func (process *Process) probeReady(ctx context.Context, readyURL string) (bool, error) {
	probeContext, cancel := context.WithTimeout(ctx, process.probe)
	defer cancel()
	request, err := http.NewRequestWithContext(probeContext, http.MethodGet, readyURL, nil)
	if err != nil {
		return false, err
	}
	response, err := process.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("readiness endpoint returned HTTP %d", response.StatusCode)
	}
	return true, nil
}

// Close stops the subject, escalating to a process-group kill if graceful
// termination does not complete within the configured shutdown window.
func (process *Process) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	process.mu.Lock()
	if process.closed {
		process.mu.Unlock()
		return nil
	}
	process.closed = true
	process.stopAsked = true
	process.mu.Unlock()
	process.addEvent("subject.stop.requested", "graceful process-group termination requested")

	select {
	case <-process.done:
		process.finishStop(false)
		return nil
	default:
	}
	if err := signalCommand(process.cmd, false); err != nil {
		select {
		case <-process.done:
			process.finishStop(false)
			return nil
		default:
		}
		return fmt.Errorf("stop subject: %w", err)
	}

	shutdownContext, cancel := context.WithTimeout(ctx, process.shutdown)
	defer cancel()
	select {
	case <-process.done:
		process.finishStop(false)
		return nil
	case <-shutdownContext.Done():
		process.addEvent("subject.stop.escalated", "graceful termination timed out; process group killed")
		if err := signalCommand(process.cmd, true); err != nil {
			select {
			case <-process.done:
				process.finishStop(true)
				return nil
			default:
			}
			return fmt.Errorf("kill subject: %w", err)
		}
		<-process.done
		process.finishStop(true)
		return nil
	}
}

// Record returns a stable snapshot of the lifecycle evidence collected so
// far. The event list is copied so callers cannot mutate process state.
func (process *Process) Record() Record {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.recordSnapshotLocked()
}

// PID returns the managed root process ID after Start succeeds. A
// Seatbelt-wrapped command keeps the same PID while it execs the subject;
// access telemetry may additionally record descendants observed by its host
// collector.
func (process *Process) PID() int {
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.cmd.Process == nil {
		return 0
	}
	return process.cmd.Process.Pid
}

func (process *Process) finishStop(forced bool) {
	stoppedAt := process.now().UTC()
	process.mu.Lock()
	process.record.StoppedAt = &stoppedAt
	if forced {
		process.record.Outcome = "killed"
	} else {
		process.record.Outcome = "stopped"
	}
	outcome := process.record.Outcome
	process.mu.Unlock()
	process.addEvent("subject.stopped", outcome)
}

func (process *Process) addEvent(kind, detail string) {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.record.Events = append(process.record.Events, Event{
		Sequence:  len(process.record.Events) + 1,
		Timestamp: process.now().UTC(),
		Kind:      kind,
		Detail:    detail,
	})
}

func (process *Process) recordSnapshotLocked() Record {
	record := process.record
	record.Command = append([]string(nil), process.record.Command...)
	record.Events = append([]Event(nil), process.record.Events...)
	record.Sandbox = cloneSandboxRecord(process.record.Sandbox)
	record.Access = cloneAccessTelemetry(process.record.Access)
	record.AccessEvents = append([]AccessEvent(nil), process.record.AccessEvents...)
	record.ExecutableObservations = append([]ExecutableObservation(nil), process.record.ExecutableObservations...)
	return record
}

func cloneSandboxRecord(record *SandboxRecord) *SandboxRecord {
	if record == nil {
		return nil
	}
	copy := *record
	copy.AllowedTools = append([]string(nil), record.AllowedTools...)
	return &copy
}

func cloneAccessTelemetry(telemetry *AccessTelemetry) *AccessTelemetry {
	if telemetry == nil {
		return nil
	}
	copy := *telemetry
	copy.ProcessIDs = append([]int(nil), telemetry.ProcessIDs...)
	return &copy
}

func (process *Process) stderrText() string {
	process.mu.Lock()
	defer process.mu.Unlock()
	return strings.TrimSpace(process.stderr.String())
}

func startupError(prefix string, cause error, stderr string) error {
	detail := prefix
	if cause != nil {
		detail += ": " + cause.Error()
	}
	if stderr != "" {
		detail += "; stderr: " + stderr
	}
	return fmt.Errorf("%s", detail)
}

func parseBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base URL must be an absolute URL: %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("subject lifecycle requires an HTTP or HTTPS base URL: %q", raw)
	}
	return parsed, nil
}

func readinessURL(base *url.URL, readyPath string) (string, error) {
	if !strings.HasPrefix(readyPath, "/") {
		return "", fmt.Errorf("readiness path must begin with /: %q", readyPath)
	}
	resolved := *base
	resolved.Path = strings.TrimRight(base.Path, "/") + readyPath
	resolved.RawPath = ""
	resolved.RawQuery = ""
	resolved.Fragment = ""
	return resolved.String(), nil
}
