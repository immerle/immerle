package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"time"
)

// mediaURLTTL bounds how long a signed stream/download URL stays valid — long
// enough for one track plus seeking, short enough that a leaked/logged URL is
// quickly useless.
const mediaURLTTL = 15 * time.Minute

// deriveSignKey derives the media-URL signing key from the server secret, so the
// raw auth secret is never used directly as the HMAC key.
func deriveSignKey(secret string) []byte {
	sum := sha256.Sum256([]byte("immerle-media-url-signing:" + secret))
	return sum[:]
}

// SignToken issues an expiry (unix seconds) and HMAC signature authorizing media
// access to id until the TTL elapses. Returns empty strings when signing is
// disabled (no key configured).
func (s *Server) SignToken(id string) (exp, sig string) {
	if len(s.signKey) == 0 {
		return "", ""
	}
	exp = strconv.FormatInt(time.Now().Add(mediaURLTTL).Unix(), 10)
	return exp, s.computeSig(id, exp)
}

func (s *Server) computeSig(id, exp string) string {
	mac := hmac.New(sha256.New, s.signKey)
	mac.Write([]byte(id + ":" + exp))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyToken reports whether (exp, sig) is a valid, unexpired signature for id.
func (s *Server) VerifyToken(id, exp, sig string) bool {
	if len(s.signKey) == 0 || exp == "" || sig == "" {
		return false
	}
	e, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > e {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s.computeSig(id, exp)), []byte(sig)) == 1
}
