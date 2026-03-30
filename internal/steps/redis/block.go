// Package redis implements Redis-backed workflow steps.
package redis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block executes a single Redis command.
type Block struct{}

// New creates a Redis workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypeRedis
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "address", Constants: true},
		{Field: "password", Constants: true},
		{Field: "args", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal Redis fields.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if strings.TrimSpace(config.Address) == "" {
		return fmt.Errorf("redis address is required")
	}
	if strings.TrimSpace(config.Command) == "" {
		return fmt.Errorf("redis command is required")
	}
	if config.TimeoutSeconds <= 0 {
		return fmt.Errorf("redis timeout seconds must be greater than zero")
	}
	return nil
}

// Execute runs one Redis command and returns the parsed reply.
func (b *Block) Execute(ctx context.Context, _ *block.RunContext, step models.Step) (*block.Result, error) {
	return execute(ctx, decodeConfig(step))
}

func execute(ctx context.Context, config redisConfig) (*block.Result, error) {
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", config.Address)
	if err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)

	reader := bufio.NewReader(conn)
	if strings.TrimSpace(config.Password) != "" {
		if err := writeRESPCommand(conn, []string{"AUTH", config.Password}); err != nil {
			return nil, fmt.Errorf("auth redis: %w", err)
		}
		if _, err := readRESPValue(reader); err != nil {
			return nil, fmt.Errorf("auth redis: %w", err)
		}
	}
	if config.DB > 0 {
		if err := writeRESPCommand(conn, []string{"SELECT", strconv.Itoa(config.DB)}); err != nil {
			return nil, fmt.Errorf("select redis db: %w", err)
		}
		if _, err := readRESPValue(reader); err != nil {
			return nil, fmt.Errorf("select redis db: %w", err)
		}
	}

	parts := append([]string{strings.ToUpper(strings.TrimSpace(config.Command))}, config.Args...)
	startedAt := time.Now()
	if err := writeRESPCommand(conn, parts); err != nil {
		return nil, fmt.Errorf("write redis command: %w", err)
	}
	value, err := readRESPValue(reader)
	durationMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("execute redis command: %w", err)
	}

	output := map[string]any{
		"address":    config.Address,
		"db":         config.DB,
		"command":    parts[0],
		"args":       config.Args,
		"result":     value,
		"durationMs": durationMs,
	}

	exports := map[string]string{
		"command":    parts[0],
		"durationMs": strconv.FormatInt(durationMs, 10),
	}
	if scalar, ok := toScalarString(value); ok {
		exports["result"] = scalar
	}

	return &block.Result{Output: output, Exports: exports}, nil
}

func writeRESPCommand(conn net.Conn, parts []string) error {
	if _, err := fmt.Fprintf(conn, "*%d\r\n", len(parts)); err != nil {
		return err
	}
	for _, part := range parts {
		if _, err := fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(part), part); err != nil {
			return err
		}
	}
	return nil
}

func readRESPValue(reader *bufio.Reader) (any, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")

	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, errors.New(line)
	case ':':
		value, parseErr := strconv.ParseInt(line, 10, 64)
		if parseErr != nil {
			return nil, parseErr
		}
		return value, nil
	case '$':
		size, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			return nil, parseErr
		}
		if size < 0 {
			return nil, nil
		}
		buffer := make([]byte, size+2)
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return nil, err
		}
		return string(buffer[:size]), nil
	case '*':
		size, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			return nil, parseErr
		}
		if size < 0 {
			return nil, nil
		}
		values := make([]any, 0, size)
		for i := 0; i < size; i++ {
			value, err := readRESPValue(reader)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported redis reply prefix: %q", prefix)
	}
}

func toScalarString(value any) (string, bool) {
	switch current := value.(type) {
	case nil:
		return "", false
	case string:
		return current, true
	case int64:
		return strconv.FormatInt(current, 10), true
	case int:
		return strconv.Itoa(current), true
	case bool:
		if current {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}
