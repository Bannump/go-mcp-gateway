package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Bannump/go-mcp-gateway/internal/auth"
	"github.com/Bannump/go-mcp-gateway/internal/observability"
	"github.com/Bannump/go-mcp-gateway/pkg/jwks"
)

const (
	testIssuer   = "https://test.issuer.example"
	testAudience = "test-audience"
	testKid      = "test-key-1"
)

func makeTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func makeJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())

	body, _ := json.Marshal(map[string]interface{}{
		"keys": []map[string]interface{}{
			{"kty": "RSA", "kid": kid, "alg": "RS256", "n": n, "e": e},
		},
	})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) //nolint:errcheck
	}))
}

func makeToken(t *testing.T, key *rsa.PrivateKey, kid, issuer, audience string, exp time.Time, iat time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": "user-123",
		"iss": issuer,
		"aud": audience,
		"exp": exp.Unix(),
		"iat": iat.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

func newVerifier(t *testing.T, jwksURL, issuer, audience string) *auth.Verifier {
	t.Helper()
	logger := observability.NewLogger("text", "error")
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })

	client := jwks.NewClient(jwksURL, time.Hour, logger, nil, nil)
	require.NoError(t, client.Start(stopCh))
	return auth.NewVerifier(client, issuer, audience)
}

func TestVerifyToken_Valid(t *testing.T) {
	key := makeTestKey(t)
	srv := makeJWKSServer(t, key, testKid)
	defer srv.Close()

	verifier := newVerifier(t, srv.URL, testIssuer, testAudience)
	token := makeToken(t, key, testKid, testIssuer, testAudience, time.Now().Add(time.Hour), time.Now())

	claims, err := verifier.VerifyToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
}

func TestVerifyToken_Expired(t *testing.T) {
	key := makeTestKey(t)
	srv := makeJWKSServer(t, key, testKid)
	defer srv.Close()

	verifier := newVerifier(t, srv.URL, testIssuer, testAudience)
	token := makeToken(t, key, testKid, testIssuer, testAudience, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))

	_, err := verifier.VerifyToken(token)
	assert.ErrorIs(t, err, auth.ErrTokenExpired)
}

func TestVerifyToken_WrongIssuer(t *testing.T) {
	key := makeTestKey(t)
	srv := makeJWKSServer(t, key, testKid)
	defer srv.Close()

	verifier := newVerifier(t, srv.URL, testIssuer, testAudience)
	token := makeToken(t, key, testKid, "https://wrong.issuer", testAudience, time.Now().Add(time.Hour), time.Now())

	_, err := verifier.VerifyToken(token)
	assert.ErrorIs(t, err, auth.ErrInvalidClaims)
}

func TestVerifyToken_WrongAudience(t *testing.T) {
	key := makeTestKey(t)
	srv := makeJWKSServer(t, key, testKid)
	defer srv.Close()

	verifier := newVerifier(t, srv.URL, testIssuer, testAudience)
	token := makeToken(t, key, testKid, testIssuer, "wrong-audience", time.Now().Add(time.Hour), time.Now())

	_, err := verifier.VerifyToken(token)
	assert.ErrorIs(t, err, auth.ErrInvalidClaims)
}

func TestVerifyToken_UnknownKid(t *testing.T) {
	key := makeTestKey(t)
	srv := makeJWKSServer(t, key, testKid)
	defer srv.Close()

	verifier := newVerifier(t, srv.URL, testIssuer, testAudience)
	token := makeToken(t, key, "unknown-kid", testIssuer, testAudience, time.Now().Add(time.Hour), time.Now())

	_, err := verifier.VerifyToken(token)
	assert.ErrorIs(t, err, auth.ErrUnknownKey)
}

func TestVerifyToken_InvalidSignature(t *testing.T) {
	key := makeTestKey(t)
	srv := makeJWKSServer(t, key, testKid)
	defer srv.Close()

	wrongKey := makeTestKey(t)
	verifier := newVerifier(t, srv.URL, testIssuer, testAudience)
	// Sign with wrong key but advertise testKid so key lookup succeeds.
	token := makeToken(t, wrongKey, testKid, testIssuer, testAudience, time.Now().Add(time.Hour), time.Now())

	_, err := verifier.VerifyToken(token)
	assert.ErrorIs(t, err, auth.ErrInvalidSignature)
}
