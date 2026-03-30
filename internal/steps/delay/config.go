package delay

import "github.com/wiregoblin/wiregoblin/internal/models"

// Example YAML config:
//
// milliseconds: 1500

type config struct {
	Milliseconds int
}

func decodeConfig(step models.Step) config {
	c := config{Milliseconds: 1000}
	switch v := step.Config["milliseconds"].(type) {
	case float64:
		c.Milliseconds = int(v)
	case int:
		c.Milliseconds = v
	}
	return c
}
