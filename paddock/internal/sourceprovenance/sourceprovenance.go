// Package sourceprovenance captures optional version-control identity for a
// source root used by a Paddock CI result.
package sourceprovenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"ingen/core/ciresult"
)

type commandRunner func(string, ...string) (string, error)

// Detect returns Git identity for root when root is inside a Git worktree.
// Version-control metadata is supplemental evidence: unavailable Git metadata
// does not make source analysis fail.
func Detect(root string) *ciresult.VCS {
	return detect(root, runGit)
}

func detect(root string, run commandRunner) *ciresult.VCS {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	inside, err := run(absolute, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return nil
	}
	revision, err := run(absolute, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(revision) == "" {
		return nil
	}
	dirtyOutput, err := run(absolute, "status", "--porcelain", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil
	}
	vcs := &ciresult.VCS{
		System:   "git",
		Revision: strings.TrimSpace(revision),
		Dirty:    strings.TrimSpace(dirtyOutput) != "",
	}
	if vcs.Dirty {
		vcs.ChangesSHA256 = changesDigest(absolute, run)
	}
	return vcs
}

func changesDigest(root string, run commandRunner) string {
	diff, err := run(root, "diff", "--no-ext-diff", "--binary", "HEAD", "--", ".")
	if err != nil {
		return ""
	}
	untrackedOutput, err := run(root, "ls-files", "--others", "--exclude-standard", "-z", "--", ".")
	if err != nil {
		return ""
	}
	paths := strings.Split(strings.TrimSuffix(untrackedOutput, "\x00"), "\x00")
	paths = filterEmpty(paths)
	sort.Strings(paths)

	hash := sha256.New()
	writePart := func(value string) {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	writePart("paddock.source-vcs-changes/v1")
	writePart(diff)
	for _, path := range paths {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return ""
		}
		writePart(path)
		_, _ = hash.Write(contents)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func filterEmpty(values []string) []string {
	filtered := values[:0]
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func runGit(root string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}
