package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Bannump/go-mcp-gateway/internal/auth"
	"github.com/Bannump/go-mcp-gateway/internal/config"
	"github.com/Bannump/go-mcp-gateway/internal/mcp"
	"github.com/Bannump/go-mcp-gateway/internal/observability"
	"github.com/Bannump/go-mcp-gateway/internal/storage"
	"github.com/Bannump/go-mcp-gateway/internal/tools"
	ctxtool "github.com/Bannump/go-mcp-gateway/internal/tools/context"
	memtool "github.com/Bannump/go-mcp-gateway/internal/tools/memory"
	"github.com/Bannump/go-mcp-gateway/internal/tools/search"
	"github.com/Bannump/go-mcp-gateway/pkg/jwks"
)

// dependencies holds all wired application components.
type dependencies struct {
	logger *slog.Logger
	server *mcp.Server
	store  storage.Store
	jwks   *jwks.Client
	authMw func(http.Handler) http.Handler
	cfg    *config.Config
}

// wire initialises all dependencies from config. stopCh closes JWKS background goroutine.
func wire(cfg *config.Config, stopCh <-chan struct{}) (*dependencies, error) {
	logger := observability.NewLogger(cfg.LogFormat, cfg.LogLevel)
	reg := prometheus.NewRegistry()
	m := observability.NewMetrics(reg)

	var jwksClient *jwks.Client
	var authMw func(http.Handler) http.Handler

	if cfg.DevMode {
		logger.Warn("DEV_MODE=true: auth is disabled — do not use in production")
		authMw = devPassthroughMiddleware
	} else {
		jwksClient = jwks.NewClient(cfg.OIDCJwksURI, cfg.JWKSRefreshInterval, logger,
			m.JWKSRefreshesTotal.Inc, m.JWKSRefreshFailures.Inc)
		if err := jwksClient.Start(stopCh); err != nil {
			return nil, fmt.Errorf("wire: jwks: %w", err)
		}
		verifier := auth.NewVerifier(jwksClient, cfg.OIDCIssuer, cfg.OIDCAudience)
		authMw = auth.Middleware(verifier, logger, func(reason string) {
			m.AuthFailuresTotal.WithLabelValues(reason).Inc()
		})
	}

	store, err := buildStore(cfg)
	if err != nil {
		return nil, err
	}

	registry, err := buildRegistry(store)
	if err != nil {
		return nil, err
	}

	mcpServer := mcp.NewServer(registry, logger, mcp.ServerMetrics{
		IncRequest:      func(method, s string) { m.RequestsTotal.WithLabelValues(method, s).Inc() },
		ObserveDuration: func(method string, d time.Duration) { m.RequestDuration.WithLabelValues(method).Observe(d.Seconds()) },
		IncToolCall:     func(name, s string) { m.ToolCallsTotal.WithLabelValues(name, s).Inc() },
	})

	return &dependencies{logger: logger, server: mcpServer, store: store, jwks: jwksClient, authMw: authMw, cfg: cfg}, nil
}

func buildStore(cfg *config.Config) (storage.Store, error) {
	if cfg.StorageBackend == "sqlite" {
		s, err := storage.NewSQLiteStore(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("wire: sqlite: %w", err)
		}
		return s, nil
	}
	return storage.NewMemoryStore(), nil
}

func buildRegistry(store storage.Store) (*tools.Registry, error) {
	registry := tools.NewRegistry()
	searchTool := search.New()
	toolList := []tools.Tool{
		searchTool,
		memtool.NewStoreTool(store),
		memtool.NewRetrieveTool(store),
		ctxtool.New(searchTool, store),
	}
	for _, t := range toolList {
		if err := registry.Register(t); err != nil {
			return nil, fmt.Errorf("wire: register %s: %w", t.Name(), err)
		}
	}
	return registry, nil
}
