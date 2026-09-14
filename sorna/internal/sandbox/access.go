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

// AccessReport contains events observed while an enforced process was alive.
// An empty Events slice is not evidence that no access was attempted.
type AccessReport struct {
	Source      string
	Events      []AccessEvent
	ParseErrors int
}

// AccessCapture is the platform-specific host telemetry handle.
type AccessCapture interface {
	Stop(processID int) (AccessReport, error)
}

// StartAccessCapture starts the platform host telemetry collector. A caller
// should start it before the enforced process and stop it after that process
// exits so the observation window covers the whole execution.
func StartAccessCapture(ctx context.Context) (AccessCapture, error) {
	return startAccessCapture(ctx)
}
