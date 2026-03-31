package postgres

import "github.com/wiregoblin/wiregoblin/internal/model"

// Example YAML config:
//
// dsn: @PostgresDSN
// query: select id, status from jobs where id = $1
// params:
//   - $JobID

type postgresConfig struct {
	DSN         string
	Query       string
	Params      []any
	Transaction []postgresStatement
}

type postgresStatement struct {
	Query  string
	Params []any
	Assign any
}

func decodeConfig(step model.Step) postgresConfig {
	config := postgresConfig{}
	if value, ok := step.Config["dsn"].(string); ok {
		config.DSN = value
	}
	if value, ok := step.Config["query"].(string); ok {
		config.Query = value
	}
	if value, ok := step.Config["params"].([]any); ok {
		config.Params = append(config.Params, value...)
	}
	if value, ok := step.Config["transaction"].([]any); ok {
		config.Transaction = decodeTransaction(value)
	}
	return config
}

func decodeTransaction(raw []any) []postgresStatement {
	statements := make([]postgresStatement, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		statement := postgresStatement{}
		if value, ok := entry["query"].(string); ok {
			statement.Query = value
		}
		if value, ok := entry["params"].([]any); ok {
			statement.Params = append(statement.Params, value...)
		}
		if value, ok := entry["assign"]; ok {
			statement.Assign = value
		}
		statements = append(statements, statement)
	}
	if len(statements) == 0 {
		return nil
	}
	return statements
}
