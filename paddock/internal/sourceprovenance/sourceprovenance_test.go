package sourceprovenance

import (
	"errors"
	"testing"
)

func TestDetectCapturesGitRevisionAndDirtyState(t *testing.T) {
	got := detect("/workspace/service", func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "rev-parse":
			if args[1] == "--is-inside-work-tree" {
				return "true\n", nil
			}
			return "0123456789abcdef\n", nil
		case "status":
			return " M internal/orders/order.go\n", nil
		case "diff":
			return "diff --git a/internal/orders/order.go b/internal/orders/order.go\n", nil
		case "ls-files":
			return "", nil
		default:
			return "", errors.New("unexpected git command")
		}
	})
	if got == nil || got.System != "git" || got.Revision != "0123456789abcdef" || !got.Dirty || len(got.ChangesSHA256) != 64 {
		t.Fatalf("Detect() = %#v, want Git revision, dirty=true, and a changes hash", got)
	}
}

func TestDetectReturnsNilWhenGitIsUnavailable(t *testing.T) {
	got := detect("/workspace/service", func(_ string, _ ...string) (string, error) {
		return "", errors.New("not a git worktree")
	})
	if got != nil {
		t.Fatalf("Detect() = %#v, want nil", got)
	}
}

func TestDetectReturnsNilWhenDirtyStateCannotBeRead(t *testing.T) {
	got := detect("/workspace/service", func(_ string, args ...string) (string, error) {
		if args[0] == "status" {
			return "", errors.New("status unavailable")
		}
		if args[0] == "rev-parse" && args[1] == "--is-inside-work-tree" {
			return "true", nil
		}
		return "0123456789abcdef", nil
	})
	if got != nil {
		t.Fatalf("Detect() = %#v, want nil when dirty state is unavailable", got)
	}
}
