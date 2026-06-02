// Package jwks provides a reusable JWKS client with caching.
package jwks

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// key holds a parsed RSA public key.
type key struct {
	publicKey *rsa.PublicKey
}

// jwkSet is the JSON representation of a JWKS response.
type jwkSet struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// Client fetches and caches JWKS from an OIDC provider.
type Client struct {
	jwksURI    string
	ttl        time.Duration
	httpClient *http.Client
	logger     *slog.Logger

	mu        sync.RWMutex
	cache     map[string]key
	fetchedAt time.Time

	refreshesTotal        func()
	refreshFailuresTotal  func()
}

// NewClient creates a JWKS client. ttl controls how long keys are cached before
// a background goroutine proactively refreshes them.
func NewClient(jwksURI string, ttl time.Duration, logger *slog.Logger, refreshesTotal, refreshFailuresTotal func()) *Client {
	c := &Client{
		jwksURI:              jwksURI,
		ttl:                  ttl,
		httpClient:           &http.Client{Timeout: 10 * time.Second},
		logger:               logger,
		cache:                make(map[string]key),
		refreshesTotal:       refreshesTotal,
		refreshFailuresTotal: refreshFailuresTotal,
	}
	return c
}

// Start fetches keys immediately, then launches a background refresh goroutine.
// It blocks until the first fetch completes. The goroutine stops when ctx is done.
func (c *Client) Start(stopCh <-chan struct{}) error {
	if err := c.refresh(); err != nil {
		return fmt.Errorf("jwks: initial fetch: %w", err)
	}
	go c.backgroundRefresh(stopCh)
	return nil
}

func (c *Client) backgroundRefresh(stopCh <-chan struct{}) {
	for {
		interval := c.ttl - 5*time.Minute
		if interval < time.Minute {
			interval = time.Minute
		}
		select {
		case <-stopCh:
			return
		case <-time.After(interval):
			if err := c.refresh(); err != nil {
				c.logger.Error("jwks: background refresh failed", "error", err)
				if c.refreshFailuresTotal != nil {
					c.refreshFailuresTotal()
				}
			}
		}
	}
}

func (c *Client) refresh() error {
	resp, err := c.httpClient.Get(c.jwksURI)
	if err != nil {
		return fmt.Errorf("jwks: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: unexpected status %d", resp.StatusCode)
	}

	var set jwkSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("jwks: decode: %w", err)
	}

	newCache := make(map[string]key, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAPublicKey(k)
		if err != nil {
			c.logger.Warn("jwks: skip malformed key", "kid", k.Kid, "error", err)
			continue
		}
		newCache[k.Kid] = key{publicKey: pub}
	}

	c.mu.Lock()
	c.cache = newCache
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	c.logger.Info("jwks: cache refreshed", "keys", len(newCache))
	if c.refreshesTotal != nil {
		c.refreshesTotal()
	}
	return nil
}

// GetKey returns the RSA public key for the given key ID.
func (c *Client) GetKey(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	k, ok := c.cache[kid]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("jwks: unknown kid %q", kid)
	}
	return k.publicKey, nil
}

// Healthy returns true if keys have been fetched successfully.
func (c *Client) Healthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache) > 0
}

func parseRSAPublicKey(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 | int(b)
	}

	return &rsa.PublicKey{N: n, E: eInt}, nil
}
