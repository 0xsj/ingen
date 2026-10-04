// Package agentlaunch builds fixed, noninteractive fresh-agent invocations.
// It never handles credential values; any optional broker route is bound to a
// local endpoint and must also be enforced by the caller's host policy. This
// package provides no provider or model attestation.
package agentlaunch

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	CodexAgent        = "codex"
	ContextSchema     = "ingen.sentinel-agent-context/v1"
	MaxPromptBytes    = 64 << 10
	StdinPromptSource = "hash-pinned-prompt-snapshot"
)

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,63}$`)

// CodexContext records the exact fresh, noninteractive invocation boundary.
// It contains path and digest metadata, never prompt or credential contents.
type CodexContext struct {
	Schema               string         `json:"schema"`
	Agent                string         `json:"agent"`
	Mode                 string         `json:"mode"`
	ExecutionID          string         `json:"execution_id"`
	Root                 string         `json:"root"`
	WorkingDirectory     string         `json:"working_directory"`
	HomePath             string         `json:"home_path"`
	CodexHomePath        string         `json:"codex_home_path"`
	PromptSourcePath     string         `json:"prompt_source_path"`
	PromptSourceSHA256   string         `json:"prompt_source_sha256"`
	PromptSnapshotPath   string         `json:"prompt_snapshot_path"`
	PromptSnapshotSHA256 string         `json:"prompt_snapshot_sha256"`
	PromptBytes          int            `json:"prompt_bytes"`
	Model                string         `json:"model,omitempty"`
	Args                 []string       `json:"args"`
	Stdin                string         `json:"stdin"`
	NetworkMode          string         `json:"network_mode"`
	Broker               *BrokerContext `json:"broker,omitempty"`
}

// BrokerContext identifies the local Responses proxy configuration. It never
// records the credential value or the child-scoped bearer token.
type BrokerContext struct {
	Provider         string `json:"provider"`
	Endpoint         string `json:"endpoint"`
	CredentialSource string `json:"credential_source"`
	CredentialEnv    string `json:"credential_env"`
	MaxRequests      int    `json:"max_requests"`
	MaxOutputTokens  int    `json:"max_output_tokens"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
}

type CodexRequest struct {
	ExecutionID          string
	Root, WorkingDir     string
	HomePath, CodexHome  string
	PromptSourcePath     string
	PromptSourceSHA256   string
	PromptSnapshotPath   string
	PromptSnapshotSHA256 string
	PromptBytes          int
	Model                string
	Broker               *BrokerContext
}

// BuildCodex returns the supported Codex command arguments and its public
// context record. The final "-" selects exact prompt bytes from stdin.
func BuildCodex(request CodexRequest) (CodexContext, []string, error) {
	if request.ExecutionID == "" || !cleanAbsolute(request.Root) || !cleanAbsolute(request.WorkingDir) || !cleanAbsolute(request.HomePath) || !cleanAbsolute(request.CodexHome) {
		return CodexContext{}, nil, fmt.Errorf("Codex launch requires a unique execution ID and clean absolute root, working directory, HOME, and CODEX_HOME")
	}
	if !cleanRelative(request.PromptSourcePath) || !cleanRelative(request.PromptSnapshotPath) {
		return CodexContext{}, nil, fmt.Errorf("Codex prompt paths must be normalized and project-relative")
	}
	if request.PromptBytes < 1 || request.PromptBytes > MaxPromptBytes || !validDigest(request.PromptSourceSHA256) || request.PromptSourceSHA256 != request.PromptSnapshotSHA256 {
		return CodexContext{}, nil, fmt.Errorf("Codex prompt requires matching valid source and snapshot digests and a bounded nonempty prompt")
	}
	if request.Model != "" && !modelName.MatchString(request.Model) {
		return CodexContext{}, nil, fmt.Errorf("Codex model must be a safe ASCII model token")
	}
	if request.Broker != nil {
		if request.Model == "" || request.Broker.Provider != "openai-responses" || request.Broker.CredentialSource != "environment" || !validCredentialEnv(request.Broker.CredentialEnv) || request.Broker.MaxRequests < 1 || request.Broker.MaxRequests > 64 || request.Broker.MaxOutputTokens < 1 || request.Broker.MaxOutputTokens > 8192 || request.Broker.TimeoutSeconds < 1 || request.Broker.TimeoutSeconds > 1800 || !validBrokerEndpoint(request.Broker.Endpoint) {
			return CodexContext{}, nil, fmt.Errorf("Codex broker launch requires a pinned model, valid local endpoint, credential environment name, and bounded limits")
		}
	}
	args := []string{
		"--no-daemon", "exec", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--skip-git-repo-check", "--json", "--color", "never",
		"-c", "memories.use_memories=false",
		"-c", "memories.generate_memories=false",
		"-c", "project_doc_max_bytes=0",
	}
	if request.Model != "" {
		args = append(args, "-m", request.Model)
	}
	if request.Broker != nil {
		args = append(args,
			"-c", "model_provider=ingen_broker",
			"-c", "model_providers.ingen_broker.name=InGenBroker",
			"-c", "model_providers.ingen_broker.base_url="+request.Broker.Endpoint,
			"-c", "model_providers.ingen_broker.env_key=INGEN_CODEX_BROKER_TOKEN",
			"-c", "model_providers.ingen_broker.wire_api=responses",
			"-c", "model_providers.ingen_broker.requires_openai_auth=false",
			"-c", "model_providers.ingen_broker.supports_websockets=false",
			"-c", "model_providers.ingen_broker.request_max_retries=0",
			"-c", "model_providers.ingen_broker.stream_max_retries=0",
			"-c", "web_search=disabled",
			"-c", "features.multi_agent=false",
			"-c", "features.hooks=false",
			"-c", "features.remote_plugin=false",
			"-c", "features.shell_snapshot=false",
			"-c", "approval_policy=never",
			"-c", "sandbox_mode=danger-full-access",
		)
	}
	args = append(args, "-")
	context := CodexContext{
		Schema: ContextSchema, Agent: "codex-cli", Mode: "exec-ephemeral", ExecutionID: request.ExecutionID,
		Root: request.Root, WorkingDirectory: request.WorkingDir, HomePath: request.HomePath, CodexHomePath: request.CodexHome,
		PromptSourcePath: request.PromptSourcePath, PromptSourceSHA256: request.PromptSourceSHA256,
		PromptSnapshotPath: request.PromptSnapshotPath, PromptSnapshotSHA256: request.PromptSnapshotSHA256,
		PromptBytes: request.PromptBytes, Model: request.Model, Args: append([]string{}, args...),
		Stdin: StdinPromptSource, NetworkMode: "disabled", Broker: request.Broker,
	}
	if request.Broker != nil {
		context.NetworkMode = "allowlist"
	}
	return context, args, nil
}

func validCredentialEnv(value string) bool {
	if value == "" || value == "INGEN_CODEX_BROKER_TOKEN" {
		return false
	}
	for index, char := range value {
		if !(char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func validBrokerEndpoint(value string) bool {
	if !strings.HasPrefix(value, "http://127.0.0.1:") || !strings.HasSuffix(value, "/v1") {
		return false
	}
	port := strings.TrimSuffix(strings.TrimPrefix(value, "http://127.0.0.1:"), "/v1")
	if port == "" || strings.HasPrefix(port, "0") {
		return false
	}
	for _, char := range port {
		if char < '0' || char > '9' {
			return false
		}
	}
	n := 0
	for _, char := range port {
		n = n*10 + int(char-'0')
		if n > 65535 {
			return false
		}
	}
	return n > 0
}

// ValidatePrompt enforces a bounded, nonempty UTF-8 prompt without trimming or
// rewriting bytes. Newlines and trailing whitespace are preserved verbatim.
func ValidatePrompt(prompt []byte) error {
	if len(prompt) == 0 || len(prompt) > MaxPromptBytes {
		return fmt.Errorf("Codex prompt must contain between 1 and %d bytes", MaxPromptBytes)
	}
	if !utf8.Valid(prompt) || strings.ContainsRune(string(prompt), '\x00') || strings.TrimSpace(string(prompt)) == "" {
		return fmt.Errorf("Codex prompt must be nonblank UTF-8 without NUL bytes")
	}
	return nil
}

func cleanAbsolute(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value
}

func cleanRelative(value string) bool {
	clean := filepath.Clean(filepath.FromSlash(value))
	return value != "" && !filepath.IsAbs(value) && !strings.ContainsRune(value, '\\') &&
		clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) && filepath.ToSlash(clean) == value
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
