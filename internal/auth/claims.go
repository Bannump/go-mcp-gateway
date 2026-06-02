// Package auth handles OIDC token verification and HTTP middleware.
package auth

import (
	"context"
	"time"
)

type contextKey string

const claimsKey contextKey = "claims"

// Claims holds verified identity information extracted from a JWT.
type Claims struct {
	Subject   string
	Email     string
	Issuer    string
	ExpiresAt time.Time
	Raw       map[string]interface{}
}

// FromContext retrieves Claims stored in the request context by the auth middleware.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

// withClaims returns a new context with the given Claims attached.
func withClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}
