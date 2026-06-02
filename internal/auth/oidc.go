package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Bannump/go-mcp-gateway/pkg/jwks"
)

// Sentinel errors returned by VerifyToken.
var (
	ErrTokenExpired      = errors.New("auth: token expired")
	ErrInvalidSignature  = errors.New("auth: invalid signature")
	ErrInvalidClaims     = errors.New("auth: invalid claims")
	ErrUnknownKey        = errors.New("auth: unknown key id")
)

// Verifier validates OIDC JWTs against a JWKS endpoint.
type Verifier struct {
	jwksClient *jwks.Client
	issuer     string
	audience   string
}

// NewVerifier creates an OIDC verifier. Constructor args determine the OIDC provider.
func NewVerifier(jwksClient *jwks.Client, issuer, audience string) *Verifier {
	return &Verifier{
		jwksClient: jwksClient,
		issuer:     issuer,
		audience:   audience,
	}
}

// VerifyToken parses and validates a JWT, returning verified Claims.
func (v *Verifier) VerifyToken(tokenString string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method: %v", t.Header["alg"])
		}

		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, ErrUnknownKey
		}

		pub, err := v.jwksClient.GetKey(kid)
		if err != nil {
			return nil, ErrUnknownKey
		}
		return pub, nil
	}, jwt.WithValidMethods([]string{"RS256"}))

	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownKey):
			return nil, ErrUnknownKey
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return nil, ErrInvalidSignature
		default:
			return nil, fmt.Errorf("%w: %v", ErrInvalidClaims, err)
		}
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	iss, _ := mapClaims["iss"].(string)
	if iss != v.issuer {
		return nil, fmt.Errorf("%w: issuer mismatch: got %q want %q", ErrInvalidClaims, iss, v.issuer)
	}

	if !audienceContains(mapClaims, v.audience) {
		return nil, fmt.Errorf("%w: audience mismatch", ErrInvalidClaims)
	}

	iat, _ := mapClaims["iat"].(float64)
	if iat > 0 && time.Unix(int64(iat), 0).After(time.Now().Add(5*time.Minute)) {
		return nil, fmt.Errorf("%w: iat in the future", ErrInvalidClaims)
	}

	exp, _ := mapClaims["exp"].(float64)
	sub, _ := mapClaims["sub"].(string)
	email, _ := mapClaims["email"].(string)

	raw := make(map[string]interface{}, len(mapClaims))
	for k, val := range mapClaims {
		raw[k] = val
	}

	return &Claims{
		Subject:   sub,
		Email:     email,
		Issuer:    iss,
		ExpiresAt: time.Unix(int64(exp), 0),
		Raw:       raw,
	}, nil
}

func audienceContains(claims jwt.MapClaims, target string) bool {
	switch v := claims["aud"].(type) {
	case string:
		return v == target
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok && s == target {
				return true
			}
		}
	}
	return false
}
