//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package custody

import (
	"fmt"
	"os"
	"sync"
)

var fallbackHandlingLocks sync.Map

func withExclusiveHandlingLock(path string, fn func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open handling-event lock: %w", err)
	}
	defer file.Close()
	value, _ := fallbackHandlingLocks.LoadOrStore(path, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}
