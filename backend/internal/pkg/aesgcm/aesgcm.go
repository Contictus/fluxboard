// Package aesgcm is a tiny authenticated-encryption helper (AES-256-GCM) used to
// encrypt secrets at rest — currently the TOTP shared secret (docs/07 §1: TOTP
// secret encrypted at rest, key from env). Ciphertext is self-describing:
// base64( nonce || ciphertext||tag ), so no separate nonce storage is needed.
package aesgcm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeySize is the required key length (AES-256).
const KeySize = 32

// Cipher encrypts/decrypts with a fixed key.
type Cipher struct{ aead cipher.AEAD }

// New builds a Cipher from a 32-byte key. The key is typically decoded from a
// base64 env var (TOTP_ENC_KEY) — see ParseKey.
func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("aesgcm: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// ParseKey decodes a base64 (std or raw) 32-byte key.
func ParseKey(b64 string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if k, err := enc.DecodeString(b64); err == nil {
			return k, nil
		}
	}
	return nil, errors.New("aesgcm: key is not valid base64")
}

// Encrypt seals plaintext and returns base64( nonce || ciphertext ).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("aesgcm: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. A tampered or wrong-key ciphertext returns an error.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("aesgcm: decode: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("aesgcm: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("aesgcm: open: %w", err)
	}
	return string(pt), nil
}
