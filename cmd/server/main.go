package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Bannump/go-mcp-gateway/internal/config"
	"github.com/Bannump/go-mcp-gateway/internal/mcp/transport"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	stopCh := make(chan struct{})
	deps, err := wire(cfg, stopCh)
	if err != nil {
		return err
	}
	defer deps.store.Close() //nolint:errcheck

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if cfg.Transport == "stdio" {
		deps.logger.Info("starting stdio transport")
		return transport.NewStdioTransport(deps.server, deps.logger).Run(ctx)
	}

	httpT := transport.NewHTTPTransport(deps.server, deps.logger, deps.authMw, deps.jwks.Healthy, cfg.RateLimitRPM)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: httpT.Handler()}
	go func() {
		deps.logger.Info("starting http transport", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			deps.logger.Error("http server error", "error", err)
		}
	}()
	<-ctx.Done()
	deps.logger.Info("shutdown signal received")
	close(stopCh)
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	deps.logger.Info("server stopped")
	return nil
}
