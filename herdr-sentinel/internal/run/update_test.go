package run

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestUpdateFilePublishesUnderReceiptLock(t *testing.T) {
	t.Chdir(t.TempDir())
	receipt := testCIReceipt("created")
	if err := SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}
	changed, err := UpdateFile("receipt.json", func(receipt *Receipt) (bool, error) {
		if err := receipt.AppendEvent(Event{
			Type: "role-launched",
			At:   "2026-09-16T10:00:02Z",
		}); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil || !changed {
		t.Fatalf("UpdateFile() = %v, %v; want published update", changed, err)
	}
	loaded, err := LoadFile("receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("events = %d, want appended event", len(loaded.Events))
	}
	if _, err := os.Stat("receipt.json.lock"); err != nil {
		t.Fatalf("receipt lock = %v, want persistent sibling lock", err)
	}
}

func TestUpdateFileDoesNotPublishFailedOrUnchangedUpdate(t *testing.T) {
	t.Chdir(t.TempDir())
	receipt := testCIReceipt("created")
	if err := SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}
	_, err := UpdateFile("receipt.json", func(receipt *Receipt) (bool, error) {
		if err := receipt.AppendEvent(Event{Type: "not-supported", At: "2026-09-16T10:00:02Z"}); err == nil {
			return false, nil
		} else {
			return false, err
		}
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("failed UpdateFile() = %v, want callback error", err)
	}
	loaded, err := LoadFile("receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 {
		t.Fatalf("events after failed update = %d, want unchanged receipt", len(loaded.Events))
	}
	changed, err := UpdateFile("receipt.json", func(receipt *Receipt) (bool, error) {
		return false, nil
	})
	if err != nil || changed {
		t.Fatalf("unchanged UpdateFile() = %v, %v; want no-op", changed, err)
	}
}

func TestUpdateFileSerializesConcurrentWriters(t *testing.T) {
	t.Chdir(t.TempDir())
	receipt := testCIReceipt("created")
	if err := SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := UpdateFile("receipt.json", func(receipt *Receipt) (bool, error) {
			close(firstEntered)
			<-releaseFirst
			if err := receipt.AppendEvent(Event{Type: "role-launched", At: "2026-09-16T10:00:02Z"}); err != nil {
				return false, err
			}
			return true, nil
		})
		firstDone <- err
	}()
	<-firstEntered
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		_, err := UpdateFile("receipt.json", func(receipt *Receipt) (bool, error) {
			close(secondStarted)
			if err := receipt.AppendEvent(Event{Type: "role-completed", At: "2026-09-16T10:00:03Z"}); err != nil {
				return false, err
			}
			return true, nil
		})
		secondDone <- err
	}()
	select {
	case <-secondStarted:
		t.Fatal("second receipt update entered before the first writer released its lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile("receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 3 {
		t.Fatalf("events = %d, want both concurrent updates preserved", len(loaded.Events))
	}
}
