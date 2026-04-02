package imap

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteReadsLatestMatchingMessage(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	errCh := make(chan error, 1)

	go serveIMAP(serverConn, []string{
		`* OK IMAP4rev1 Service Ready`,
		`A001 OK LOGIN completed`,
		`* 2 EXISTS`,
		`A002 OK SELECT completed`,
		`* SEARCH 1 2`,
		`A003 OK SEARCH completed`,
	}, []imapFetch{
		{
			Sequence: "2",
			Body:     mockEmail("noreply@example.com", "user@example.com", "Verify your email", "Your code is 123456"),
		},
	}, errCh)

	block := New()
	block.dial = func(context.Context, imapConfig) (net.Conn, error) { return clientConn, nil }
	result, err := block.Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"host":     "imap.example.com",
			"port":     993,
			"username": "demo",
			"password": "secret",
			"mailbox":  "INBOX",
			"criteria": map[string]any{
				"message_id":       "<demo@example.com>",
				"to":               "user@example.com",
				"subject_contains": "Verify",
				"body_contains":    "123456",
				"unseen_only":      false,
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := result.Output.(map[string]any)
	message := output["message"].(map[string]any)
	if got := message["subject"]; got != "Verify your email" {
		t.Fatalf("subject = %v", got)
	}
	if result.Request["mailbox"] != "INBOX" {
		t.Fatalf("request.mailbox = %#v, want INBOX", result.Request["mailbox"])
	}
	criteria := result.Request["criteria"].(map[string]any)
	if criteria["message_id"] != "<demo@example.com>" {
		t.Fatalf("request.criteria.message_id = %#v", criteria["message_id"])
	}
	if got := result.Exports["text"]; got != "Your code is 123456" {
		t.Fatalf("text export = %q", got)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveIMAP() error = %v", err)
		}
	default:
	}
}

func TestExecuteReturnsRequestWhenMessageNotFound(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	errCh := make(chan error, 1)

	go serveIMAP(serverConn, []string{
		`* OK IMAP4rev1 Service Ready`,
		`A001 OK LOGIN completed`,
		`* 0 EXISTS`,
		`A002 OK SELECT completed`,
		`* SEARCH`,
		`A003 OK SEARCH completed`,
	}, nil, errCh)

	block := New()
	block.dial = func(context.Context, imapConfig) (net.Conn, error) { return clientConn, nil }
	nowCalls := 0
	block.now = func() time.Time {
		nowCalls++
		return time.Unix(int64(nowCalls), 0)
	}
	result, err := block.Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"host":     "imap.example.com",
			"port":     993,
			"username": "demo",
			"password": "secret",
			"mailbox":  "INBOX",
			"criteria": map[string]any{
				"subject_contains": "Verify",
			},
			"wait": map[string]any{
				"timeout_ms": 1,
			},
		},
	})
	if err == nil || err.Error() != "imap message not found" {
		t.Fatalf("error = %v, want imap message not found", err)
	}
	if result == nil || result.Request == nil {
		t.Fatal("request = nil, want request for logging")
	}
}

type imapFetch struct {
	Sequence string
	Body     string
}

func serveIMAP(conn net.Conn, setupLines []string, fetches []imapFetch, errCh chan<- error) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	if err := writeLine(conn, setupLines[0]); err != nil {
		errCh <- err
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			errCh <- nil
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.Contains(line, " LOGIN "):
			err = writeLine(conn, setupLines[1])
		case strings.Contains(line, " SELECT "):
			err = writeLine(conn, setupLines[2])
			if err == nil {
				err = writeLine(conn, setupLines[3])
			}
		case strings.Contains(line, " SEARCH "):
			err = writeLine(conn, setupLines[4])
			if err == nil {
				err = writeLine(conn, setupLines[5])
			}
		case strings.Contains(line, " FETCH "):
			fields := strings.Fields(line)
			tag := fields[0]
			seq := fields[2]
			for _, fetch := range fetches {
				if fetch.Sequence == seq {
					err = writeLiteral(conn, seq, fetch.Body)
					break
				}
			}
			if err == nil {
				err = writeLine(conn, tag+" OK FETCH completed")
			}
		case strings.Contains(line, " STORE "):
			tag := strings.Fields(line)[0]
			err = writeLine(conn, tag+" OK STORE completed")
		case strings.Contains(line, " EXPUNGE"):
			tag := strings.Fields(line)[0]
			err = writeLine(conn, tag+" OK EXPUNGE completed")
		case strings.Contains(line, " LOGOUT"):
			tag := strings.Fields(line)[0]
			err = writeLine(conn, "* BYE logging out")
			if err == nil {
				err = writeLine(conn, tag+" OK LOGOUT completed")
			}
			errCh <- err
			return
		default:
			errCh <- fmt.Errorf("unexpected IMAP command: %q", line)
			return
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}

func writeLine(conn net.Conn, line string) error {
	if _, err := fmt.Fprintf(conn, "%s\r\n", line); err != nil {
		return fmt.Errorf("writeLine(): %w", err)
	}
	return nil
}

func writeLiteral(conn net.Conn, sequence, body string) error {
	if _, err := fmt.Fprintf(
		conn,
		"* %s FETCH (RFC822 {%d}\r\n%s\r\n)\r\n",
		sequence,
		len(body),
		body,
	); err != nil {
		return fmt.Errorf("writeLiteral(): %w", err)
	}
	return nil
}

func mockEmail(from, to, subject, text string) string {
	return fmt.Sprintf(
		"%s\r\n%s\r\n%s\r\n%s\r\n%s\r\n\r\n%s",
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"Message-Id: <demo@example.com>",
		"Content-Type: text/plain; charset=UTF-8",
		text,
	)
}
