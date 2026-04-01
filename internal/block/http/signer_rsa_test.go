package http //nolint:revive // package name matches the block domain intentionally.

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func generateTestRSAKey(t *testing.T) (pemKey string, privateKey *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block)), key
}

func verifyRSASHA256(t *testing.T, key *rsa.PrivateKey, message, sigB64 string) {
	t.Helper()
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("decode base64 signature: %v", err)
	}
	digest := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("RSA signature verification failed: %v", err)
	}
}

func TestRSASigner_Header(t *testing.T) {
	pemKey, privateKey := generateTestRSAKey(t)
	body := `{"order":"123"}`

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
				"type":    "rsa_sha256",
				"key":     pemKey,
				"include": []any{"body"},
				"header":  "X-Signature",
			},
		},
	})

	if gotSig == "" {
		t.Fatal("X-Signature header is empty")
	}
	verifyRSASHA256(t, privateKey, body, gotSig)
}

func TestRSASigner_HeaderWithPrefix(t *testing.T) {
	pemKey, privateKey := generateTestRSAKey(t)
	body := `{"order":"123"}`
	const prefix = "rsa-sha256="

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
				"type":    "rsa_sha256",
				"key":     pemKey,
				"include": []any{"body"},
				"header":  "X-Signature",
				"prefix":  prefix,
			},
		},
	})

	if !strings.HasPrefix(gotSig, prefix) {
		t.Fatalf("X-Signature = %q, want prefix %q", gotSig, prefix)
	}
	verifyRSASHA256(t, privateKey, body, strings.TrimPrefix(gotSig, prefix))
}

func TestRSASigner_BodyField(t *testing.T) {
	pemKey, privateKey := generateTestRSAKey(t)
	body := `{"amount":500}`

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
				"type":       "rsa_sha256",
				"key":        pemKey,
				"include":    []any{"body"},
				"body_field": "sign",
			},
		},
	})

	if gotBody == nil {
		t.Fatal("server received no body")
	}
	sig, _ := gotBody["sign"].(string)
	if sig == "" {
		t.Fatal("body.sign field is empty")
	}
	verifyRSASHA256(t, privateKey, body, sig)
}

func TestRSASigner_EscapedNewlinesInKey(t *testing.T) {
	pemKey, _ := generateTestRSAKey(t)
	escaped := strings.ReplaceAll(pemKey, "\n", `\n`)

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	_, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodPost,
			"body":   `{"x":1}`,
			"sign": map[string]any{
				"type":    "rsa_sha256",
				"key":     escaped,
				"include": []any{"body"},
				"header":  "X-Sig",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() with escaped newlines in key: %v", err)
	}
}

func TestNewRSASigner_InvalidKey(t *testing.T) {
	_, err := newRSASigner(authConfig{
		Type:   "rsa_sha256",
		Key:    "not-a-pem-key",
		Header: "X-Sig",
	})
	if err == nil {
		t.Fatal("expected error for invalid PEM key, got nil")
	}
}

func TestHMACSHA512_Header(t *testing.T) {
	body := `{"hello":"world"}`

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
				"type":    "hmac_sha512",
				"key":     testKey,
				"include": []any{"body"},
				"header":  "X-Signature",
			},
		},
	})

	if gotSig == "" {
		t.Fatal("X-Signature header is empty")
	}
	// SHA512 produces a 128-char hex string vs 64-char for SHA256
	if len(gotSig) != 128 {
		t.Fatalf("hmac_sha512 signature length = %d, want 128", len(gotSig))
	}
}
