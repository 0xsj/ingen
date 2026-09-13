package lifecycle

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestManagedProcessWaitsForReadinessAndStops(t *testing.T) {
	address := freeAddress(t)
	process, err := Start(context.Background(), Config{
		Command:         []string{os.Args[0], "-test.run=TestLifecycleHelperProcess", "--"},
		Env:             []string{"INGEN_LIFECYCLE_HELPER=1", "INGEN_LIFECYCLE_ADDR=" + address},
		BaseURL:         "http://" + address,
		ReadyPath:       "/healthz",
		StartupTimeout:  2 * time.Second,
		ShutdownTimeout: 2 * time.Second,
		PollInterval:    10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	started := process.Record()
	if started.Mode != "managed-process" || started.Outcome != "ready" {
		t.Fatalf("initial lifecycle record = %+v, want managed ready process", started)
	}
	if started.ReadyAt == nil || len(started.Events) < 2 {
		t.Fatalf("initial lifecycle record = %+v, want start and ready evidence", started)
	}

	if err := process.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := process.Record()
	if stopped.Outcome != "stopped" || stopped.StoppedAt == nil {
		t.Fatalf("stopped lifecycle record = %+v, want stopped process", stopped)
	}
	if len(stopped.Events) < 4 || stopped.Events[len(stopped.Events)-1].Kind != "subject.stopped" {
		t.Fatalf("lifecycle events = %+v, want final subject.stopped event", stopped.Events)
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("second Close() = %v, want idempotent shutdown", err)
	}
}

func TestStartReportsMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-subject")
	_, err := Start(context.Background(), Config{
		Command:        []string{missing},
		BaseURL:        "http://127.0.0.1:1",
		StartupTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "start subject") {
		t.Fatalf("Start() error = %v, want executable start error", err)
	}
}

func TestStartTimesOutWhenReadinessNeverSucceeds(t *testing.T) {
	address := freeAddress(t)
	_, err := Start(context.Background(), Config{
		Command:         []string{os.Args[0], "-test.run=TestLifecycleHelperProcess", "--"},
		Env:             []string{"INGEN_LIFECYCLE_HELPER=1", "INGEN_LIFECYCLE_ADDR=" + address, "INGEN_LIFECYCLE_NOT_READY=1"},
		BaseURL:         "http://" + address,
		ReadyPath:       "/healthz",
		StartupTimeout:  120 * time.Millisecond,
		ShutdownTimeout: time.Second,
		ProbeTimeout:    20 * time.Millisecond,
		PollInterval:    10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "readiness timeout") {
		t.Fatalf("Start() error = %v, want readiness timeout", err)
	}
}

func TestLifecycleHelperProcess(t *testing.T) {
	if os.Getenv("INGEN_LIFECYCLE_HELPER") != "1" {
		return
	}
	address := os.Getenv("INGEN_LIFECYCLE_ADDR")
	listener, err := net.Listen("tcp", address)
	if err != nil {
		os.Exit(2)
	}
	ready := os.Getenv("INGEN_LIFECYCLE_NOT_READY") != "1"
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = server.Serve(listener) }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	_ = server.Shutdown(context.Background())
	_ = listener.Close()
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
