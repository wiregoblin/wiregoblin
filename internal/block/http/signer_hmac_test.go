package http //nolint:revive // package name matches the block domain intentionally.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

const testKey = "secret"

func testHMAC(message string) string {
	mac := hmac.New(sha256.New, []byte(testKey))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestHMACSigner_Header(t *testing.T) {
	body := `{"hello":"world"}`
	expected := testHMAC(body)

	var gotSig string
	server := httptest.NewServer(stdhttp.HandlerFunc(func(_ stdhttp.ResponseWriter, r *stdhttp.Request) {
		gotSig = r.Header.Get("X-Signature")
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	_, _ = New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodPost,
			"body":   body,
			"sign": map[string]any{
				"type":    "hmac_sha256",
				"key":     testKey,
				"include": []any{"body"},
				"header":  "X-Signature",
			},
		},
	})

	if gotSig != expected {
		t.Fatalf("X-Signature = %q, want %q", gotSig, expected)
	}
}

func TestHMACSigner_HeaderWithPrefix(t *testing.T) {
	body := `{"hello":"world"}`
	expected := "sha256=" + testHMAC(body)

	var gotSig string
	server := httptest.NewServer(stdhttp.HandlerFunc(func(_ stdhttp.ResponseWriter, r *stdhttp.Request) {
		gotSig = r.Header.Get("X-Signature")
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	_, _ = New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodPost,
			"body":   body,
			"sign": map[string]any{
				"type":    "hmac_sha256",
				"key":     testKey,
				"include": []any{"body"},
				"header":  "X-Signature",
				"prefix":  "sha256=",
			},
		},
	})

	if gotSig != expected {
		t.Fatalf("X-Signature = %q, want %q", gotSig, expected)
	}
}

func TestHMACSigner_QueryParam(t *testing.T) {
	body := `{"hello":"world"}`
	expected := testHMAC(body)

	var gotSig string
	server := httptest.NewServer(stdhttp.HandlerFunc(func(_ stdhttp.ResponseWriter, r *stdhttp.Request) {
		gotSig = r.URL.Query().Get("sig")
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	serverURL, _ := url.Parse(server.URL)
	serverURL.Path = "/api"

	_, _ = New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    serverURL.String(),
			"method": stdhttp.MethodPost,
			"body":   body,
			"sign": map[string]any{
				"type":        "hmac_sha256",
				"key":         testKey,
				"sign":        []any{"body"},
				"query_param": "sig",
			},
		},
	})

	if gotSig != expected {
		t.Fatalf("query sig = %q, want %q", gotSig, expected)
	}
}

func TestHMACSigner_BodyField_SortedJSON(t *testing.T) {
	// Body with unsorted keys — canonical form will be {"a":1,"b":2}
	body := `{"b":2,"a":1}`
	canonical := `{"a":1,"b":2}`
	expected := testHMAC(canonical)

	var gotBody map[string]any
	server := httptest.NewServer(stdhttp.HandlerFunc(func(_ stdhttp.ResponseWriter, r *stdhttp.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	_, _ = New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodPost,
			"body":   body,
			"sign": map[string]any{
				"type":        "hmac_sha256",
				"key":         testKey,
				"sign":        []any{"body"},
				"body_format": "sorted_json",
				"body_field":  "sign",
			},
		},
	})

	if gotBody == nil {
		t.Fatal("server received no body")
	}
	sig, _ := gotBody["sign"].(string)
	if sig != expected {
		t.Fatalf("body.sign = %q, want %q", sig, expected)
	}
}

func TestHMACSigner_SignMethodURLBody(t *testing.T) {
	body := `{"x":1}`

	var gotSig string
	server := httptest.NewServer(stdhttp.HandlerFunc(func(_ stdhttp.ResponseWriter, r *stdhttp.Request) {
		gotSig = r.Header.Get("X-Sig")
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	// Compute expected: method + "\n" + url + "\n" + body
	method := stdhttp.MethodPost
	rawURL := server.URL + "/"
	message := method + "\n" + rawURL + "\n" + body
	expected := testHMAC(message)

	_, _ = New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    rawURL,
			"method": method,
			"body":   body,
			"sign": map[string]any{
				"type":    "hmac_sha256",
				"key":     testKey,
				"include": []any{"method", "url", "body"},
				"header":  "X-Sig",
			},
		},
	})

	if gotSig != expected {
		t.Fatalf("X-Sig = %q, want %q", gotSig, expected)
	}
}

func TestNewSigner_ErrorOnMultipleDestinations(t *testing.T) {
	_, err := newSigner(authConfig{
		Type:       "hmac_sha256",
		Key:        "k",
		Header:     "X-Sig",
		QueryParam: "sig",
	})
	if err == nil {
		t.Fatal("expected error for multiple destinations, got nil")
	}
}

func TestNewSigner_ErrorOnUnknownType(t *testing.T) {
	_, err := newSigner(authConfig{
		Type:   "rsa",
		Key:    "k",
		Header: "X-Sig",
	})
	if err == nil {
		t.Fatal("expected error for unknown auth type, got nil")
	}
}

func TestNewSigner_NilWhenNoType(t *testing.T) {
	s, err := newSigner(authConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil signer when no type configured")
	}
}
