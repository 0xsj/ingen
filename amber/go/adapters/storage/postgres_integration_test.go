package amberstorage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	amber "github.com/0xsj/ingen/amber"
)

func TestPostgresStoreLive(t *testing.T) {
	dsn := os.Getenv("AMBER_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AMBER_POSTGRES_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres database: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres database: %v", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin postgres transaction: %v", err)
	}
	defer tx.Rollback()

	store, err := NewPostgresStore(tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure postgres schema: %v", err)
	}
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, root); err != nil {
		t.Fatalf("put root: %v", err)
	}
	if err := store.Put(ctx, child); err != nil {
		t.Fatalf("put child: %v", err)
	}
	if err := store.Put(ctx, root); err != nil {
		t.Fatalf("same value should be idempotent: %v", err)
	}

	stored, err := store.Get(ctx, root.ExecutionID())
	if err != nil || stored.ExecutionID() != root.ExecutionID() {
		t.Fatalf("get root: execution=%s err=%v", stored.ExecutionID(), err)
	}
	workHistory, err := store.ListByWorkID(ctx, root.WorkID())
	if err != nil || len(workHistory) != 1 || workHistory[0].ExecutionID() != root.ExecutionID() {
		t.Fatalf("work history=%v err=%v", workHistory, err)
	}
	causalHistory, err := store.ListByCausation(ctx, "execution", root.ExecutionID())
	if err != nil || len(causalHistory) != 1 || causalHistory[0].ExecutionID() != child.ExecutionID() {
		t.Fatalf("causal history=%v err=%v", causalHistory, err)
	}
	correlated, err := store.ListByCorrelationID(ctx, root.CorrelationID())
	if err != nil || len(correlated) != 2 {
		t.Fatalf("correlation history=%v err=%v", correlated, err)
	}

	conflictingData := replaceJSONField(mustJSON(root), "origin", "incoming")
	conflicting, err := amber.FromJSON(conflictingData)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
