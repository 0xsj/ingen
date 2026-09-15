package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	amberpostgres "github.com/0xsj/ingen/amber/adapters/storage/postgres"
)

func main() {
	dsn := os.Getenv("AMBER_POSTGRES_DSN")
	if dsn == "" {
		fail("AMBER_POSTGRES_DSN must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fail("open PostgreSQL database", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		fail("ping PostgreSQL database", err)
	}

	store, err := amberpostgres.NewPostgresStore(db)
	if err != nil {
		fail("create Amber PostgreSQL store", err)
	}
	if err := store.CheckSchema(ctx); err != nil {
		fail("check Amber PostgreSQL schema", err)
	}
	fmt.Printf("PostgreSQL schema ready: %s v%d\n", amberpostgres.PostgresSchemaName, amberpostgres.PostgresSchemaVersion)
}

func fail(message string, causes ...error) {
	if len(causes) == 0 {
		fmt.Fprintln(os.Stderr, message)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, causes[0])
	}
	os.Exit(1)
}
