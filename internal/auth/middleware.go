package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Middleware returns an HTTP middleware that verifies bearer tokens.
func Middleware(verifier *Verifier, logger *slog.Logger, incAuthFailure func(reason string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				reject(w, r, logger, incAuthFailure, "missing_token", "missing or malformed Authorization header", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := verifier.VerifyToken(tokenStr)
			if err != nil {
				reason := authErrorReason(err)
				logger.Warn("auth: token verification failed",
					"reason", reason,
					"client_ip", clientIP(r),
				)
				reject(w, r, logger, incAuthFailure, reason, err.Error(), http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), claims)))
		})
	}
}

func reject(w http.ResponseWriter, _ *http.Request, _ *slog.Logger, incAuthFailure func(string), reason, message string, status int) {
	if incAuthFailure != nil {
		incAuthFailure(reason)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Code: reason, Message: message}) //nolint:errcheck
}

func authErrorReason(err error) string {
	switch {
	case errors.Is(err, ErrTokenExpired):
		return "token_expired"
	case errors.Is(err, ErrInvalidSignature):
		return "invalid_signature"
	case errors.Is(err, ErrUnknownKey):
		return "unknown_key"
	default:
		return "invalid_claims"
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}
