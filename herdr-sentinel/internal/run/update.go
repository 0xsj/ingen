package run

import (
	"fmt"
	"path/filepath"
	"strings"
)

// UpdateFile serializes a read-modify-publish receipt update with a sibling
// advisory lock file. It does not write when update reports no change, which
// keeps idempotent callback retries read-only.
func UpdateFile(path string, update func(*Receipt) (bool, error)) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, fmt.Errorf("update Sentinel run: receipt path must not be empty")
	}
	if update == nil {
		return false, fmt.Errorf("update Sentinel run: update function is required")
	}
	path = filepath.Clean(path)
	lock, err := lockReceipt(path + ".lock")
	if err != nil {
		return false, fmt.Errorf("update Sentinel run: lock %s: %w", path, err)
	}
	defer func() { _ = unlockReceipt(lock) }()
	receipt, err := LoadFile(path)
	if err != nil {
		return false, fmt.Errorf("update Sentinel run: %w", err)
	}
	changed, err := update(&receipt)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := SaveFile(path, receipt); err != nil {
		return false, fmt.Errorf("update Sentinel run: %w", err)
	}
	return true, nil
}
