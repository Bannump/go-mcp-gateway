package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the gateway.
type Metrics struct {
	RequestsTotal        *prometheus.CounterVec
	RequestDuration      *prometheus.HistogramVec
	ToolCallsTotal       *prometheus.CounterVec
	AuthFailuresTotal    *prometheus.CounterVec
	JWKSRefreshesTotal   prometheus.Counter
	JWKSRefreshFailures  prometheus.Counter
}

// NewMetrics registers and returns all Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		RequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "mcp_requests_total",
			Help: "Total MCP JSON-RPC requests by method and status.",
		}, []string{"method", "status"}),

		RequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "mcp_request_duration_seconds",
			Help:    "Latency of MCP JSON-RPC requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),

		ToolCallsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "mcp_tool_calls_total",
			Help: "Total tool calls by tool name and status.",
		}, []string{"tool_name", "status"}),

		AuthFailuresTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "auth_failures_total",
			Help: "Total authentication failures by reason.",
		}, []string{"reason"}),

		JWKSRefreshesTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "jwks_cache_refreshes_total",
			Help: "Total successful JWKS cache refreshes.",
		}),

		JWKSRefreshFailures: factory.NewCounter(prometheus.CounterOpts{
			Name: "jwks_cache_refresh_failures_total",
			Help: "Total failed JWKS cache refresh attempts.",
		}),
	}
}
