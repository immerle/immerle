package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// secretBox provides reversible password encryption keyed by the config secret.
// Passwords are encrypted, not one-way hashed, because Subsonic's legacy token
// auth (md5(password+salt)) needs the server to recover the plaintext. Trade-off:
// an attacker with both the DB and the config secret can decrypt every password.
// Switching to hashing would break Subsonic auth for existing users.
type secretBox struct {
	gcm cipher.AEAD
}

func newSecretBox(secret string) (*secretBox, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretBox{gcm: gcm}, nil
}

// Encrypt returns a base64 ciphertext of plaintext.
func (s *secretBox) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt recovers the plaintext from a base64 ciphertext produced by Encrypt.
func (s *secretBox) Decrypt(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode password: %w", err)
	}
	ns := s.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := s.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt password: %w", err)
	}
	return string(pt), nil
}
