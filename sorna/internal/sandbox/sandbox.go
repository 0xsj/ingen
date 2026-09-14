// Package sandbox prepares commands for execution under a capability policy.
package sandbox

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"ingen/sorna/internal/policy"
)

// Prepared is a command wrapped by a host enforcement backend.
type Prepared struct {
	Command       []string
	Backend       string
	Enforcement   string
	PolicySHA256  string
	AllowedReads  []string
	AllowedWrites []string
	DeniedPaths   []string
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
	if err := validateBackendPolicy(sealed.Document); err != nil {
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
	prepared, err := preparePlatform(command, commandPath, root, readPaths, writePaths, denyPaths)
	if err != nil {
		return Prepared{}, err
	}
	prepared.PolicySHA256 = sealed.SHA256
	prepared.AllowedReads = readPaths
	prepared.AllowedWrites = writePaths
	prepared.DeniedPaths = denyPaths
	return prepared, nil
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

func validateBackendPolicy(document policy.Document) error {
	network, ok := document.Policy["network"].(map[string]any)
	if !ok {
		return fmt.Errorf("sandbox policy network must be an object")
	}
	mode, ok := network["mode"].(string)
	if !ok || mode == "" {
		return fmt.Errorf("sandbox policy network.mode must be a string")
	}
	if mode != "disabled" {
		return fmt.Errorf("macOS Seatbelt backend currently requires network.mode=disabled, got %q", mode)
	}
	return nil
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
