// Package postgres implements PostgreSQL-backed workflow steps.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	// Register the pgx database/sql driver for PostgreSQL block execution.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "postgres"

// Block executes SQL queries against PostgreSQL.
type Block struct {
	mu  sync.Mutex
	dbs map[string]*sql.DB
}

var openSQL = sql.Open

// db returns a cached *sql.DB for the given DSN, opening one if needed.
func (b *Block) db(dsn string) (*sql.DB, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if db, ok := b.dbs[dsn]; ok {
		return db, nil
	}
	db, err := openSQL("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}
	if b.dbs == nil {
		b.dbs = make(map[string]*sql.DB)
	}
	b.dbs[dsn] = db
	return db, nil
}

// Close closes all cached database connections.
func (b *Block) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, db := range b.dbs {
		_ = db.Close()
	}
	b.dbs = nil
}

type sqlExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// New creates a PostgreSQL workflow block.
func New() *Block {
	return &Block{
		dbs: make(map[string]*sql.DB),
	}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept @ references and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "dsn", Constants: true},
		{Field: "query", Constants: true, Variables: true, InlineOnly: true},
		{Field: "params", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal PostgreSQL fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.DSN == "" {
		return fmt.Errorf("postgres dsn is required")
	}
	if config.Query == "" && len(config.Transaction) == 0 {
		return fmt.Errorf("postgres query or transaction is required")
	}
	if config.Query != "" && len(config.Transaction) != 0 {
		return fmt.Errorf("postgres query and transaction are mutually exclusive")
	}
	for index, statement := range config.Transaction {
		if strings.TrimSpace(statement.Query) == "" {
			return fmt.Errorf("postgres transaction statement %d query is required", index+1)
		}
	}
	return nil
}

// Execute runs the configured SQL query and returns a JSON-friendly result.
func (b *Block) Execute(
	ctx context.Context,
	runCtx *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	if len(config.Transaction) != 0 {
		output, exports, err := b.executeTransaction(ctx, runCtx, config)
		if err != nil {
			return &block.Result{Output: output, Exports: exports}, err
		}
		return &block.Result{Output: output, Exports: exports}, nil
	}
	output, exports, err := b.executeQuery(ctx, config)
	if err != nil {
		return nil, err
	}
	return &block.Result{Output: output, Exports: exports}, nil
}

func (b *Block) executeQuery(ctx context.Context, config postgresConfig) (map[string]any, map[string]string, error) {
	db, err := b.db(config.DSN)
	if err != nil {
		return nil, nil, err
	}

	if isQueryStatement(config.Query) {
		return executeQueryRows(ctx, db, config.Query, config.Params)
	}
	return executeExec(ctx, db, config.Query, config.Params)
}

func (b *Block) executeTransaction(
	ctx context.Context,
	runCtx *block.RunContext,
	config postgresConfig,
) (map[string]any, map[string]string, error) {
	db, err := b.db(config.DSN)
	if err != nil {
		return nil, nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin postgres transaction: %w", err)
	}

	results := make([]map[string]any, 0, len(config.Transaction))
	for index, statement := range config.Transaction {
		resolved := resolveStatement(runCtx, statement)
		output, exports, execErr := executeStatement(ctx, tx, resolved.Query, resolved.Params)
		resultEntry := map[string]any{
			"index": index,
			"query": resolved.Query,
		}
		for key, value := range output {
			resultEntry[key] = value
		}
		if execErr != nil {
			resultEntry["error"] = execErr.Error()
			results = append(results, resultEntry)
			_ = tx.Rollback()
			return map[string]any{
					"transaction": map[string]any{
						"committed":   false,
						"rolledBack":  true,
						"failedIndex": index,
					},
					"results": results,
				},
				nil,
				fmt.Errorf("execute postgres transaction statement %d: %w", index+1, execErr)
		}
		if len(exports) != 0 {
			resultEntry["outputs"] = exports
		}
		results = append(results, resultEntry)
		if err := applyStatementAssignments(runCtx, output, exports, statement.Assign); err != nil {
			_ = tx.Rollback()
			return map[string]any{
					"transaction": map[string]any{
						"committed":   false,
						"rolledBack":  true,
						"failedIndex": index,
					},
					"results": results,
				},
				nil,
				fmt.Errorf("apply postgres transaction statement %d assign: %w", index+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return map[string]any{
				"transaction": map[string]any{
					"committed":  false,
					"rolledBack": true,
				},
				"results": results,
			},
			nil,
			fmt.Errorf("commit postgres transaction: %w", err)
	}

	return map[string]any{
		"transaction": map[string]any{
			"committed":  true,
			"rolledBack": false,
		},
		"results": results,
	}, nil, nil
}

func executeQueryRows(
	ctx context.Context,
	db sqlExecutor,
	query string,
	params []any,
) (map[string]any, map[string]string, error) {
	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, nil, fmt.Errorf("execute postgres query: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("read postgres columns: %w", err)
	}

	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}

		if err := rows.Scan(destinations...); err != nil {
			return nil, nil, fmt.Errorf("scan postgres row: %w", err)
		}

		row := make(map[string]any, len(columns))
		for index, column := range columns {
			row[column] = normalizeValue(values[index])
		}
		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate postgres rows: %w", err)
	}

	output := map[string]any{
		"rows":     resultRows,
		"rowCount": len(resultRows),
	}
	exports := map[string]string{
		"rowCount": fmt.Sprintf("%d", len(resultRows)),
	}
	return output, exports, nil
}

func executeExec(
	ctx context.Context,
	db sqlExecutor,
	query string,
	params []any,
) (map[string]any, map[string]string, error) {
	result, err := db.ExecContext(ctx, query, params...)
	if err != nil {
		return nil, nil, fmt.Errorf("execute postgres query: %w", err)
	}

	rowsAffected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		rowsAffected = 0
	}

	output := map[string]any{
		"rowsAffected": rowsAffected,
	}
	exports := map[string]string{
		"rowsAffected": fmt.Sprintf("%d", rowsAffected),
	}
	return output, exports, nil
}

func executeStatement(
	ctx context.Context,
	executor sqlExecutor,
	query string,
	params []any,
) (map[string]any, map[string]string, error) {
	if isQueryStatement(query) {
		return executeQueryRows(ctx, executor, query, params)
	}
	return executeExec(ctx, executor, query, params)
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

func resolveStatement(runCtx *block.RunContext, statement postgresStatement) postgresStatement {
	policy := block.ReferencePolicy{
		Constants:  true,
		Variables:  true,
		InlineOnly: true,
	}
	resolved := postgresStatement{
		Query: block.ResolveReferences(runCtx, statement.Query, policy),
	}
	if len(statement.Params) != 0 {
		resolved.Params = make([]any, len(statement.Params))
		for index, value := range statement.Params {
			resolved.Params[index] = resolveStatementValue(runCtx, value, policy)
		}
	}
	resolved.Assign = statement.Assign
	return resolved
}

func resolveStatementValue(runCtx *block.RunContext, value any, policy block.ReferencePolicy) any {
	switch typed := value.(type) {
	case string:
		return block.ResolveReferences(runCtx, typed, policy)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = resolveStatementValue(runCtx, item, policy)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = resolveStatementValue(runCtx, item, policy)
		}
		return out
	default:
		return value
	}
}

type assignmentEntry struct {
	Key  string
	Path string
}

func decodeAssignments(raw any) []assignmentEntry {
	if raw == nil {
		return nil
	}
	if shorthand, ok := raw.(map[string]any); ok {
		entries := make([]assignmentEntry, 0, len(shorthand))
		for key, value := range shorthand {
			target := strings.TrimSpace(key)
			path := strings.TrimSpace(fmt.Sprint(value))
			if target == "" || path == "" {
				continue
			}
			entries = append(entries, assignmentEntry{Key: target, Path: path})
		}
		return entries
	}
	if items, ok := raw.([]any); ok {
		entries := make([]assignmentEntry, 0, len(items))
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			target := strings.TrimSpace(fmt.Sprint(entry["key"]))
			path := strings.TrimSpace(fmt.Sprint(entry["path"]))
			if target == "" || path == "" {
				continue
			}
			entries = append(entries, assignmentEntry{Key: target, Path: path})
		}
		return entries
	}
	return nil
}

func applyStatementAssignments(
	runCtx *block.RunContext,
	output map[string]any,
	exports map[string]string,
	raw any,
) error {
	entries := decodeAssignments(raw)
	if runCtx == nil || len(entries) == 0 {
		return nil
	}
	values := make(map[string]string, len(entries))
	result := &block.Result{Output: output, Exports: exports}
	for _, entry := range entries {
		value, ok := readAssignedValue(result, entry.Path)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			values[entry.Key] = typed
		default:
			body, err := json.Marshal(typed)
			if err != nil {
				continue
			}
			values[entry.Key] = string(body)
		}
	}
	return applyRuntimeAssignments(runCtx, values)
}

func readAssignedValue(result *block.Result, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "outputs.") {
		key := strings.TrimPrefix(path, "outputs.")
		value, ok := result.Exports[key]
		return value, ok
	}
	if strings.HasPrefix(path, "body.") {
		return readBodyValue(result, strings.TrimPrefix(path, "body."))
	}
	path = strings.TrimPrefix(path, "response.")
	if path == "" {
		return nil, false
	}
	if strings.HasPrefix(path, "output.") {
		return readMappedValue(result.Output, strings.TrimPrefix(path, "output."))
	}
	return readMappedValue(result.Output, path)
}

func readBodyValue(result *block.Result, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return result.Output, result.Output != nil
	}
	if outputMap, ok := result.Output.(map[string]any); ok {
		if body, found := outputMap["body"]; found {
			return readMappedValue(body, path)
		}
	}
	return readMappedValue(result.Output, path)
}

func readMappedValue(obj any, path string) (any, bool) {
	current := obj
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		switch typed := current.(type) {
		case map[string]any:
			resolved, found := resolveKey(typed, part)
			if !found {
				return nil, false
			}
			current = typed[resolved]
		default:
			index, err := strconv.Atoi(part)
			if err != nil {
				return nil, false
			}
			value := reflect.ValueOf(current)
			if !value.IsValid() || (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) {
				return nil, false
			}
			if index < 0 || index >= value.Len() {
				return nil, false
			}
			current = value.Index(index).Interface()
		}
	}
	return current, true
}

func resolveKey(m map[string]any, key string) (string, bool) {
	if _, ok := m[key]; ok {
		return key, true
	}
	lower := strings.ToLower(key)
	for current := range m {
		if strings.ToLower(current) == lower {
			return current, true
		}
	}
	return "", false
}

func applyRuntimeAssignments(runCtx *block.RunContext, assignments map[string]string) error {
	if runCtx == nil || len(assignments) == 0 {
		return nil
	}
	for target := range assignments {
		if err := validateRuntimeTarget(runCtx, target); err != nil {
			return err
		}
	}
	for target, value := range assignments {
		name := parseRuntimeTarget(target)
		if _, ok := runCtx.SecretVariables[name]; ok {
			runCtx.SecretVariables[name] = value
			continue
		}
		runCtx.Variables[name] = value
	}
	return nil
}

func validateRuntimeTarget(runCtx *block.RunContext, target string) error {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return fmt.Errorf("runtime target is required")
	}
	if strings.HasPrefix(trimmed, "@") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
		return fmt.Errorf("runtime target must use $%s, not @%s", name, name)
	}
	if !strings.HasPrefix(trimmed, "$") {
		return fmt.Errorf("runtime target must start with $")
	}
	name := parseRuntimeTarget(trimmed)
	if _, ok := runCtx.Constants[name]; ok {
		return fmt.Errorf("cannot write to read-only constant @%s", name)
	}
	if _, ok := runCtx.Secrets[name]; ok {
		return fmt.Errorf("cannot write to read-only secret @%s", name)
	}
	return nil
}

func parseRuntimeTarget(target string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(target), "$"))
}
