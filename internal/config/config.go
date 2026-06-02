// Package config loads and validates server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all server configuration.
type Config struct {
	OIDCIssuer   string
	OIDCAudience string
	OIDCJwksURI  string

	Port           string
	Transport      string
	StorageBackend string
	SQLitePath     string
	LogLevel       string
	LogFormat      string

	JWKSRefreshInterval time.Duration
	RateLimitRPM        int
}

// Load reads configuration from environment variables.
// It returns an error listing ALL missing required variables.
func Load() (*Config, error) {
	var missing []string

	required := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	optional := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	optionalInt := func(key string, def int) int {
		s := os.Getenv(key)
		if s == "" {
			return def
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		return v
	}

	cfg := &Config{
		OIDCIssuer:   required("OIDC_ISSUER"),
		OIDCAudience: required("OIDC_AUDIENCE"),
		OIDCJwksURI:  required("OIDC_JWKS_URI"),

		Port:           optional("PORT", "8080"),
		Transport:      optional("TRANSPORT", "http"),
		StorageBackend: optional("STORAGE_BACKEND", "memory"),
		SQLitePath:     optional("SQLITE_PATH", "./mcp.db"),
		LogLevel:       optional("LOG_LEVEL", "info"),
		LogFormat:      optional("LOG_FORMAT", "json"),

		JWKSRefreshInterval: time.Duration(optionalInt("JWKS_REFRESH_MINUTES", 60)) * time.Minute,
		RateLimitRPM:        optionalInt("RATE_LIMIT_RPM", 60),
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
