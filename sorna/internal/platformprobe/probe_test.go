package platformprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProbeProducesStrictDiagnosticReport(t *testing.T) {
	report := Probe(testChildOptions("valid"))
	if report.Schema != Schema || report.OperatingSystem != runtime.GOOS || report.Architecture != runtime.GOARCH {
		t.Fatalf("report identity = %+v", report)
	}
	if strings.Contains(strings.ToLower(report.Purpose), "ready") || len(report.Scope) < 3 {
		t.Fatalf("report makes readiness claim or omits scope limitations: %+v", report)
	}
	if !validStatus(report.NoNewPrivileges.Status) || !validStatus(report.SeccompFilter.Status) {
		t.Fatalf("child statuses are invalid: %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"schema", "purpose", "operating_system", "architecture", "landlock", "no_new_privileges", "seccomp_filter", "proc_executable_observation", "scope"} {
		if _, ok := decoded[required]; !ok {
			t.Fatalf("serialized report omitted required field %q: %s", required, encoded)
		}
	}
	if runtime.GOOS != "linux" && report.Landlock.Status != Unavailable {
		t.Fatalf("non-Linux Landlock status = %q; want unavailable", report.Landlock.Status)
	}
}

func TestCollectChildRejectsInvalidOutput(t *testing.T) {
	for _, mode := range []string{"duplicate", "trailing", "missing", "null", "nested-null", "unknown", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			report := Report{}
			collectChild(&report, testChildOptions(mode))
			if report.NoNewPrivileges.Status != Indeterminate || report.SeccompFilter.Status != Indeterminate {
				t.Fatalf("invalid child output accepted: nnp=%+v seccomp=%+v", report.NoNewPrivileges, report.SeccompFilter)
			}
		})
	}
}

func TestCollectChildBoundsExecutionTime(t *testing.T) {
	report := Report{}
	options := testChildOptions("sleep")
	options.Timeout = 40 * time.Millisecond
	collectChild(&report, options)
	if report.NoNewPrivileges.Status != Indeterminate || report.SeccompFilter.Status != Indeterminate || !strings.Contains(report.NoNewPrivileges.Detail, "time limit") {
		t.Fatalf("timed child status = %+v / %+v", report.NoNewPrivileges, report.SeccompFilter)
	}
}

func TestProbeChildHelper(t *testing.T) {
	mode := os.Getenv("INGEN_PLATFORM_PROBE_TEST_CHILD")
	if mode == "" {
		return
	}
	switch mode {
	case "valid":
		_ = json.NewEncoder(os.Stdout).Encode(RunSeccompProbeChild())
	case "duplicate":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":0,"initial_seccomp_mode":0,"initial_no_new_privs":0,"no_new_privileges":{"status":"available"},"seccomp_filter":{"status":"available"}}`)
	case "trailing":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":0,"initial_no_new_privs":0,"no_new_privileges":{"status":"available"},"seccomp_filter":{"status":"available"}} {}`)
	case "missing":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":0,"initial_no_new_privs":0,"no_new_privileges":{"status":"available"}}`)
	case "null":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":null,"initial_no_new_privs":0,"no_new_privileges":{"status":"available"},"seccomp_filter":{"status":"available"}}`)
	case "nested-null":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":0,"initial_no_new_privs":0,"no_new_privileges":{"status":"available","detail":null},"seccomp_filter":{"status":"available"}}`)
	case "unknown":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":0,"initial_no_new_privs":0,"no_new_privileges":{"status":"available"},"seccomp_filter":{"status":"available"},"extra":true}`)
	case "overflow":
		_, _ = io.WriteString(os.Stdout, `{"initial_seccomp_mode":0,"initial_no_new_privs":0,"no_new_privileges":{"status":"available"},"seccomp_filter":{"status":"available"}}`+strings.Repeat(" ", 70<<10))
	case "sleep":
		time.Sleep(time.Second)
	default:
		fmt.Fprintln(os.Stdout, "unexpected test child mode")
		os.Exit(1)
	}
	os.Exit(0)
}

func testChildOptions(mode string) Options {
	return Options{
		SeccompChildCommand: []string{os.Args[0], "-test.run=^TestProbeChildHelper$"},
		SeccompChildEnv:     []string{"INGEN_PLATFORM_PROBE_TEST_CHILD=" + mode},
	}
}

func TestLimitedOutputBoundsBytes(t *testing.T) {
	var bounded limitedOutput
	if n, err := bounded.Write(bytes.Repeat([]byte("x"), (64<<10)+1)); err != nil || n != (64<<10)+1 || !bounded.overflow || len(bounded.Bytes()) != (64<<10) {
		t.Fatalf("bounded writer n=%d err=%v overflow=%v bytes=%d", n, err, bounded.overflow, len(bounded.Bytes()))
	}
}
