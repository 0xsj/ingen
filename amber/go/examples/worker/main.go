package main

import (
	"context"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

type workerResult struct {
	child   amber.Provenance
	retry   amber.Provenance
	history []amber.Provenance
}

// processJob represents an application-owned background worker. The worker
// derives a new execution for the job and records a retry as another execution
// of that same logical work.
func processJob(ctx context.Context, store amberstorage.Store, parent amber.Provenance) (workerResult, error) {
	child, err := parent.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		return workerResult{}, err
	}
	if err := store.Put(ctx, child); err != nil {
		return workerResult{}, err
	}

	retry, err := child.Retry()
	if err != nil {
		return workerResult{}, err
	}
	if err := store.Put(ctx, retry); err != nil {
		return workerResult{}, err
	}

	history, err := store.ListByWorkID(ctx, child.WorkID())
	if err != nil {
		return workerResult{}, err
	}
	return workerResult{child: child, retry: retry, history: history}, nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	store := amberstorage.NewMemoryStore()
	parent, err := amber.Start()
	if err != nil {
		return err
	}
	result, err := processJob(context.Background(), store, parent)
	if err != nil {
		return err
	}
	if len(result.history) != 2 || result.history[0].ExecutionID() != result.child.ExecutionID() || result.history[1].ExecutionID() != result.retry.ExecutionID() {
		return fmt.Errorf("worker history was not ordered as child then retry")
	}

	fmt.Printf("worker child execution: %s\n", result.child.ExecutionID())
	fmt.Printf("worker retry execution: %s\n", result.retry.ExecutionID())
	fmt.Printf("worker history records: %d\n", len(result.history))
	return nil
}
