// Package smtp implements SMTP email delivery workflow steps.
package smtp //nolint:revive // package name matches the block domain intentionally.

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "smtp"

type smtpClient interface {
	Hello(string) error
	Extension(string) (bool, string)
	StartTLS(*tls.Config) error
	Auth(smtp.Auth) error
	Mail(string) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type clientFactory func(ctx context.Context, config smtpConfig) (smtpClient, error)

// Block sends SMTP email.
type Block struct {
	newClient clientFactory
	now       func() time.Time
}

// New creates an SMTP workflow block.
func New() *Block {
	return &Block{
		newClient: dialSMTPClient,
		now:       time.Now,
	}
}

// Type returns the SMTP block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether SMTP output can be assigned into runtime variables.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which SMTP fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "host", Constants: true, Variables: true, InlineOnly: true},
		{Field: "port", Constants: true, Variables: true, InlineOnly: true},
		{Field: "username", Constants: true, Variables: true, InlineOnly: true},
		{Field: "password", Constants: true, Variables: true, InlineOnly: true},
		{Field: "from", Constants: true, Variables: true, InlineOnly: true},
		{Field: "to", Constants: true, Variables: true, InlineOnly: true},
		{Field: "cc", Constants: true, Variables: true, InlineOnly: true},
		{Field: "bcc", Constants: true, Variables: true, InlineOnly: true},
		{Field: "subject", Constants: true, Variables: true, InlineOnly: true},
		{Field: "text", Constants: true, Variables: true, InlineOnly: true},
		{Field: "html", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the SMTP configuration.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.host == "" {
		return fmt.Errorf("smtp host is required")
	}
	if config.port <= 0 {
		return fmt.Errorf("smtp port must be greater than 0")
	}
	if config.from == "" {
		return fmt.Errorf("smtp from is required")
	}
	if len(config.to) == 0 && len(config.cc) == 0 && len(config.bcc) == 0 {
		return fmt.Errorf("smtp at least one recipient is required")
	}
	if config.text == "" && config.html == "" {
		return fmt.Errorf("smtp text or html body is required")
	}
	if config.tls && config.startTLS {
		return fmt.Errorf("smtp tls and starttls are mutually exclusive")
	}
	if config.timeoutSeconds < 0 {
		return fmt.Errorf("smtp timeout_seconds must be non-negative")
	}
	return nil
}

// Execute sends one SMTP message and returns delivery metadata.
func (b *Block) Execute(ctx context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
	config := decodeConfig(step)
	request := smtpRequest(config)
	client, err := b.newClient(ctx, config)
	if err != nil {
		return &block.Result{Request: request}, err
	}
	defer func() { _ = client.Close() }()

	messageID := generateMessageID(b.now(), config.from)
	message, err := buildMessage(config, messageID)
	if err != nil {
		return &block.Result{Request: request}, err
	}

	if err := client.Mail(config.from); err != nil {
		return &block.Result{Request: request}, fmt.Errorf("smtp mail from: %w", err)
	}
	recipients := recipients(config)
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return &block.Result{Request: request}, fmt.Errorf("smtp rcpt %s: %w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return &block.Result{Request: request}, fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return &block.Result{Request: request}, fmt.Errorf("smtp write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return &block.Result{Request: request}, fmt.Errorf("smtp finalize message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return &block.Result{Request: request}, fmt.Errorf("smtp quit: %w", err)
	}

	output := map[string]any{
		"from":    config.from,
		"to":      recipients,
		"subject": config.subject,
	}
	return &block.Result{
		Output: output,
		Exports: map[string]string{
			"message_id": messageID,
			"subject":    config.subject,
		},
		Request: request,
	}, nil
}

func smtpRequest(config smtpConfig) map[string]any {
	request := map[string]any{
		"host":     config.host,
		"port":     config.port,
		"tls":      config.tls,
		"starttls": config.startTLS,
		"from":     config.from,
		"to":       cloneStringSlice(config.to),
		"cc":       cloneStringSlice(config.cc),
		"bcc":      cloneStringSlice(config.bcc),
		"subject":  config.subject,
	}
	if config.username != "" {
		request["username"] = config.username
	}
	if config.password != "" {
		request["password"] = config.password
	}
	if config.text != "" {
		request["text"] = config.text
	}
	if config.html != "" {
		request["html"] = config.html
	}
	return request
}

func cloneStringSlice(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func dialSMTPClient(ctx context.Context, config smtpConfig) (smtpClient, error) {
	address := net.JoinHostPort(config.host, fmt.Sprintf("%d", config.port))
	dialer := &net.Dialer{}
	if config.timeoutSeconds > 0 {
		dialer.Timeout = time.Duration(config.timeoutSeconds) * time.Second
	}

	var conn net.Conn
	var err error
	if config.tls {
		tlsConfig := &tls.Config{
			ServerName: config.host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("smtp dial tls: %w", err)
		}
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("smtp dial: %w", err)
		}
	}

	client, err := smtp.NewClient(conn, config.host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp create client: %w", err)
	}

	localName, _ := os.Hostname()
	if localName == "" {
		localName = "localhost"
	}
	if err := client.Hello(localName); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("smtp hello: %w", err)
	}

	if config.startTLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			_ = client.Close()
			return nil, fmt.Errorf("smtp server does not support STARTTLS")
		}
		tlsConfig := &tls.Config{
			ServerName: config.host,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if config.username != "" || config.password != "" {
		ok, _ := client.Extension("AUTH")
		if !ok {
			_ = client.Close()
			return nil, fmt.Errorf("smtp server does not support AUTH")
		}
		auth := smtp.PlainAuth("", config.username, config.password, config.host)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp auth: %w", err)
		}
	}

	return client, nil
}

func buildMessage(config smtpConfig, messageID string) ([]byte, error) {
	var buffer bytes.Buffer
	writeHeader(&buffer, "From", config.from)
	if len(config.to) != 0 {
		writeHeader(&buffer, "To", strings.Join(config.to, ", "))
	}
	if len(config.cc) != 0 {
		writeHeader(&buffer, "Cc", strings.Join(config.cc, ", "))
	}
	writeHeader(&buffer, "Subject", config.subject)
	writeHeader(&buffer, "Message-Id", messageID)
	writeHeader(&buffer, "MIME-Version", "1.0")

	switch {
	case config.text != "" && config.html != "":
		writer := multipart.NewWriter(&buffer)
		writeHeader(&buffer, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", writer.Boundary()))
		buffer.WriteString("\r\n")
		textPart, err := writer.CreatePart(textprotoMIMEHeader("text/plain; charset=UTF-8"))
		if err != nil {
			return nil, fmt.Errorf("smtp create text part: %w", err)
		}
		if _, err := textPart.Write([]byte(config.text)); err != nil {
			return nil, fmt.Errorf("smtp write text part: %w", err)
		}
		htmlPart, err := writer.CreatePart(textprotoMIMEHeader("text/html; charset=UTF-8"))
		if err != nil {
			return nil, fmt.Errorf("smtp create html part: %w", err)
		}
		if _, err := htmlPart.Write([]byte(config.html)); err != nil {
			return nil, fmt.Errorf("smtp write html part: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("smtp finalize multipart body: %w", err)
		}
	default:
		contentType := "text/plain; charset=UTF-8"
		body := config.text
		if config.html != "" {
			contentType = "text/html; charset=UTF-8"
			body = config.html
		}
		writeHeader(&buffer, "Content-Type", contentType)
		buffer.WriteString("\r\n")
		buffer.WriteString(body)
	}

	return buffer.Bytes(), nil
}

func writeHeader(buffer *bytes.Buffer, key, value string) {
	buffer.WriteString(key)
	buffer.WriteString(": ")
	buffer.WriteString(value)
	buffer.WriteString("\r\n")
}

func recipients(config smtpConfig) []string {
	out := make([]string, 0, len(config.to)+len(config.cc)+len(config.bcc))
	out = append(out, config.to...)
	out = append(out, config.cc...)
	out = append(out, config.bcc...)
	return out
}

func textprotoMIMEHeader(contentType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", contentType)
	return header
}

func generateMessageID(now time.Time, from string) string {
	domain := "localhost"
	if parts := strings.Split(strings.TrimSpace(from), "@"); len(parts) == 2 && parts[1] != "" {
		domain = parts[1]
	}
	return fmt.Sprintf("<%d.%d@%s>", now.UnixNano(), os.Getpid(), domain)
}
