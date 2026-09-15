package amberstorage

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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
	runStoreContract(t, func(*testing.T) Store { return store })
}
