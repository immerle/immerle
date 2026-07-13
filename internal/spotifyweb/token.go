package spotifyweb

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

// secretsURL is a community-maintained tracker of Spotify's current TOTP
// cipher (reverse-engineered from the web player's JS bundle, which Spotify
// rotates periodically). It's fetched fresh on every access-token refresh
// (roughly hourly — see accessToken.valid), so a rotation heals itself
// without a code change. Deliberately no hardcoded fallback secret: shipping
// Spotify's cipher bytes in source is exactly what got a similar tracker repo
// a cease-and-desist — if secretsURL is unreachable or its format changes,
// this fails loudly instead.
const secretsURL = "https://raw.githubusercontent.com/xyloflake/spot-secrets-go/main/secrets/secretDict.json"

// currentSecret fetches the freshest (version, cipher) pair from secretsURL.
func currentSecret(ctx context.Context, client *http.Client) (version int, cipher []byte, err error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, secretsURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("fetching TOTP secret from %s: %w", secretsURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("fetching TOTP secret from %s: returned %s", secretsURL, resp.Status)
	}

	var dict map[string][]int
	if err := json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(&dict); err != nil {
		return 0, nil, fmt.Errorf("decoding TOTP secret from %s: %w", secretsURL, err)
	}

	best := -1
	var bestCipher []byte
	for k, v := range dict {
		n, err := strconv.Atoi(k)
		if err != nil || len(v) == 0 || n <= best {
			continue
		}
		best = n
		bestCipher = make([]byte, len(v))
		for i, b := range v {
			bestCipher[i] = byte(b)
		}
	}
	if best < 0 {
		return 0, nil, fmt.Errorf("no usable TOTP secret found in %s", secretsURL)
	}
	return best, bestCipher, nil
}

// totpKey derives the HMAC-SHA1 key from the versioned cipher: XOR each byte
// with (its index mod 33) + 9, then concatenate the decimal digits of the
// results into one ASCII string — that string's bytes are the key. (Spotify's
// own implementation base32-encodes that string for the client and the
// client base32-decodes it back before use; the round trip is a no-op, so
// this skips both steps.)
func totpKey(cipher []byte) []byte {
	var sb strings.Builder
	for i, b := range cipher {
		sb.WriteString(strconv.Itoa(int(b ^ byte((i%33)+9))))
	}
	return []byte(sb.String())
}

// totpCode computes the 6-digit TOTP code (RFC 6238, SHA1, 30s step) for t.
func totpCode(key []byte, t time.Time) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(t.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}

type accessToken struct {
	value     string
	expiresAt time.Time
}

func (t accessToken) valid() bool {
	return t.value != "" && time.Now().Before(t.expiresAt)
}

// fetchAccessToken mints a fresh anonymous web-player access token by
// answering the TOTP challenge above.
func fetchAccessToken(ctx context.Context, client *http.Client) (accessToken, error) {
	version, cipher, err := currentSecret(ctx, client)
	if err != nil {
		return accessToken{}, fmt.Errorf("spotifyweb: %w", err)
	}
	code := totpCode(totpKey(cipher), time.Now())
	q := url.Values{
		"reason":      {"init"},
		"productType": {"web-player"},
		"totp":        {code},
		"totpServer":  {code},
		"totpVer":     {strconv.Itoa(version)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://open.spotify.com/api/token?"+q.Encode(), nil)
	if err != nil {
		return accessToken{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://open.spotify.com/")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return accessToken{}, fmt.Errorf("spotifyweb: requesting access token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return accessToken{}, fmt.Errorf("spotifyweb: access token request returned %s (TOTP secret v%d from %s may be stale)", resp.Status, version, secretsURL)
	}

	var body struct {
		AccessToken string `json:"accessToken"`
		ExpiresAtMs int64  `json:"accessTokenExpirationTimestampMs"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(&body); err != nil {
		return accessToken{}, fmt.Errorf("spotifyweb: decoding access token response: %w", err)
	}
	if body.AccessToken == "" {
		return accessToken{}, fmt.Errorf("spotifyweb: access token response had no token")
	}
	return accessToken{value: body.AccessToken, expiresAt: time.UnixMilli(body.ExpiresAtMs)}, nil
}
