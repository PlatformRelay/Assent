package github

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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// auth.go — the two credential shapes (REQ-E10-S06-02) and the identity the
// marker filter matches (ADR-0021 item 7).
//
// A PAT authenticates as a User: the artifacts assent authors are authored by
// the user login the client was constructed with, and Identity() returns that
// login as a user identity.
//
// A GitHub App installation authenticates as a BOT: the app identity mints a
// JWT from its private key, exchanges it for an installation access token, and
// requests carry that token. Token refresh is handled here; a missing or
// expired credential fails CLOSED with an error naming no secret material
// (REQ-E10-S06-02). The key material never appears in any error (REQ-E10-S06-03).

// appAuth carries the GitHub App installation credentials: the PEM-encoded RSA
// private key minting the app JWT, the App id, and the Installation id whose
// token is minted. None of the three may appear in a log line or an error.
type appAuth struct {
	// keyPEM is the PEM-encoded RSA private key minting the app JWT.
	keyPEM string
	appID  int64
	instID int64

	// minted caches the installation access token for the client's lifetime: a
	// run is one process, and re-minting per request would multiply auth
	// round-trips (the harness would also serve the exchange once, not once per
	// request). GitHub installation tokens expire (default one hour); the cache
	// re-mints only once the stored token's expiry is within a minute. The
	// cached token is secret material — it is never logged and never embedded in
	// an error (REQ-E10-S06-03).
	tokMu    sync.Mutex
	tok      string
	tokUntil time.Time
}

// WithApp installs the GitHub App credential shape: requests authenticate as
// the installation's bot identity and the installation token is minted from
// the app JWT, refreshed when it expires.
func WithApp(keyPEM string, appID, installationID int64) Option {
	return func(c *Client) {
		c.appAuth = &appAuth{keyPEM: keyPEM, appID: appID, instID: installationID}
	}
}

// bearer returns the credential the Authorization header carries. With a PAT it
// is the PAT itself; with App credentials it is the installation access token,
// minted from the app JWT. A missing or expired credential fails CLOSED with an
// error naming no secret material (REQ-E10-S06-02).
func (c *Client) bearer(ctx context.Context) (string, error) {
	if c.appAuth == nil {
		if strings.TrimSpace(c.token) == "" {
			return "", errors.New("github: missing credential — no token was supplied")
		}
		return c.token, nil
	}
	return c.appAuth.installationToken(ctx, c.endpoint)
}

// Identity reports the authenticated identity marker filtering matches
// (ADR-0021 item 7 / REQ-E10-S02-06): a PAT is a USER; a GitHub App
// installation is a BOT (the app-slug[bot]-shaped login its artifacts are
// authored as). The conformance identity case proves markers authored by the
// reported identity are recognised as our own under the shape the credential
// produces.
func (c *Client) Identity() (forge.Identity, error) {
	kind := forge.IdentityUser
	if c.appAuth != nil {
		kind = forge.IdentityApp
	}
	return forge.Identity{
		Kind:  kind,
		Login: c.botName,
		ID:    c.botName,
	}, nil
}

// appJWT mints the RS256 JWT the installation-token exchange consumes:
// {"alg":"RS256","typ":"JWT"} with {iat, exp (2 min), iss: <app id>}. The key
// may arrive as PKCS#1 ("BEGIN RSA PRIVATE KEY") or PKCS#8 ("BEGIN PRIVATE
// KEY") — GitHub issues both shapes. The signing key material never appears in
// an error (secret hygiene).
func (a *appAuth) appJWT() (string, error) {
	block, _ := pem.Decode([]byte(a.keyPEM))
	if block == nil {
		return "", errors.New("github: app credential is not a parseable PEM private key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// GitHub also issues PKCS#8 keys ("BEGIN PRIVATE KEY"): try that
		// shape, and require the decoded key to be RSA — the JWT is signed
		// RS256, and a non-RSA PKCS#8 key (an EC key) cannot produce it.
		parsed, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if pkcs8Err != nil {
			return "", errors.New("github: app credential is not an RSA private key")
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("github: app credential is not an RSA private key")
		}
		key = rsaKey
	}
	header, err := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", errors.New("github: app JWT header encoding failed")
	}
	now := time.Now().UTC()
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-30 * time.Second).Unix(),
		"exp": now.Add(2 * time.Minute).Unix(),
		"iss": a.appID,
	})
	if err != nil {
		return "", errors.New("github: app JWT claims encoding failed")
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(cryptoRandReader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", errors.New("github: app JWT signing failed")
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// installationToken mints an installation access token: POST
// /app/installations/{id}/access_tokens with the app JWT. The exchange rides
// the configured REST endpoint, not a hard-coded api.github.com: the public
// endpoint already serves /app/installations/{id}/access_tokens, and an
// httptest-backed harness serves the same route — the constructor seam that
// keeps the App credential path hermetic under test. The minted token lives
// for one Client (a run is one process; re-minting per request would multiply
// auth round-trips) and is re-minted only when its reported expiry is within a
// minute. An exchange failure is a hard error naming nothing secret.
func (a *appAuth) installationToken(ctx context.Context, endpoint string) (string, error) {
	a.tokMu.Lock()
	defer a.tokMu.Unlock()
	if a.tok != "" && time.Now().Before(a.tokUntil.Add(-time.Minute)) {
		return a.tok, nil
	}
	jwt, err := a.appJWT()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(endpoint, "/")+"/app/installations/"+fmt.Sprintf("%d", a.instID)+"/access_tokens",
		strings.NewReader(`{}`))
	if err != nil {
		return "", errors.New("github: build installation token request")
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("github: installation token exchange failed (transport): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := readBounded(resp.Body, 1<<16)
	if err != nil {
		return "", errors.New("github: installation token response read failed")
	}
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		var minted struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := json.Unmarshal(raw, &minted); err != nil || strings.TrimSpace(minted.Token) == "" {
			return "", errors.New("github: installation token exchange returned no usable token")
		}
		a.tok = minted.Token
		// GitHub reports an absolute expiry; when the field is absent the
		// documented default (one hour, minus a safety margin) is assumed.
		if exp, err := time.Parse(time.RFC3339, minted.ExpiresAt); err == nil {
			a.tokUntil = exp
		} else {
			a.tokUntil = time.Now().Add(55 * time.Minute)
		}
		return a.tok, nil
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return "", errors.New("github: installation credential rejected (401/403/404) — check app id, installation id and key; credential material is never echoed")
	default:
		return "", fmt.Errorf("github: installation token exchange: unexpected status %d", resp.StatusCode)
	}
}

// cryptoRandReader is the injected randomness seam for JWT signing.
// #nosec G404 -- not a decision input; the adapter is not the decision path.
var cryptoRandReader = rand.Reader
