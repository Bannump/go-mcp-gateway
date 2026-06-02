// Package transport implements HTTP+SSE and stdio MCP transports.
package transport

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/Bannump/go-mcp-gateway/internal/auth"
	"github.com/Bannump/go-mcp-gateway/internal/mcp"
)

// Handler is called by the HTTP server to indicate storage and JWKS health.
type ReadinessChecker func() bool

// HTTPTransport wires MCP over HTTP with SSE for server-initiated messages.
type HTTPTransport struct {
	server    *mcp.Server
	logger    *slog.Logger
	authMw    func(http.Handler) http.Handler
	readiness ReadinessChecker

	rateLimitRPM int
	limiters     sync.Map // subject → *rate.Limiter

	sseClients   sync.Map // chan string → struct{}
}

// NewHTTPTransport creates an HTTPTransport.
func NewHTTPTransport(
	server *mcp.Server,
	logger *slog.Logger,
	authMw func(http.Handler) http.Handler,
	readiness ReadinessChecker,
	rateLimitRPM int,
) *HTTPTransport {
	return &HTTPTransport{
		server:       server,
		logger:       logger,
		authMw:       authMw,
		readiness:    readiness,
		rateLimitRPM: rateLimitRPM,
	}
}

// Handler returns the http.Handler for all routes.
func (h *HTTPTransport) Handler() http.Handler {
	mux := http.NewServeMux()

	// Unauthenticated probes and metrics.
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)
	mux.Handle("/metrics", promhttp.Handler())

	// Authenticated MCP routes.
	mcpHandler := h.authMw(h.rateLimitMiddleware(http.HandlerFunc(h.handleMCP)))
	sseHandler := h.authMw(http.HandlerFunc(h.handleSSE))

	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/events", sseHandler)

	return mux
}

func (h *HTTPTransport) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func (h *HTTPTransport) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.readiness != nil && !h.readiness() {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"not ready"}`)
		return
	}
	fmt.Fprint(w, `{"status":"ready"}`)
}

func (h *HTTPTransport) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSONRPCError(w, nil, mcp.CodeParseError, "parse error")
		return
	}

	claims, _ := auth.FromContext(r.Context())
	subject := ""
	if claims != nil {
		subject = claims.Subject
	}

	resp, hasResp := h.server.Handle(r.Context(), raw)

	h.logger.Info("mcp request",
		"method", extractMethod(raw),
		"client_subject", subject,
		"latency_ms", 0,
	)

	if !hasResp {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *HTTPTransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 16)
	h.sseClients.Store(ch, struct{}{})
	defer h.sseClients.Delete(ch)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// rateLimitMiddleware enforces per-subject rate limiting.
func (h *HTTPTransport) rateLimitMiddleware(next http.Handler) http.Handler {
	rps := rate.Limit(float64(h.rateLimitRPM) / 60.0)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.FromContext(r.Context())
		if !ok || claims == nil {
			next.ServeHTTP(w, r)
			return
		}

		limiterAny, _ := h.limiters.LoadOrStore(claims.Subject, rate.NewLimiter(rps, h.rateLimitRPM))
		limiter := limiterAny.(*rate.Limiter)
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"code":"rate_limited","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSONRPCError(w http.ResponseWriter, id *json.RawMessage, code int, msg string) {
	resp := mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcp.RPCError{Code: code, Message: msg},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func extractMethod(raw json.RawMessage) string {
	var r struct {
		Method string `json:"method"`
	}
	json.Unmarshal(raw, &r) //nolint:errcheck
	return r.Method
}
