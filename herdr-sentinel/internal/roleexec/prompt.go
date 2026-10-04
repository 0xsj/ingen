package roleexec

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ingen/herdr-sentinel/internal/agentlaunch"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func loadPromptUnderRoot(root, relative string) ([]byte, error) {
	if err := validatePromptPath(root, relative); err != nil {
		return nil, err
	}
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rooted.Close()
	return readPromptUnderRoot(rooted, relative)
}

// PromptIdentity returns the exact bounded prompt bytes and digest from a
// regular, nonsymlink file beneath root. Callers may pin the digest into a
// later immutable wrapper invocation.
func PromptIdentity(root, relative string) ([]byte, string, error) {
	data, err := loadPromptUnderRoot(root, relative)
	if err != nil {
		return nil, "", err
	}
	return data, hashBytes(data), nil
}

func validatePromptPath(root, relative string) error {
	if err := validateIDFreeRelativePath(relative); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(root, relative); err != nil {
		return err
	}
	return sentinelrun.ValidatePathUnderRoot(root, relative)
}

func readPromptUnderRoot(rooted *os.Root, relative string) ([]byte, error) {
	file, err := openPromptFile(rooted, filepath.ToSlash(relative))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > agentlaunch.MaxPromptBytes {
		return nil, fmt.Errorf("prompt must be a regular file between 1 and %d bytes", agentlaunch.MaxPromptBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, agentlaunch.MaxPromptBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != info.Size() || len(data) > agentlaunch.MaxPromptBytes {
		return nil, fmt.Errorf("prompt changed while being read or exceeds %d bytes", agentlaunch.MaxPromptBytes)
	}
	if err := agentlaunch.ValidatePrompt(data); err != nil {
		return nil, err
	}
	return data, nil
}

func publishPromptSnapshot(rooted *os.Root, relative string, data []byte) error {
	if err := agentlaunch.ValidatePrompt(data); err != nil {
		return err
	}
	if err := validateIDFreeRelativePath(relative); err != nil {
		return err
	}
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
	if err := rooted.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temporary := filepath.ToSlash(relative) + ".tmp-" + hex.EncodeToString(nonce[:])
	file, err := rooted.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if n, writeErr := file.Write(data); writeErr != nil {
		_ = file.Close()
		_ = rooted.Remove(temporary)
		return writeErr
	} else if n != len(data) {
		_ = file.Close()
		_ = rooted.Remove(temporary)
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = rooted.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		_ = rooted.Remove(temporary)
		return err
	}
	if err := rooted.Link(temporary, filepath.ToSlash(relative)); err != nil {
		_ = rooted.Remove(temporary)
		return err
	}
	if err := rooted.Remove(temporary); err != nil {
		return err
	}
	if err := syncRootDirectory(rooted, directory); err != nil {
		_ = rooted.Remove(filepath.ToSlash(relative))
		_ = syncRootDirectory(rooted, directory)
		return err
	}
	return nil
}

func promptSnapshotMatches(a, b []byte) bool { return bytes.Equal(a, b) }
