package redis

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteSuccess(t *testing.T) {
	address := startRedisTestServer(t, func(conn net.Conn, reader *bufio.Reader) {
		mustReadCommand(t, reader, []string{"AUTH", "secret"})
		_, _ = conn.Write([]byte("+OK\r\n"))
		mustReadCommand(t, reader, []string{"SELECT", "2"})
		_, _ = conn.Write([]byte("+OK\r\n"))
		mustReadCommand(t, reader, []string{"GET", "key"})
		_, _ = conn.Write([]byte("$5\r\nvalue\r\n"))
	})

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"address":         address,
			"password":        "secret",
			"db":              2,
			"command":         "get",
			"args":            []any{"key"},
			"timeout_seconds": 1,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Exports["result"] != "value" {
		t.Fatalf("result = %q, want value", result.Exports["result"])
	}

	output := result.Output.(map[string]any)
	if output["command"] != "GET" {
		t.Fatalf("command = %v, want GET", output["command"])
	}
}

func TestExecuteReturnsRedisError(t *testing.T) {
	address := startRedisTestServer(t, func(conn net.Conn, reader *bufio.Reader) {
		mustReadCommand(t, reader, []string{"PING"})
		_, _ = conn.Write([]byte("-ERR failed\r\n"))
	})

	_, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"address":         address,
			"command":         "PING",
			"timeout_seconds": 1,
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want redis error")
	}
	if err.Error() != "execute redis command: ERR failed" {
		t.Fatalf("error = %q", err.Error())
	}
}

func startRedisTestServer(t *testing.T, handler func(conn net.Conn, reader *bufio.Reader)) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		handler(conn, bufio.NewReader(conn))
	}()

	return listener.Addr().String()
}

func mustReadCommand(t *testing.T, reader *bufio.Reader, want []string) {
	t.Helper()

	value, err := readRESPValue(reader)
	if err != nil {
		t.Fatalf("readRESPValue() error = %v", err)
	}
	rawParts, ok := value.([]any)
	if !ok {
		t.Fatalf("value type = %T, want []any", value)
	}
	if len(rawParts) != len(want) {
		t.Fatalf("len(parts) = %d, want %d", len(rawParts), len(want))
	}
	for index, expected := range want {
		got := rawParts[index].(string)
		if got != expected {
			t.Fatalf("part[%d] = %q, want %q", index, got, expected)
		}
	}
}

func TestToScalarStringInt64(t *testing.T) {
	got, ok := toScalarString(int64(7))
	if !ok || got != strconv.FormatInt(7, 10) {
		t.Fatalf("toScalarString(int64(7)) = (%q, %v)", got, ok)
	}
}
