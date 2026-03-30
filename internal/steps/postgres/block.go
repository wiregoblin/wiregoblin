// Package postgres implements PostgreSQL-backed workflow steps.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	// Register the pgx database/sql driver for PostgreSQL block execution.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block executes SQL queries against PostgreSQL.
type Block struct{}

var openSQL = sql.Open

// New creates a PostgreSQL workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypePostgres
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "dsn", Constants: true},
		{Field: "query", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal PostgreSQL fields.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if config.DSN == "" {
		return fmt.Errorf("postgres dsn is required")
	}
	if config.Query == "" {
		return fmt.Errorf("postgres query is required")
	}
	return nil
}

// Execute runs the configured SQL query and returns a JSON-friendly result.
func (b *Block) Execute(
	ctx context.Context,
	_ *block.RunContext,
	step models.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	output, err := executeQuery(ctx, config)
	if err != nil {
		return nil, err
	}
	return &block.Result{Output: output}, nil
}

func executeQuery(ctx context.Context, config postgresConfig) (map[string]any, error) {
	db, err := openSQL("pgx", config.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if isQueryStatement(config.Query) {
		return executeQueryRows(ctx, db, config.Query)
	}
	return executeExec(ctx, db, config.Query)
}

func executeQueryRows(ctx context.Context, db *sql.DB, query string) (map[string]any, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("execute postgres query: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read postgres columns: %w", err)
	}

	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}

		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan postgres row: %w", err)
		}

		row := make(map[string]any, len(columns))
		for index, column := range columns {
			row[column] = normalizeValue(values[index])
		}
		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres rows: %w", err)
	}

	return map[string]any{
		"rows":     resultRows,
		"rowCount": len(resultRows),
	}, nil
}

func executeExec(ctx context.Context, db *sql.DB, query string) (map[string]any, error) {
	result, err := db.ExecContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("execute postgres query: %w", err)
	}

	rowsAffected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		rowsAffected = 0
	}

	return map[string]any{
		"rowsAffected": rowsAffected,
	}, nil
}

func isQueryStatement(query string) bool {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return false
	}

	switch strings.ToUpper(fields[0]) {
	case "SELECT", "WITH", "EXPLAIN", "SHOW":
		return true
	default:
		return false
	}
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return typed
	}
}
