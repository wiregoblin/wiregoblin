package postgres

import "github.com/wiregoblin/wiregoblin/internal/models"

// Example YAML config:
//
// dsn: $PostgresDSN
// query: select id, status from jobs where id = @JobID

type postgresConfig struct {
	DSN   string
	Query string
}

func decodeConfig(step models.Step) postgresConfig {
	config := postgresConfig{}
	if value, ok := step.Config["dsn"].(string); ok {
		config.DSN = value
	}
	if value, ok := step.Config["query"].(string); ok {
		config.Query = value
	}
	return config
}
