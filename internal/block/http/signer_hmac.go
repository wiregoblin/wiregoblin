package http //nolint:revive // package name matches the block domain intentionally.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"net/http"
)

type hmacSigner struct {
	cfg  authConfig
	hash func() hash.Hash
}

func newHMACSigner(cfg authConfig) (*hmacSigner, error) {
	var h func() hash.Hash
	switch cfg.Type {
	case "hmac_sha256":
		h = sha256.New
	case "hmac_sha512":
		h = sha512.New
	default:
		return nil, fmt.Errorf("hmac: unsupported type %q", cfg.Type)
	}
	return &hmacSigner{cfg: cfg, hash: h}, nil
}

// Sign computes an HMAC signature and writes it to the configured destination.
func (s *hmacSigner) Sign(req *http.Request, body []byte) ([]byte, error) {
	signingBody, err := prepareBody(body, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("hmac sign: %w", err)
	}

	message := buildMessage(req, signingBody, s.cfg)
	mac := hmac.New(s.hash, []byte(s.cfg.Key))
	mac.Write([]byte(message))
	sig := hex.EncodeToString(mac.Sum(nil))

	return applyDestination(req, body, s.cfg, s.cfg.Prefix+sig)
}
