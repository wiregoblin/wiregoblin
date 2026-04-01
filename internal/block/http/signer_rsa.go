package http //nolint:revive // package name matches the block domain intentionally.

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
)

type rsaSigner struct {
	cfg authConfig
	key *rsa.PrivateKey
}

func newRSASigner(cfg authConfig) (*rsaSigner, error) {
	key, err := parseRSAPrivateKey(cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("rsa_sha256: parse private key: %w", err)
	}
	return &rsaSigner{cfg: cfg, key: key}, nil
}

// Sign computes an RSA-SHA256 signature and writes it to the configured destination.
// The signature is base64-encoded (standard encoding).
func (s *rsaSigner) Sign(req *http.Request, body []byte) ([]byte, error) {
	signingBody, err := prepareBody(body, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("rsa sign: %w", err)
	}

	message := buildMessage(req, signingBody, s.cfg)
	digest := sha256.Sum256([]byte(message))

	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return nil, fmt.Errorf("rsa sign: %w", err)
	}

	value := s.cfg.Prefix + base64.StdEncoding.EncodeToString(sig)
	return applyDestination(req, body, s.cfg, value)
}

// parseRSAPrivateKey parses a PEM-encoded RSA private key (PKCS#1 or PKCS#8).
// Literal \n escape sequences in the string are treated as newlines to support
// keys stored as single-line secrets.
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	pemStr = strings.ReplaceAll(pemStr, `\n`, "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %q", block.Type)
	}
}
