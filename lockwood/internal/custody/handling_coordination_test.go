package custody

import (
	"testing"
	"time"
)

func TestMemoryHandlingEventLockSerializesPerCustody(t *testing.T) {
	store := NewMemory()
	const custodyID = "lockwood-handling-lock-memory"
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- store.withHandlingEventLock(custodyID, func() error {
			close(firstEntered)
			<-release
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- store.withHandlingEventLock(custodyID, func() error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second memory handling-event operation entered while first was held")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestFilesystemHandlingEventLockSerializesAcrossStoreInstances(t *testing.T) {
	root := t.TempDir()
	first, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-handling-lock-filesystem"
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- first.withHandlingEventLock(custodyID, func() error {
			close(firstEntered)
			<-release
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- second.withHandlingEventLock(custodyID, func() error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second filesystem handling-event operation entered while first was held")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}
