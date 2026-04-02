package smtp //nolint:revive // package name matches the block domain intentionally.

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

type fakeSMTPClient struct {
	mailFrom   string
	recipients []string
	data       bytes.Buffer
}

func (f *fakeSMTPClient) Hello(string) error              { return nil }
func (f *fakeSMTPClient) Extension(string) (bool, string) { return true, "" }
func (f *fakeSMTPClient) StartTLS(*tls.Config) error      { return nil }
func (f *fakeSMTPClient) Auth(smtp.Auth) error            { return nil }
func (f *fakeSMTPClient) Mail(from string) error          { f.mailFrom = from; return nil }
func (f *fakeSMTPClient) Rcpt(to string) error            { f.recipients = append(f.recipients, to); return nil }
func (f *fakeSMTPClient) Data() (io.WriteCloser, error)   { return nopWriteCloser{Writer: &f.data}, nil }
func (f *fakeSMTPClient) Quit() error                     { return nil }
func (f *fakeSMTPClient) Close() error                    { return nil }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestExecuteSendsMessage(t *testing.T) {
	client := &fakeSMTPClient{}
	block := New()
	block.newClient = func(context.Context, smtpConfig) (smtpClient, error) { return client, nil }
	block.now = func() time.Time { return time.Unix(100, 0) }

	result, err := block.Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"host":    "smtp.example.com",
			"port":    587,
			"from":    "noreply@example.com",
			"to":      []any{"user@example.com"},
			"subject": "Verify your email",
			"text":    "Your code is 123456",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.mailFrom != "noreply@example.com" {
		t.Fatalf("mail from = %q", client.mailFrom)
	}
	if len(client.recipients) != 1 || client.recipients[0] != "user@example.com" {
		t.Fatalf("recipients = %#v", client.recipients)
	}
	got := client.data.String()
	if !strings.Contains(got, "Subject: Verify your email") ||
		!strings.Contains(got, "Your code is 123456") {
		t.Fatalf("message = %q", got)
	}
	output := result.Output.(map[string]any)
	if output["subject"] != "Verify your email" {
		t.Fatalf("subject = %v", output["subject"])
	}
	if result.Request["from"] != "noreply@example.com" {
		t.Fatalf("request.from = %#v, want noreply@example.com", result.Request["from"])
	}
	to := result.Request["to"].([]any)
	if len(to) != 1 || to[0] != "user@example.com" {
		t.Fatalf("request.to = %#v", result.Request["to"])
	}
	if result.Request["text"] != "Your code is 123456" {
		t.Fatalf("request.text = %#v", result.Request["text"])
	}
}

func TestExecuteReturnsRequestOnClientError(t *testing.T) {
	block := New()
	block.newClient = func(context.Context, smtpConfig) (smtpClient, error) {
		return nil, errors.New("dial failed")
	}

	result, err := block.Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"host":    "smtp.example.com",
			"port":    587,
			"from":    "noreply@example.com",
			"to":      []any{"user@example.com"},
			"subject": "Verify your email",
			"text":    "Your code is 123456",
		},
	})
	if err == nil || err.Error() != "dial failed" {
		t.Fatalf("error = %v, want dial failed", err)
	}
	if result == nil || result.Request == nil {
		t.Fatal("request = nil, want request for logging")
	}
}
