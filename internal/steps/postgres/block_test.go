package postgres

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestExecuteReturnsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) { return db, nil }
	defer func() { openSQL = oldOpen }()

	mock.ExpectQuery("select id, name from users").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "alice"))

	result, err := New().Execute(context.Background(), nil, models.Step{
		Config: map[string]any{
			"dsn":   "postgres://ignored",
			"query": "select id, name from users",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["rowCount"] != 1 {
		t.Fatalf("rowCount = %v, want 1", output["rowCount"])
	}
	rows := output["rows"].([]map[string]any)
	wantRow := map[string]any{"id": int64(1), "name": "alice"}
	if !reflect.DeepEqual(rows[0], wantRow) {
		t.Fatalf("row = %#v, want %#v", rows[0], wantRow)
	}
}

func TestExecuteUsesExecForUpdateStatements(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) { return db, nil }
	defer func() { openSQL = oldOpen }()

	mock.ExpectExec("update users set active = true").
		WillReturnResult(sqlmock.NewResult(0, 3))

	result, err := New().Execute(context.Background(), nil, models.Step{
		Config: map[string]any{
			"dsn":   "postgres://ignored",
			"query": "update users set active = true",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["rowsAffected"] != int64(3) {
		t.Fatalf("rowsAffected = %v, want 3", output["rowsAffected"])
	}
}

func TestExecuteDoesNotFallbackToExecOnQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) { return db, nil }
	defer func() { openSQL = oldOpen }()

	mock.ExpectQuery("select id from users").
		WillReturnError(context.DeadlineExceeded)

	_, err = New().Execute(context.Background(), nil, models.Step{
		Config: map[string]any{
			"dsn":   "postgres://ignored",
			"query": "select id from users",
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want query error")
	}
	if err.Error() != "execute postgres query: context deadline exceeded" {
		t.Fatalf("error = %q", err.Error())
	}
}
