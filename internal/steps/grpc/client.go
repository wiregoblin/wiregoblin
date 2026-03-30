package grpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jhump/protoreflect/v2/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Client manages the active gRPC connection and dynamic unary invocation.
type Client struct {
	mu   sync.RWMutex
	conn *grpc.ClientConn
}

// NewClient creates a minimal gRPC client wrapper for unary dynamic calls.
func NewClient() *Client {
	return &Client{}
}

// Connect establishes a gRPC client connection to the target server.
func (c *Client) Connect(ctx context.Context, address string, useTLS bool) error {
	transportCredentials := credentials.TransportCredentials(insecure.NewCredentials())
	if useTLS {
		transportCredentials = credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	}

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(transportCredentials),
	)
	if err != nil {
		return fmt.Errorf("create gRPC client: %w", err)
	}

	conn.Connect()

	if ctx == nil {
		ctx = context.Background()
	}

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(ctx, state) {
			_ = conn.Close()
			return fmt.Errorf("connect to %s: %w", address, ctx.Err())
		}
		if conn.GetState() == connectivity.Shutdown {
			_ = conn.Close()
			return fmt.Errorf("connect to %s: connection shutdown", address)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = conn

	return nil
}

// Conn returns the active gRPC connection.
func (c *Client) Conn() (*grpc.ClientConn, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	return c.conn, nil
}

// InvokeUnary invokes a unary RPC and returns the response as formatted JSON.
func (c *Client) InvokeUnary(
	ctx context.Context,
	method protoreflect.MethodDescriptor,
	request proto.Message,
	headers map[string]string,
) (string, error) {
	conn, err := c.Conn()
	if err != nil {
		return "", err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if len(headers) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(headers))
	}

	stub := grpcdynamic.NewStub(conn)
	response, err := stub.InvokeRpc(ctx, method, request)
	if err != nil {
		return "", fmt.Errorf("invoke %s: %w", method.FullName(), err)
	}

	marshaler := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}

	body, err := marshaler.Marshal(response)
	if err != nil {
		fallback, fallbackErr := json.MarshalIndent(map[string]string{
			"error": err.Error(),
		}, "", "  ")
		if fallbackErr != nil {
			return "", err
		}
		return string(fallback), nil
	}

	return string(body), nil
}

// Close closes the active gRPC connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}
