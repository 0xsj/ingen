//go:build darwin

package sandbox

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVerifyProcessExecutableMatchesLiveProcess(t *testing.T) {
	command := exec.Command("/bin/sleep", "1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()
	identity, err := ObserveProcessExecutable(command.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(identity.Path, "/sleep") || len(identity.SHA256) != 64 {
		t.Fatalf("identity = %+v, want sleep path and SHA-256", identity)
	}
	if observed, err := VerifyProcessExecutable(command.Process.Pid, identity.Path, identity.SHA256); err != nil || observed != identity {
		t.Fatalf("VerifyProcessExecutable() = %+v, %v; want matching identity", observed, err)
	}
}

func TestVerifyProcessExecutableRejectsDigestMismatch(t *testing.T) {
	command := exec.Command("/bin/sleep", "1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()
	identity, err := ObserveProcessExecutable(command.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyProcessExecutable(command.Process.Pid, identity.Path, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("VerifyProcessExecutable() = %v, want identity mismatch", err)
	}
}

func TestVerifyProcessExecutableMatchesSandboxDescendant(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "/bin/sleep 2")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()

	path := canonicalizeExistingParent("/bin/sleep")
	digest, err := hashExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := VerifyProcessExecutable(command.Process.Pid, path, digest)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Path != path || observed.SHA256 != digest {
		t.Fatalf("observed = %+v, want %s (%s)", observed, path, digest)
	}
}
