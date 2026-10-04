//go:build unix

package provenance

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func signalContext(parent context.Context) (context.Context, func()) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
