package sandbox

import (
	"context"
	"time"
)

// AccessEvent is a normalized host-reported sandbox decision.
type AccessEvent struct {
	Timestamp time.Time
	Process   string
	PID       int
	Decision  string
	Operation string
	Resource  string
}

// ExecutableObservation is one host observation of a process identity during
// an enforced run. Repeated samples with the same identity are coalesced by
// the platform collector; a new path or digest creates a new observation.
type ExecutableObservation struct {
	Timestamp time.Time
	PID       int
	Path      string
	SHA256    string
}

// AccessReport contains events observed while an enforced process was alive.
// An empty Events slice is not evidence that no access was attempted. A
// process-tree error means the descendant PID observation had a gap.
type AccessReport struct {
	Source                      string
	Events                      []AccessEvent
	ProcessIDs                  []int
	ParseErrors                 int
	ProcessTreeErrors           int
	ExecutableSampleCount       int
	ExecutableSamplingInterval  time.Duration
	ExecutableSamplingStartedAt time.Time
	ExecutableSamplingStoppedAt time.Time
	ExecutableObservations      []ExecutableObservation
	ExecutableObservationErrors int
}

// AccessCapture is the platform-specific host telemetry handle.
type AccessCapture interface {
	Attach(processID int) error
	Stop(processID int) (AccessReport, error)
}

// StartAccessCapture starts the platform host telemetry collector. A caller
// should start it before the enforced process and stop it after that process
// exits so the observation window covers the whole execution.
func StartAccessCapture(ctx context.Context) (AccessCapture, error) {
	return startAccessCapture(ctx)
}
