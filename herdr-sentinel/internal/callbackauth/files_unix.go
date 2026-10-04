//go:build darwin || linux

package callbackauth

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

func readPrivateKeyFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect callback key: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("callback key must be a nonsymlink regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open callback key: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("callback key changed while opening")
	}
	mode := opened.Mode()
	if mode.Perm() != 0o600 && mode.Perm() != 0o400 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, fmt.Errorf("callback key must have mode 0600 or 0400 with no special bits")
	}
	stat, ok := opened.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, fmt.Errorf("callback key must be owned by the effective user")
	}
	contents, err := io.ReadAll(io.LimitReader(file, MaxKeyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read callback key: %w", err)
	}
	return contents, nil
}

func readRegularFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fmt.Errorf("input must be a bounded nonsymlink regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("input changed while opening")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > maximum {
		return nil, fmt.Errorf("input exceeds %d bytes", maximum)
	}
	return contents, nil
}
