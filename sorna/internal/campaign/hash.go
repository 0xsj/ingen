package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// HashTree returns a deterministic SHA-256 identity for a directory's regular
// files. Relative paths and file bytes are included; directory metadata and
// filesystem traversal order are not.
func HashTree(root string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("hash tree root is not a directory: %s", root)
	}

	type file struct {
		path string
		rel  string
	}
	files := make([]file, 0)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("hash tree does not support symbolic links: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("hash tree does not support non-regular file: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, file{path: path, rel: filepath.ToSlash(rel)})
		return nil
	}); err != nil {
		return "", err
	}

	// WalkDir is currently lexical, but sorting here makes the identity an
	// explicit contract rather than an implementation detail.
	sort.Slice(files, func(left, right int) bool {
		return files[left].rel < files[right].rel
	})
	hash := sha256.New()
	for _, item := range files {
		contents, err := os.ReadFile(item.path)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(item.rel))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(contents)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
