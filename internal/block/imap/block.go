// Package imap implements IMAP mailbox polling workflow steps.
package imap

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "imap"

var literalPattern = regexp.MustCompile(`\{(\d+)\}$`)

type dialFunc func(ctx context.Context, config imapConfig) (net.Conn, error)

// Block reads one email from an IMAP mailbox.
type Block struct {
	dial dialFunc
	now  func() time.Time
}

// New creates an IMAP workflow block.
func New() *Block {
	return &Block{
		dial: dialIMAP,
		now:  time.Now,
	}
}

// Type returns the IMAP block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether IMAP output can be assigned into runtime variables.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which IMAP fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "host", Constants: true, Variables: true, InlineOnly: true},
		{Field: "port", Constants: true, Variables: true, InlineOnly: true},
		{Field: "username", Constants: true, Variables: true, InlineOnly: true},
		{Field: "password", Constants: true, Variables: true, InlineOnly: true},
		{Field: "mailbox", Constants: true, Variables: true, InlineOnly: true},
		{Field: "criteria", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the IMAP configuration.
func (b *Block) Validate(step model.Step) error {
	return decodeConfig(step).validate()
}

// Execute polls the configured mailbox until a matching message is found or the wait deadline is reached.
func (b *Block) Execute(ctx context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
	config := decodeConfig(step)
	conn, err := b.dial(ctx, config)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	client := &imapClient{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}
	if err := client.readGreeting(); err != nil {
		return nil, err
	}
	if err := client.login(config.username, config.password); err != nil {
		return nil, err
	}
	defer func() { _ = client.logout() }()
	if err := client.selectMailbox(config.mailbox); err != nil {
		return nil, err
	}

	deadline := time.Time{}
	if config.wait.TimeoutMS > 0 {
		deadline = b.now().Add(time.Duration(config.wait.TimeoutMS) * time.Millisecond)
	}

	for {
		message, err := client.findMessage(config)
		if err != nil {
			return nil, err
		}
		if message != nil {
			if config.markAsSeen {
				if err := client.storeFlags(message.Sequence, `\Seen`); err != nil {
					return nil, err
				}
			}
			if config.delete {
				if err := client.storeFlags(message.Sequence, `\Deleted`); err != nil {
					return nil, err
				}
				if err := client.expunge(); err != nil {
					return nil, err
				}
			}
			output := map[string]any{
				"mailbox": config.mailbox,
				"matched": true,
				"message": message.toMap(),
			}
			return &block.Result{
				Output: output,
				Exports: map[string]string{
					"message_id": message.MessageID,
					"subject":    message.Subject,
					"text":       message.Text,
					"html":       message.HTML,
				},
			}, nil
		}
		if deadline.IsZero() || !b.now().Before(deadline) {
			return &block.Result{
				Output: map[string]any{
					"mailbox": config.mailbox,
					"matched": false,
				},
			}, fmt.Errorf("imap message not found")
		}
		wait := time.Duration(config.wait.PollIntervalMS) * time.Millisecond
		if wait <= 0 {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func dialIMAP(ctx context.Context, config imapConfig) (net.Conn, error) {
	address := net.JoinHostPort(config.host, fmt.Sprintf("%d", config.port))
	dialer := &net.Dialer{}
	if config.timeoutSeconds > 0 {
		dialer.Timeout = time.Duration(config.timeoutSeconds) * time.Second
	}
	if config.tls {
		tlsConfig := &tls.Config{
			ServerName: config.host,
			MinVersion: tls.VersionTLS12,
		}
		return tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	}
	return dialer.DialContext(ctx, "tcp", address)
}

type imapClient struct {
	conn       net.Conn
	r          *bufio.Reader
	w          *bufio.Writer
	tagCounter int
}

type imapMessage struct {
	Sequence  string
	MessageID string
	Subject   string
	From      []string
	To        []string
	Date      string
	Text      string
	HTML      string
	Headers   map[string]string
}

func (m *imapMessage) toMap() map[string]any {
	return map[string]any{
		"message_id": m.MessageID,
		"subject":    m.Subject,
		"from":       m.From,
		"to":         m.To,
		"date":       m.Date,
		"text":       m.Text,
		"html":       m.HTML,
		"headers":    m.Headers,
	}
}

func (c *imapClient) readGreeting() error {
	line, err := c.readLine()
	if err != nil {
		return fmt.Errorf("imap greeting: %w", err)
	}
	if !strings.HasPrefix(strings.ToUpper(line), "* OK") {
		return fmt.Errorf("imap greeting: unexpected response %q", line)
	}
	return nil
}

func (c *imapClient) login(username, password string) error {
	_, _, err := c.command("LOGIN %q %q", username, password)
	if err != nil {
		return fmt.Errorf("imap login: %w", err)
	}
	return nil
}

func (c *imapClient) selectMailbox(mailbox string) error {
	_, _, err := c.command("SELECT %q", mailbox)
	if err != nil {
		return fmt.Errorf("imap select mailbox: %w", err)
	}
	return nil
}

func (c *imapClient) logout() error {
	_, _, err := c.command("LOGOUT")
	return err
}

func (c *imapClient) expunge() error {
	_, _, err := c.command("EXPUNGE")
	if err != nil {
		return fmt.Errorf("imap expunge: %w", err)
	}
	return nil
}

func (c *imapClient) storeFlags(sequence, flag string) error {
	_, _, err := c.command("STORE %s +FLAGS.SILENT (%s)", sequence, flag)
	if err != nil {
		return fmt.Errorf("imap store flags: %w", err)
	}
	return nil
}

func (c *imapClient) findMessage(config imapConfig) (*imapMessage, error) {
	sequences, err := c.search(config.criteria.UnseenOnly)
	if err != nil {
		return nil, err
	}
	if len(sequences) == 0 {
		return nil, nil
	}
	if config.selectMode == "latest" {
		slices.Reverse(sequences)
	}
	for _, sequence := range sequences {
		message, err := c.fetchMessage(sequence)
		if err != nil {
			return nil, err
		}
		if matchesCriteria(message, config.criteria) {
			return message, nil
		}
	}
	return nil, nil
}

func (c *imapClient) search(unseenOnly bool) ([]string, error) {
	query := "SEARCH ALL"
	if unseenOnly {
		query = "SEARCH UNSEEN"
	}
	lines, _, err := c.command("%s", query)
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.ToUpper(line), "* SEARCH") {
			fields := strings.Fields(line)
			if len(fields) <= 2 {
				return nil, nil
			}
			return fields[2:], nil
		}
	}
	return nil, nil
}

func (c *imapClient) fetchMessage(sequence string) (*imapMessage, error) {
	_, literals, err := c.command("FETCH %s RFC822", sequence)
	if err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	if len(literals) == 0 {
		return nil, fmt.Errorf("imap fetch: no message content returned")
	}
	return parseRFC822(sequence, literals[0])
}

func (c *imapClient) command(format string, args ...any) ([]string, [][]byte, error) {
	c.tagCounter++
	tag := fmt.Sprintf("A%03d", c.tagCounter)
	if _, err := fmt.Fprintf(c.w, "%s %s\r\n", tag, fmt.Sprintf(format, args...)); err != nil {
		return nil, nil, err
	}
	if err := c.w.Flush(); err != nil {
		return nil, nil, err
	}
	lines := []string{}
	literals := [][]byte{}
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, nil, err
		}
		lines = append(lines, line)
		if size, ok := parseLiteralSize(line); ok {
			literal := make([]byte, size)
			if _, err := io.ReadFull(c.r, literal); err != nil {
				return nil, nil, err
			}
			literals = append(literals, literal)
			if _, err := c.readLine(); err != nil {
				return nil, nil, err
			}
			continue
		}
		if strings.HasPrefix(line, tag+" ") {
			if strings.Contains(strings.ToUpper(line), " OK") {
				return lines, literals, nil
			}
			return nil, nil, fmt.Errorf("%s", line)
		}
	}
}

func (c *imapClient) readLine() (string, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func parseLiteralSize(line string) (int, bool) {
	matches := literalPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}
	size, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return size, true
}

func parseRFC822(sequence string, body []byte) (*imapMessage, error) {
	message, err := mail.ReadMessage(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imap parse message: %w", err)
	}
	textBody, htmlBody, err := decodeMessageBody(message.Header, message.Body)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{}
	for key, values := range message.Header {
		headers[key] = strings.Join(values, ", ")
	}
	return &imapMessage{
		Sequence:  sequence,
		MessageID: message.Header.Get("Message-Id"),
		Subject:   message.Header.Get("Subject"),
		From:      parseAddressHeader(message.Header.Get("From")),
		To:        parseAddressHeader(message.Header.Get("To")),
		Date:      parseDateHeader(message.Header),
		Text:      textBody,
		HTML:      htmlBody,
		Headers:   headers,
	}, nil
}

func decodeMessageBody(header mail.Header, body io.Reader) (string, string, error) {
	contentType := header.Get("Content-Type")
	if contentType == "" {
		decoded, err := io.ReadAll(decodeTransferEncoding(header.Get("Content-Transfer-Encoding"), body))
		if err != nil {
			return "", "", fmt.Errorf("imap read body: %w", err)
		}
		return string(decoded), "", nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", "", fmt.Errorf("imap parse content type: %w", err)
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(
			decodeTransferEncoding(header.Get("Content-Transfer-Encoding"), body),
			params["boundary"],
		)
		var textBody string
		var htmlBody string
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", "", fmt.Errorf("imap read multipart body: %w", err)
			}
			partBytes, err := io.ReadAll(decodeTransferEncoding(part.Header.Get("Content-Transfer-Encoding"), part))
			_ = part.Close()
			if err != nil {
				return "", "", fmt.Errorf("imap read multipart part: %w", err)
			}
			partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			switch partType {
			case "text/plain":
				if textBody == "" {
					textBody = string(partBytes)
				}
			case "text/html":
				if htmlBody == "" {
					htmlBody = string(partBytes)
				}
			}
		}
		return textBody, htmlBody, nil
	}
	decoded, err := io.ReadAll(decodeTransferEncoding(header.Get("Content-Transfer-Encoding"), body))
	if err != nil {
		return "", "", fmt.Errorf("imap read body: %w", err)
	}
	if mediaType == "text/html" {
		return "", string(decoded), nil
	}
	return string(decoded), "", nil
}

func decodeTransferEncoding(encoding string, reader io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		return quotedprintable.NewReader(reader)
	default:
		return reader
	}
}

func parseAddressHeader(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	addresses, err := mail.ParseAddressList(raw)
	if err != nil {
		return []string{raw}
	}
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, strings.ToLower(address.Address))
	}
	return out
}

func parseDateHeader(header mail.Header) string {
	date, err := header.Date()
	if err != nil {
		return ""
	}
	return date.UTC().Format(time.RFC3339)
}

func matchesCriteria(message *imapMessage, criteria criteriaConfig) bool {
	if message == nil {
		return false
	}
	if criteria.MessageID != "" &&
		!strings.EqualFold(strings.TrimSpace(message.MessageID), strings.TrimSpace(criteria.MessageID)) {
		return false
	}
	if criteria.From != "" && !containsAddress(message.From, criteria.From) {
		return false
	}
	if criteria.To != "" && !containsAddress(message.To, criteria.To) {
		return false
	}
	if criteria.SubjectContains != "" && !containsFold(message.Subject, criteria.SubjectContains) {
		return false
	}
	if criteria.BodyContains != "" && !containsFold(message.Text+"\n"+message.HTML, criteria.BodyContains) {
		return false
	}
	return true
}

func containsAddress(addresses []string, expected string) bool {
	expected = strings.ToLower(strings.TrimSpace(expected))
	for _, address := range addresses {
		if strings.EqualFold(address, expected) || strings.Contains(strings.ToLower(address), expected) {
			return true
		}
	}
	return false
}

func containsFold(value, expected string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(expected)))
}
