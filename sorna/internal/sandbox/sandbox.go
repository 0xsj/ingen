// Package sandbox prepares commands for execution under a capability policy.
package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ingen/sorna/internal/policy"
)

// Prepared is a command wrapped by a host enforcement backend.
type Prepared struct {
	Command          []string
	Backend          string
	Enforcement      string
	PolicySHA256     string
	NetworkMode      string
	SubjectID        string
	ExecutablePath   string
	ExecutableSHA256 string
	CanInvokeSubject bool
	AllowedTools     []string
	AllowedReads     []string
	AllowedWrites    []string
	DeniedPaths      []string
}

// NetworkRule is the normalized subset of a policy that a host backend can
// apply. A missing policy direction defaults to outbound for compatibility
// with the original v1 allowlist shape.
type NetworkRule struct {
	Direction string
	Host      string
	Ports     []int64
}

// Prepare resolves policy roots and delegates to the platform enforcement
// backend. The returned command can be passed directly to exec.Command.
func Prepare(command []string, rootDir string, sealed policy.Sealed) (Prepared, error) {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return Prepared{}, fmt.Errorf("sandbox command must contain an executable")
	}
	if rootDir == "" {
		rootDir = "."
	}
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve sandbox root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Prepared{}, fmt.Errorf("canonicalize sandbox root: %w", err)
	}
	if problems := policy.Validate(sealed.Document); len(problems) > 0 {
		return Prepared{}, fmt.Errorf("sandbox policy is invalid: %s", strings.Join(problems, "; "))
	}
	networkMode, networkRules, err := networkCapabilities(sealed.Document)
	if err != nil {
		return Prepared{}, err
	}
	subjectID, canInvokeSubject, allowedTools, err := processCapabilities(sealed.Document)
	if err != nil {
		return Prepared{}, err
	}
	readPaths, writePaths, denyPaths, err := filesystemPaths(sealed, root)
	if err != nil {
		return Prepared{}, err
	}
	commandPath, err := exec.LookPath(command[0])
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve sandbox executable %q: %w", command[0], err)
	}
	if !filepath.IsAbs(commandPath) {
		commandPath, err = filepath.Abs(commandPath)
		if err != nil {
			return Prepared{}, fmt.Errorf("resolve sandbox executable %q: %w", command[0], err)
		}
	}
	commandPath = canonicalizeExistingParent(commandPath)
	executableSHA256, err := hashExecutable(commandPath)
	if err != nil {
		return Prepared{}, err
	}
	prepared, err := preparePlatform(command, commandPath, root, readPaths, writePaths, denyPaths, networkRules, allowedTools)
	if err != nil {
		return Prepared{}, err
	}
	prepared.PolicySHA256 = sealed.SHA256
	prepared.NetworkMode = networkMode
	prepared.SubjectID = subjectID
	prepared.ExecutablePath = commandPath
	prepared.ExecutableSHA256 = executableSHA256
	prepared.CanInvokeSubject = canInvokeSubject
	prepared.AllowedTools = allowedTools
	prepared.AllowedReads = readPaths
	prepared.AllowedWrites = writePaths
	prepared.DeniedPaths = denyPaths
	return prepared, nil
}

func hashExecutable(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open sandbox executable %q: %w", path, err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash sandbox executable %q: %w", path, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func processCapabilities(document policy.Document) (string, bool, []string, error) {
	subjectID, err := policy.SubjectID(document)
	if err != nil {
		return "", false, nil, err
	}
	process, ok := document.Policy["process"].(map[string]any)
	if !ok {
		return "", false, nil, fmt.Errorf("sandbox policy process must be an object")
	}
	canInvokeSubject, ok := process["can_invoke_subject"].(bool)
	if !ok {
		return "", false, nil, fmt.Errorf("sandbox policy process.can_invoke_subject must be a boolean")
	}
	rawTools, present := process["allowed_tools"]
	if !present {
		return subjectID, canInvokeSubject, nil, nil
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return "", false, nil, fmt.Errorf("sandbox policy process.allowed_tools must be a list")
	}
	resolved := make([]string, 0, len(tools))
	for index, rawTool := range tools {
		entry, ok := rawTool.(map[string]any)
		if !ok {
			return "", false, nil, fmt.Errorf("sandbox policy process.allowed_tools[%d] must be an object", index)
		}
		name, ok := entry["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return "", false, nil, fmt.Errorf("sandbox policy process.allowed_tools[%d].name must be a string", index)
		}
		path, err := exec.LookPath(name)
		if err != nil {
			return "", false, nil, fmt.Errorf("resolve sandbox tool %q: %w", name, err)
		}
		if !filepath.IsAbs(path) {
			path, err = filepath.Abs(path)
			if err != nil {
				return "", false, nil, fmt.Errorf("resolve sandbox tool %q: %w", name, err)
			}
		}
		resolved = append(resolved, canonicalizeExistingParent(path))
	}
	return subjectID, canInvokeSubject, resolved, nil
}

// PathCovered reports whether target is inside one of the supplied capability
// roots. It uses the same canonical path behavior as Prepare and is intended
// for callers that bind a specific output or input argument to a policy root.
func PathCovered(roots []string, target string) bool {
	resolvedTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	resolvedTarget = canonicalizeExistingParent(resolvedTarget)
	for _, rawRoot := range roots {
		resolvedRoot, err := filepath.Abs(rawRoot)
		if err != nil {
			continue
		}
		resolvedRoot = canonicalizeExistingParent(resolvedRoot)
		relative, err := filepath.Rel(resolvedRoot, resolvedTarget)
		if err != nil {
			continue
		}
		if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func networkCapabilities(document policy.Document) (string, []NetworkRule, error) {
	network, ok := document.Policy["network"].(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("sandbox policy network must be an object")
	}
	mode, ok := network["mode"].(string)
	if !ok || mode == "" {
		return "", nil, fmt.Errorf("sandbox policy network.mode must be a string")
	}
	switch mode {
	case "disabled":
		return mode, nil, nil
	case "allowlist":
		rawEntries, ok := network["allow"].([]any)
		if !ok {
			return "", nil, fmt.Errorf("sandbox policy network.allow must be a list")
		}
		rules := make([]NetworkRule, 0, len(rawEntries))
		for index, rawEntry := range rawEntries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("sandbox policy network.allow[%d] must be an object", index)
			}
			host, ok := entry["host"].(string)
			if !ok || strings.TrimSpace(host) == "" {
				return "", nil, fmt.Errorf("sandbox policy network.allow[%d].host must be a string", index)
			}
			direction := "outbound"
			if value, present := entry["direction"]; present {
				direction, ok = value.(string)
				if !ok || (direction != "inbound" && direction != "outbound" && direction != "both") {
					return "", nil, fmt.Errorf("sandbox policy network.allow[%d].direction is invalid", index)
				}
			}
			rawPorts, ok := entry["ports"].([]any)
			if !ok {
				return "", nil, fmt.Errorf("sandbox policy network.allow[%d].ports must be a list", index)
			}
			ports := make([]int64, 0, len(rawPorts))
			for portIndex, rawPort := range rawPorts {
				port, valid := policyInteger(rawPort)
				if !valid || port < 1 || port > 65535 {
					return "", nil, fmt.Errorf("sandbox policy network.allow[%d].ports[%d] is invalid", index, portIndex)
				}
				ports = append(ports, port)
			}
			rules = append(rules, NetworkRule{Direction: direction, Host: host, Ports: ports})
		}
		return mode, rules, nil
	case "unrestricted":
		return "", nil, fmt.Errorf("macOS Seatbelt backend does not support network.mode=unrestricted")
	default:
		return "", nil, fmt.Errorf("sandbox policy network.mode is unsupported: %q", mode)
	}
}

func policyInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		return int64(value), uint64(value) <= uint64(^uint64(0)>>1)
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(value), true
	case float64:
		return int64(value), float64(int64(value)) == value
	default:
		return 0, false
	}
}

func filesystemPaths(sealed policy.Sealed, root string) ([]string, []string, []string, error) {
	filesystem, ok := sealed.Document.Policy["filesystem"].(map[string]any)
	if !ok {
		return nil, nil, nil, fmt.Errorf("sandbox policy filesystem must be an object")
	}
	paths := func(category string) ([]string, error) {
		entries, ok := filesystem[category].([]any)
		if !ok {
			return nil, fmt.Errorf("sandbox policy filesystem.%s must be a list", category)
		}
		resolved := make([]string, 0, len(entries))
		for index, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("sandbox policy filesystem.%s[%d] must be an object", category, index)
			}
			path, ok := entry["path"].(string)
			if !ok || strings.TrimSpace(path) == "" {
				return nil, fmt.Errorf("sandbox policy filesystem.%s[%d].path must be a non-empty string", category, index)
			}
			resolvedPath, err := resolvePath(root, path)
			if err != nil {
				return nil, fmt.Errorf("sandbox policy filesystem.%s[%d]: %w", category, index, err)
			}
			resolved = append(resolved, resolvedPath)
		}
		return resolved, nil
	}
	readPaths, err := paths("read")
	if err != nil {
		return nil, nil, nil, err
	}
	writePaths, err := paths("write")
	if err != nil {
		return nil, nil, nil, err
	}
	denyPaths, err := paths("deny")
	if err != nil {
		return nil, nil, nil, err
	}
	return readPaths, writePaths, denyPaths, nil
}

func resolvePath(root, raw string) (string, error) {
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	resolved := filepath.Clean(filepath.Join(root, raw))
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("relative path escapes sandbox root: %q", raw)
	}
	return resolved, nil
}

func canonicalizeExistingParent(path string) string {
	current := filepath.Clean(path)
	suffix := make([]string, 0)
	for {
		if canonical, err := filepath.EvalSymlinks(current); err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				canonical = filepath.Join(canonical, suffix[index])
			}
			return canonical
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
