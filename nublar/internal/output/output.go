// Package output publishes complete Nublar JSON artifacts.
package output

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile atomically replaces path with data after the complete file has
// been written and synced. The destination's parent directory must already
// exist, matching the command's existing output-path behavior.
func WriteFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".nublar-output-*")
	if err != nil {
		return fmt.Errorf("create temporary Nublar output in %s: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Nublar output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Nublar output permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Nublar output: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Nublar output: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Nublar output %s: %w", path, err)
	}
	return nil
}
