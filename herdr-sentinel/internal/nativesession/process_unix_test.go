//go:build unix

package nativesession

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const containedGraceHelperEnv = "INGEN_NATIVE_CONTAINED_GRACE_HELPER"

func TestContainedRoleRunnerGraceAllowsInnerReportPublication(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "inner-report")
	if os.Getenv(containedGraceHelperEnv) == "1" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT)
		defer signal.Stop(signals)
		if err := os.WriteFile(markerFromEnv()+".started", []byte("started"), 0o600); err != nil {
			t.Fatal(err)
		}
		<-signals
		// Model the contained role runner's bounded child cancellation and
		// final report publication, both of which outlast the generic grace.
		time.Sleep(2500 * time.Millisecond)
		if err := os.WriteFile(markerFromEnv(), []byte("published"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestContainedRoleRunnerGraceAllowsInnerReportPublication$")
	command.Env = append(os.Environ(), containedGraceHelperEnv+"=1", "INGEN_NATIVE_CONTAINED_GRACE_MARKER="+marker)
	runChild, stopSignals := prepareCommandRunner(containedRoleExecutionInterruptGrace)
	defer stopSignals()
	processErr, interrupted, started, _, cleanupErr := runChild(command, func() bool {
		_, err := os.Stat(marker + ".started")
		return err == nil
	})
	if processErr != nil || !interrupted || !started || cleanupErr != nil {
		t.Fatalf("contained runner result: process=%v interrupted=%v started=%v cleanup=%v", processErr, interrupted, started, cleanupErr)
	}
	contents, err := os.ReadFile(marker)
	if err != nil || string(contents) != "published" {
		t.Fatalf("inner report publication = %q, %v; outer wrapper killed inner runner too early", contents, err)
	}
}

func markerFromEnv() string { return os.Getenv("INGEN_NATIVE_CONTAINED_GRACE_MARKER") }
