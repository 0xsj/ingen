//go:build !darwin

package sandbox

import (
	"context"
	"fmt"
)

func startAccessCapture(context.Context) (AccessCapture, error) {
	return nil, fmt.Errorf("no host access telemetry backend is available on this operating system")
}
