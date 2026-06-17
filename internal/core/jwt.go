package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Minimal HS256 JWT (RFC 7519) implementation — no external dependency. Used for
// device-session tokens: each carries a unique jti so it can be tracked and
// revoked via the devices registry.

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtClaims are the registered claims we use.
type jwtClaims struct {
	Sub string `json:"sub"` // user id
	JTI string `json:"jti"` // device id (unique token id)
	IAT int64  `json:"iat"`
	EXP int64  `json:"exp"`
}

var errInvalidJWT = errors.New("invalid jwt")

// looksLikeJWT reports whether a token is shaped like a JWT (three segments).
func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

func signJWT(key []byte, claims jwtClaims) (string, error) {
	header, _ := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64url(header) + "." + b64url(payload)
	sig := hmacSHA256(key, signingInput)
	return signingInput + "." + b64url(sig), nil
}

// parseJWT verifies the signature and expiry, returning the claims.
func parseJWT(key []byte, token string) (jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, errInvalidJWT
	}
	signingInput := parts[0] + "." + parts[1]
	expected := hmacSHA256(key, signingInput)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || subtle.ConstantTimeCompare(expected, got) != 1 {
		return jwtClaims{}, errInvalidJWT
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, errInvalidJWT
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return jwtClaims{}, errInvalidJWT
	}
	if claims.EXP > 0 && time.Now().Unix() > claims.EXP {
		return jwtClaims{}, errInvalidJWT
	}
	if claims.Sub == "" || claims.JTI == "" {
		return jwtClaims{}, errInvalidJWT
	}
	return claims, nil
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
