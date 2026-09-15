package amberstorage

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	amber "github.com/0xsj/ingen/amber"
)

func TestNewPostgresStoreRejectsNilDatabase(t *testing.T) {
	if _, err := NewPostgresStore(nil); !errors.Is(err, amber.ErrInvalidTransition) {
		t.Fatalf("expected invalid database error, got %v", err)
	}
}

func TestPostgresHistoryQueriesUseContractOrdering(t *testing.T) {
	queries := map[string]string{
		"work":        postgresWorkQuery,
		"causation":   postgresCausationQuery,
		"correlation": postgresCorrelationQuery,
	}
	fragments := []string{
		"(value_json->>'depth')::NUMERIC ASC",
		"(value_json->>'attempt')::NUMERIC ASC",
		"execution_id ASC",
	}
	for name, query := range queries {
		for _, fragment := range fragments {
			if !strings.Contains(query, fragment) {
				t.Errorf("%s query is missing deterministic ordering fragment %q", name, fragment)
			}
		}
	}
}

func TestPostgresStoreEnsuresSchemaAndReadsValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	data, err := root.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec(regexp.QuoteMeta(PostgresSchema)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(postgresSchemaVersionQuery)).
		WithArgs(PostgresSchemaName).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow(PostgresSchemaVersion))
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(postgresInsertQuery)).
		WithArgs(root.ExecutionID().String(), root.WorkID().String(), root.CorrelationID().String(), nil, nil, string(data)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := store.Put(context.Background(), root); err != nil {
		t.Fatalf("put root: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(postgresGetQuery)).
		WithArgs(root.ExecutionID().String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow(data))
	got, err := store.Get(context.Background(), root.ExecutionID())
	if err != nil || got.ExecutionID() != root.ExecutionID() {
		t.Fatalf("get root: execution=%s err=%v", got.ExecutionID(), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreRejectsUnsupportedSchemaVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(PostgresSchema)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(postgresSchemaVersionQuery)).
		WithArgs(PostgresSchemaName).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow(PostgresSchemaVersion + 1))
	if err := store.EnsureSchema(context.Background()); !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf("expected unsupported schema version error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreChecksSchemaWithoutCreatingIt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(postgresSchemaVersionQuery)).
		WithArgs(PostgresSchemaName).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow(PostgresSchemaVersion))
	if err := store.CheckSchema(context.Background()); err != nil {
		t.Fatalf("check schema: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreRejectsMissingSchemaMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(postgresSchemaVersionQuery)).
		WithArgs(PostgresSchemaName).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}))
	if err := store.CheckSchema(context.Background()); !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf("expected missing schema metadata error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStorePreservesIdempotencyAndConflicts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	data, err := root.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	conflictingData := replaceJSONField(data, "origin", "incoming")
	conflicting, err := amber.FromJSON(conflictingData)
	if err != nil {
		t.Fatal(err)
	}
	conflictingEncoded, err := conflicting.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec(regexp.QuoteMeta(postgresInsertQuery)).
		WithArgs(root.ExecutionID().String(), root.WorkID().String(), root.CorrelationID().String(), nil, nil, string(data)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := store.Put(context.Background(), root); err != nil {
		t.Fatalf("initial put: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(postgresInsertQuery)).
		WithArgs(root.ExecutionID().String(), root.WorkID().String(), root.CorrelationID().String(), nil, nil, string(data)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(postgresGetQuery)).
		WithArgs(root.ExecutionID().String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow(data))
	if err := store.Put(context.Background(), root); err != nil {
		t.Fatalf("same value should be idempotent: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(postgresInsertQuery)).
		WithArgs(root.ExecutionID().String(), root.WorkID().String(), root.CorrelationID().String(), nil, nil, string(conflictingEncoded)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(postgresGetQuery)).
		WithArgs(root.ExecutionID().String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow(data))
	if err := store.Put(context.Background(), conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreSupportsIndexedHistoryQueries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		t.Fatal(err)
	}
	rootData, err := root.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	childData, err := child.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(postgresWorkQuery)).
		WithArgs(root.WorkID().String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow(rootData))
	workHistory, err := store.ListByWorkID(context.Background(), root.WorkID())
	if err != nil || len(workHistory) != 1 || workHistory[0].ExecutionID() != root.ExecutionID() {
		t.Fatalf("work history=%v err=%v", workHistory, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(postgresCausationQuery)).
		WithArgs("execution", root.ExecutionID().String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow(childData))
	causalHistory, err := store.ListByCausation(context.Background(), "execution", root.ExecutionID())
	if err != nil || len(causalHistory) != 1 || causalHistory[0].ExecutionID() != child.ExecutionID() {
		t.Fatalf("causal history=%v err=%v", causalHistory, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(postgresCorrelationQuery)).
		WithArgs(root.CorrelationID().String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow(rootData).AddRow(childData))
	correlated, err := store.ListByCorrelationID(context.Background(), root.CorrelationID())
	if err != nil || len(correlated) != 2 {
		t.Fatalf("correlation history=%v err=%v", correlated, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreReturnsMissingAndCancelled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	missing := amber.ID("11111111-1111-4111-8111-111111111111")
	mock.ExpectQuery(regexp.QuoteMeta(postgresGetQuery)).
		WithArgs(missing.String()).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}))
	if _, err := store.Get(context.Background(), missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(cancelled, amber.Provenance{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var _ SQLDB = (*sql.DB)(nil)
var _ Store = (*PostgresStore)(nil)
