// Command codex-fixture is an offline executable for the typed Codex profile.
// It is not Codex and never contacts a model provider.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type event struct {
	Type                   string `json:"type"`
	Mode                   string `json:"mode"`
	PID                    int    `json:"pid"`
	PromptSHA256           string `json:"prompt_sha256,omitempty"`
	PriorContextUnreadable bool   `json:"prior_context_unreadable,omitempty"`
	CredentialFree         bool   `json:"credential_free,omitempty"`
	PrivateContext         bool   `json:"private_context,omitempty"`
	NetworkDenied          bool   `json:"network_denied,omitempty"`
	Reason                 string `json:"reason,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	want := []string{
		"--no-daemon", "exec", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--skip-git-repo-check", "--json", "--color", "never", "-c", "memories.use_memories=false",
		"-c", "memories.generate_memories=false", "-c", "project_doc_max_bytes=0", "-",
	}
	if !equalArgs(os.Args[1:], want) {
		return fmt.Errorf("fixed Codex profile argv mismatch: %q", os.Args[1:])
	}
	home, codexHome := os.Getenv("HOME"), os.Getenv("CODEX_HOME")
	if home == "" || codexHome == "" || !strings.Contains(home, string(filepath.Separator)+".runtime"+string(filepath.Separator)) || !strings.Contains(codexHome, string(filepath.Separator)+".runtime"+string(filepath.Separator)) {
		return fmt.Errorf("private HOME/CODEX_HOME are missing")
	}
	for _, key := range []string{"OPENAI_API_KEY", "OPENAI_ORG_ID", "OPENAI_BASE_URL", "CODEX_API_KEY", "AZURE_OPENAI_API_KEY"} {
		if _, exists := os.LookupEnv(key); exists {
			return fmt.Errorf("credential environment variable %s was inherited", key)
		}
	}
	prompt, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10+1))
	if err != nil {
		return fmt.Errorf("read prompt stdin: %w", err)
	}
	if len(prompt) == 0 || len(prompt) > 64<<10 {
		return fmt.Errorf("prompt stdin was empty or exceeded the fixture limit")
	}
	received := filepath.Join(home, "prompt.received")
	if err := os.WriteFile(received, prompt, 0o600); err != nil {
		return fmt.Errorf("preserve prompt bytes: %w", err)
	}
	mode := promptValue(prompt, "NATIVE_FIXTURE_MODE")
	forbidden := promptValue(prompt, "FORBIDDEN_CONTEXT")
	legacyHome := promptValue(prompt, "LEGACY_HOME")
	legacyCodexHome := promptValue(prompt, "LEGACY_CODEX_HOME")
	networkProbe := promptValue(prompt, "NETWORK_PROBE_ADDR")
	if mode == "" || forbidden == "" || legacyHome == "" || legacyCodexHome == "" || networkProbe == "" {
		return fmt.Errorf("prompt omitted fixture mode or prior-context probes")
	}
	if home == legacyHome || codexHome == legacyCodexHome {
		return fmt.Errorf("fresh private context paths reused legacy state")
	}
	for _, path := range []string{forbidden, filepath.Join(legacyHome, ".codex", "session.json"), filepath.Join(legacyCodexHome, "history.jsonl")} {
		if _, err := os.ReadFile(path); err == nil {
			return fmt.Errorf("prior context was readable: %s", path)
		}
	}
	entries, err := os.ReadDir(codexHome)
	if err != nil {
		return fmt.Errorf("inspect fresh CODEX_HOME: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("fresh CODEX_HOME contains preexisting state")
	}
	if err := probeNetworkDenied(networkProbe); err != nil {
		return err
	}
	var signals chan os.Signal
	if mode == "graceful" {
		signals = make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(signals)
	} else if mode == "ignore-interrupt" {
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
		defer signal.Reset(os.Interrupt, syscall.SIGTERM)
	}
	promptHash := sha256Hex(prompt)
	writeEvent(event{
		Type: "fixture-started", Mode: mode, PID: os.Getpid(), PromptSHA256: promptHash,
		PriorContextUnreadable: true, CredentialFree: true, PrivateContext: true, NetworkDenied: true,
	})
	switch mode {
	case "complete":
		writeEvent(event{Type: "fixture-completed", Mode: mode, PID: os.Getpid(), PromptSHA256: promptHash, PriorContextUnreadable: true, CredentialFree: true, PrivateContext: true})
	case "hold":
		time.Sleep(20 * time.Second)
	case "graceful":
		return waitForSignal(promptHash, mode, signals)
	case "ignore-interrupt":
		time.Sleep(8 * time.Second)
	case "orphan":
		time.Sleep(12 * time.Second)
	default:
		return fmt.Errorf("unknown fixture mode %q", mode)
	}
	return nil
}

func waitForSignal(promptHash, mode string, signals <-chan os.Signal) error {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case received := <-signals:
		writeEvent(event{Type: "fixture-caught-signal", Mode: mode, PID: os.Getpid(), PromptSHA256: promptHash, Reason: received.String()})
		return nil
	case <-timer.C:
		return fmt.Errorf("fixture signal wait timed out")
	}
}

func probeNetworkDenied(address string) error {
	conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("sandbox allowed the fixture's loopback network probe")
	}
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return nil
	}
	return fmt.Errorf("loopback probe failed without an explicit sandbox denial: %w", err)
}

func promptValue(prompt []byte, key string) string {
	for _, line := range bytes.Split(prompt, []byte("\n")) {
		prefix := key + "="
		if bytes.HasPrefix(line, []byte(prefix)) {
			return string(bytes.TrimSuffix(line[len(prefix):], []byte("\r")))
		}
	}
	return ""
}

func equalArgs(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func writeEvent(value event) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
