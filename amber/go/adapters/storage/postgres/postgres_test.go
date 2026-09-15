package amberpostgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
	amberpostgres "github.com/0xsj/ingen/amber/adapters/storage/postgres"
)

type schemaChecker interface {
	CheckSchema(context.Context) error
}

var (
	_ amberstorage.Store = (*amberpostgres.PostgresStore)(nil)
	_ schemaChecker     = (*amberpostgres.PostgresStore)(nil)
)

func TestPublicPostgresBoundary(t *testing.T) {
	if amberpostgres.PostgresSchemaName != "amber_provenance" {
		t.Fatalf("schema name = %q, want amber_provenance", amberpostgres.PostgresSchemaName)
	}
	if amberpostgres.PostgresSchemaVersion <= 0 {
		t.Fatalf("schema version = %d, want positive version", amberpostgres.PostgresSchemaVersion)
	}
	for _, fragment := range []string{
		"amber_provenance_schema",
		"schema_version",
		"amber_provenance",
	} {
		if !strings.Contains(amberpostgres.PostgresSchema, fragment) {
			t.Errorf("PostgresSchema is missing %q", fragment)
		}
	}
	if amberpostgres.ErrUnsupportedSchemaVersion != amberstorage.ErrUnsupportedSchemaVersion {
		t.Fatal("optional package did not preserve the storage schema error sentinel")
	}
	if _, err := amberpostgres.NewPostgresStore(nil); !errors.Is(err, amber.ErrInvalidTransition) {
		t.Fatalf("nil database error = %v, want invalid transition", err)
	}
}
