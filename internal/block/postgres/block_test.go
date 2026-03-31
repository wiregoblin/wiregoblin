package postgres

import (
	"context"
	"database/sql"
	"reflect"
	"sync/atomic"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
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

	result, err := New().Execute(context.Background(), nil, model.Step{
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
	if got := result.Exports["rowCount"]; got != "1" {
		t.Fatalf("exports[rowCount] = %q, want %q", got, "1")
	}
	rows := output["rows"].([]map[string]any)
	wantRow := map[string]any{"id": int64(1), "name": "alice"}
	if !reflect.DeepEqual(rows[0], wantRow) {
		t.Fatalf("row = %#v, want %#v", rows[0], wantRow)
	}
}

func TestExecutePassesQueryParams(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) { return db, nil }
	defer func() { openSQL = oldOpen }()

	mock.ExpectQuery("select id, name from users where id = \\$1").
		WithArgs("user-42").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("user-42", "alice"))

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"dsn":    "postgres://ignored",
			"query":  "select id, name from users where id = $1",
			"params": []any{"user-42"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	rows := output["rows"].([]map[string]any)
	wantRow := map[string]any{"id": "user-42", "name": "alice"}
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

	mock.ExpectExec("update users set active = \\$1 where id = \\$2").
		WithArgs(true, "user-42").
		WillReturnResult(sqlmock.NewResult(0, 3))

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"dsn":    "postgres://ignored",
			"query":  "update users set active = $1 where id = $2",
			"params": []any{true, "user-42"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["rowsAffected"] != int64(3) {
		t.Fatalf("rowsAffected = %v, want 3", output["rowsAffected"])
	}
	if got := result.Exports["rowsAffected"]; got != "3" {
		t.Fatalf("exports[rowsAffected] = %q, want %q", got, "3")
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

	_, err = New().Execute(context.Background(), nil, model.Step{
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

func TestExecuteTransactionCommitsAndAssignsIntermediateValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) { return db, nil }
	defer func() { openSQL = oldOpen }()

	mock.ExpectBegin()
	mock.ExpectExec("insert into users \\(name\\) values \\(\\$1\\)").
		WithArgs("Alice").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("select count\\(\\*\\) as total from users where name = \\$1").
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectExec("update metrics set total = \\$1").
		WithArgs("1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	runCtx := &block.RunContext{
		Variables:       map[string]string{},
		SecretVariables: map[string]string{},
		Constants:       map[string]string{},
		Secrets:         map[string]string{},
	}
	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Config: map[string]any{
			"dsn": "postgres://ignored",
			"transaction": []any{
				map[string]any{
					"query":  "insert into users (name) values ($1)",
					"params": []any{"Alice"},
				},
				map[string]any{
					"query":  "select count(*) as total from users where name = $1",
					"params": []any{"Alice"},
					"assign": map[string]any{
						"$row_count": "body.rows.0.total",
					},
				},
				map[string]any{
					"query":  "update metrics set total = $1",
					"params": []any{"$row_count"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := runCtx.Variables["row_count"]; got != "1" {
		t.Fatalf("row_count = %q, want 1", got)
	}

	output := result.Output.(map[string]any)
	transaction := output["transaction"].(map[string]any)
	if transaction["committed"] != true || transaction["rolledBack"] != false {
		t.Fatalf("transaction = %#v", transaction)
	}
	results := output["results"].([]map[string]any)
	if len(results) != 3 {
		t.Fatalf("results len = %d, want 3", len(results))
	}
	if got := results[1]["rowCount"]; got != 1 {
		t.Fatalf("second rowCount = %v, want 1", got)
	}
}

func TestExecuteTransactionRollsBackOnStatementError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) { return db, nil }
	defer func() { openSQL = oldOpen }()

	mock.ExpectBegin()
	mock.ExpectExec("insert into users \\(name\\) values \\(\\$1\\)").
		WithArgs("Alice").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("update users set active = \\$1").
		WithArgs(true).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables:       map[string]string{},
		SecretVariables: map[string]string{},
		Constants:       map[string]string{},
		Secrets:         map[string]string{},
	}, model.Step{
		Config: map[string]any{
			"dsn": "postgres://ignored",
			"transaction": []any{
				map[string]any{
					"query":  "insert into users (name) values ($1)",
					"params": []any{"Alice"},
				},
				map[string]any{
					"query":  "update users set active = $1",
					"params": []any{true},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want transaction error")
	}
	wantErr := "execute postgres transaction statement 2: execute postgres query: context deadline exceeded"
	if got := err.Error(); got != wantErr {
		t.Fatalf("error = %q", got)
	}

	output := result.Output.(map[string]any)
	transaction := output["transaction"].(map[string]any)
	if transaction["committed"] != false || transaction["rolledBack"] != true {
		t.Fatalf("transaction = %#v", transaction)
	}
	if transaction["failedIndex"] != 1 {
		t.Fatalf("failedIndex = %v, want 1", transaction["failedIndex"])
	}
}

func TestExecuteReusesOpenedDBByDSN(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	var opens int32
	oldOpen := openSQL
	openSQL = func(string, string) (*sql.DB, error) {
		atomic.AddInt32(&opens, 1)
		return db, nil
	}
	defer func() { openSQL = oldOpen }()

	mock.ExpectQuery("select 1").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1))
	mock.ExpectQuery("select 1").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1))

	blk := New()
	step := model.Step{
		Config: map[string]any{
			"dsn":   "postgres://ignored",
			"query": "select 1",
		},
	}
	for range 2 {
		if _, err := blk.Execute(context.Background(), nil, step); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	}
	if got := atomic.LoadInt32(&opens); got != 1 {
		t.Fatalf("openSQL calls = %d, want 1", got)
	}
}
