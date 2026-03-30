package container

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

const (
	defaultTimeoutSeconds = 300
	defaultWorkdir        = "/workspace"
)

// Example YAML config:
//
// image: alpine:3.20
// command: echo hello
// env:
//   TOKEN: $ApiToken
// workdir: /workspace
// timeout_seconds: 60

type containerConfig struct {
	Image          string
	Command        string
	Env            map[string]string
	Workdir        string
	TimeoutSeconds int
	MountSource    string
	DockerPath     string
}

func decodeConfig(step models.Step) containerConfig {
	if step.Config == nil {
		step.Config = map[string]any{}
	}
	config := containerConfig{
		Env:            map[string]string{},
		Workdir:        defaultWorkdir,
		TimeoutSeconds: defaultTimeoutSeconds,
	}
	if v, ok := step.Config["image"].(string); ok {
		config.Image = v
	}
	if v, ok := step.Config["command"].(string); ok {
		config.Command = v
	}
	switch v := step.Config["env"].(type) {
	case map[string]string:
		config.Env = v
	case map[string]any:
		config.Env = make(map[string]string, len(v))
		for key, value := range v {
			config.Env[key] = fmt.Sprint(value)
		}
	}
	if v, ok := step.Config["workdir"].(string); ok && strings.TrimSpace(v) != "" {
		config.Workdir = v
	}
	switch v := step.Config["timeout_seconds"].(type) {
	case float64:
		config.TimeoutSeconds = int(v)
	case int:
		config.TimeoutSeconds = v
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			config.TimeoutSeconds = parsed
		}
	}
	if v, ok := step.Config["mount_source"].(string); ok {
		config.MountSource = strings.TrimSpace(v)
	}
	if v, ok := step.Config["docker_path"].(string); ok {
		config.DockerPath = strings.TrimSpace(v)
	}
	return config
}
