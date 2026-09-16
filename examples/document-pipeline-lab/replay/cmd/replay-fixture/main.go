// Command replay-fixture starts a fresh document-pipeline subject outside
// Sorna, waits for its readiness endpoint, and then invokes Sorna replay.
// Keeping this orchestration separate preserves the replay command's contract:
// replay itself never launches or tears down a subject.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("replay-fixture", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectRoot := flags.String("project-root", ".", "repository root used to run Sorna")
	subjectCommand := flags.String("subject-command", "", "fresh subject executable")
	baseURL := flags.String("base-url", "", "absolute URL for the fresh subject")
	readyPath := flags.String("ready-path", "/healthz", "subject readiness path")
	oraclePath := flags.String("oracle", "", "canonical frozen oracle path")
	evidenceDir := flags.String("evidence", "", "stored evidence directory to replay")
	outputPath := flags.String("output", "", "shared CI result output path")
	startupTimeout := flags.Duration("startup-timeout", 10*time.Second, "maximum time to wait for subject readiness")
	var subjectArgs stringList
	flags.Var(&subjectArgs, "subject-arg", "argument passed to the subject; may be repeated")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *subjectCommand == "" || *baseURL == "" || *oraclePath == "" || *evidenceDir == "" || *outputPath == "" || len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "usage: replay-fixture --subject-command <path> --subject-arg <arg> --base-url <url> --oracle <path> --evidence <dir> --output <path> [--project-root <dir>] [--ready-path <path>]")
		return 2
	}
	readyURL, err := readinessURL(*baseURL, *readyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	subject := exec.Command(*subjectCommand, subjectArgs...)
	subject.Dir = *projectRoot
	subject.Stdout = os.Stdout
	subject.Stderr = os.Stderr
	if err := subject.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start replay fixture:", err)
		return 1
	}
	defer stopSubject(subject)

	if err := waitReady(context.Background(), readyURL, *startupTimeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	replayArgs := []string{
		"run", "./sorna/cmd/sorna", "evidence", "replay",
		"--format", "ci-result",
		"--oracle", *oraclePath,
		"--base-url", *baseURL,
		"--output", *outputPath,
		*evidenceDir,
	}
	replay := exec.Command("go", replayArgs...)
	replay.Dir = *projectRoot
	replay.Stdout = os.Stdout
	replay.Stderr = os.Stderr
	if err := replay.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "run Sorna replay:", err)
		return 1
	}
	return 0
}

func readinessURL(rawBaseURL, readyPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL must be an absolute URL: %q", rawBaseURL)
	}
	if !strings.HasPrefix(readyPath, "/") {
		return "", fmt.Errorf("ready path must start with '/': %q", readyPath)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + readyPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func waitReady(ctx context.Context, readyURL string, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("startup timeout must be positive")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
					return nil
				}
			}
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("replay fixture did not become ready at %s within %s", readyURL, timeout)
		case <-ticker.C:
		}
	}
}

func stopSubject(subject *exec.Cmd) {
	if subject.Process == nil {
		return
	}
	_ = subject.Process.Kill()
	_, _ = subject.Process.Wait()
}

type stringList []string

func (list *stringList) String() string {
	return strings.Join(*list, " ")
}

func (list *stringList) Set(value string) error {
	*list = append(*list, value)
	return nil
}
