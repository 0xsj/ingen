// Package platformprobe reports a small set of Linux kernel sandbox features.
// It does not install a policy or claim that Sorna is ready to enforce one.
package platformprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const Schema = "ingen.linux-platform-probe/v1"

type Status string

const (
	Available     Status = "available"
	Unavailable   Status = "unavailable"
	Indeterminate Status = "indeterminate"
)

// Options names the command used to run the isolated seccomp/NNP child probe.
// ChildEnv is the child's complete environment; it is not inherited implicitly.
type Options struct {
	SeccompChildCommand []string
	SeccompChildEnv     []string
	Timeout             time.Duration
}

type Check struct {
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type AccessBit struct {
	Name                string `json:"name"`
	Domain              string `json:"domain"`
	Mask                string `json:"mask"`
	IntroducedABI       int    `json:"introduced_abi"`
	RequiredForBaseline bool   `json:"required_for_baseline"`
	Status              Status `json:"status"`
	Detail              string `json:"detail,omitempty"`
}

type LandlockReport struct {
	Status             Status      `json:"status"`
	ABI                *int        `json:"abi,omitempty"`
	RequiredAccessBits []AccessBit `json:"required_access_bits"`
	Detail             string      `json:"detail,omitempty"`
}

type ChildReport struct {
	InitialSeccompMode int   `json:"initial_seccomp_mode"`
	InitialNoNewPrivs  int   `json:"initial_no_new_privs"`
	NoNewPrivileges    Check `json:"no_new_privileges"`
	SeccompFilter      Check `json:"seccomp_filter"`
}

type Report struct {
	Schema                    string         `json:"schema"`
	Purpose                   string         `json:"purpose"`
	OperatingSystem           string         `json:"operating_system"`
	Architecture              string         `json:"architecture"`
	KernelRelease             string         `json:"kernel_release,omitempty"`
	KernelReleaseStatus       Check          `json:"kernel_release_status"`
	Landlock                  LandlockReport `json:"landlock"`
	NoNewPrivileges           Check          `json:"no_new_privileges"`
	SeccompFilter             Check          `json:"seccomp_filter"`
	ProcExecutableObservation Check          `json:"proc_executable_observation"`
	Scope                     []string       `json:"scope"`
}

// Probe queries kernel support without enforcing a sandbox. Landlock feature
// checks create and immediately close empty ruleset descriptors; they never
// add access rules or call landlock_restrict_self. Seccomp and no_new_privs are
// tested only in the supplied child process.
func Probe(options Options) Report {
	report := Report{
		Schema:              Schema,
		Purpose:             "bounded kernel capability diagnostics; no capability grants or product readiness claim",
		OperatingSystem:     runtime.GOOS,
		Architecture:        runtime.GOARCH,
		KernelReleaseStatus: Check{Status: Indeterminate, Detail: "kernel release was not queried"},
		Landlock: LandlockReport{
			Status:             Unavailable,
			RequiredAccessBits: []AccessBit{},
		},
		NoNewPrivileges:           Check{Status: Indeterminate, Detail: "owned child probe did not run"},
		SeccompFilter:             Check{Status: Indeterminate, Detail: "owned child probe did not run"},
		ProcExecutableObservation: Check{Status: Unavailable, Detail: "Linux /proc self-executable observation is not applicable"},
		Scope: []string{
			"reports kernel interfaces only; does not install or grant a sandbox policy",
			"seccomp and no_new_privs are tested on a locked OS thread in a separate child process only",
			"the seccomp probe does not test TSYNC or claim whole-process thread synchronization",
			"proc executable status describes self-observation and does not establish access to arbitrary process IDs",
			"Landlock TCP checks are port-only and do not provide or test remote-host allowlisting",
			"unavailable kernel interfaces may reflect kernel configuration or an enclosing sandbox; this probe does not identify the cause",
			"available kernel primitives do not establish Sorna or Sentinel Linux enforcement readiness",
		},
	}
	collectPlatform(&report)
	collectChild(&report, options)
	return report
}

func collectChild(report *Report, options Options) {
	if len(options.SeccompChildCommand) == 0 {
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "seccomp probe child command is not configured"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "seccomp probe child command is not configured"}
		return
	}
	timeout := options.Timeout
	if timeout <= 0 || timeout > 15*time.Second {
		timeout = 4 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, options.SeccompChildCommand[0], options.SeccompChildCommand[1:]...)
	command.Env = append([]string(nil), options.SeccompChildEnv...)
	var output limitedOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	err := command.Run()
	if err != nil {
		detail := fmt.Sprintf("owned child probe failed: %v", err)
		if ctx.Err() != nil {
			detail = "owned child probe exceeded its time limit"
		}
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: detail}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: detail}
		return
	}
	if output.overflow {
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe output exceeded the limit"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe output exceeded the limit"}
		return
	}
	if err := rejectDuplicateJSONKeys(output.Bytes()); err != nil {
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned invalid or duplicate-key JSON"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned invalid or duplicate-key JSON"}
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil || len(fields) != 4 {
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned an incomplete result"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned an incomplete result"}
		return
	}
	for _, name := range []string{"initial_seccomp_mode", "initial_no_new_privs", "no_new_privileges", "seccomp_filter"} {
		value, ok := fields[name]
		if !ok || strings.TrimSpace(string(value)) == "null" {
			report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe omitted a required result field"}
			report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe omitted a required result field"}
			return
		}
	}
	for _, name := range []string{"no_new_privileges", "seccomp_filter"} {
		var checkFields map[string]json.RawMessage
		if err := json.Unmarshal(fields[name], &checkFields); err != nil || len(checkFields) < 1 || len(checkFields) > 2 {
			report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned an invalid check object"}
			report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned an invalid check object"}
			return
		}
		status, ok := checkFields["status"]
		if !ok || strings.TrimSpace(string(status)) == "null" {
			report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe omitted a check status"}
			report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe omitted a check status"}
			return
		}
		if detail, ok := checkFields["detail"]; ok && strings.TrimSpace(string(detail)) == "null" {
			report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned a null check detail"}
			report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned a null check detail"}
			return
		}
	}
	var child ChildReport
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&child); err != nil {
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned invalid JSON"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned invalid JSON"}
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned trailing JSON"}
			report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned trailing JSON"}
			return
		}
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned trailing JSON"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned trailing JSON"}
		return
	}
	if !validStatus(child.NoNewPrivileges.Status) || !validStatus(child.SeccompFilter.Status) || child.InitialSeccompMode < 0 || child.InitialSeccompMode > 2 || child.InitialNoNewPrivs < 0 || child.InitialNoNewPrivs > 1 {
		report.NoNewPrivileges = Check{Status: Indeterminate, Detail: "owned child probe returned an invalid capability result"}
		report.SeccompFilter = Check{Status: Indeterminate, Detail: "owned child probe returned an invalid capability result"}
		return
	}
	report.NoNewPrivileges = child.NoNewPrivileges
	report.SeccompFilter = child.SeccompFilter
}

func validStatus(status Status) bool {
	return status == Available || status == Unavailable || status == Indeterminate
}

type limitedOutput struct {
	data     []byte
	overflow bool
}

func (w *limitedOutput) Write(contents []byte) (int, error) {
	const maximum = 64 << 10
	remaining := maximum - len(w.data)
	if remaining > 0 {
		if len(contents) < remaining {
			remaining = len(contents)
		}
		w.data = append(w.data, contents[:remaining]...)
	}
	if len(contents) > remaining {
		w.overflow = true
	}
	return len(contents), nil
}

func (w *limitedOutput) Bytes() []byte { return w.data }

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		if err != nil {
			return err
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
