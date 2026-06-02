// Package search implements the search_documents MCP tool.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Bannump/go-mcp-gateway/internal/mcp"
)

// document is an entry in the hardcoded corpus.
type document struct {
	ID      string
	Title   string
	Content string
}

var corpus = []document{
	{"doc-1", "Introduction to Go", "Go is a statically typed compiled language designed at Google. It features garbage collection, strong typing, and excellent concurrency primitives via goroutines and channels."},
	{"doc-2", "Understanding Goroutines", "Goroutines are lightweight threads managed by the Go runtime. They enable massive concurrency. Use channels to communicate between goroutines and avoid shared state."},
	{"doc-3", "Model Context Protocol Overview", "MCP is a protocol for AI models to interact with tools and data sources using JSON-RPC 2.0. It standardises how models access external context."},
	{"doc-4", "JWT and OIDC Authentication", "JSON Web Tokens encode claims signed with RS256. OIDC extends OAuth 2.0 for identity. JWTs are verified using the issuer's public key fetched from a JWKS endpoint."},
	{"doc-5", "Prometheus Metrics in Go", "Use the prometheus/client_golang library to expose counters, histograms, and gauges. Metrics are scraped by Prometheus and visualised in Grafana dashboards."},
	{"doc-6", "SQLite WAL Mode", "SQLite's Write-Ahead Logging mode allows concurrent reads while a write is in progress. Enable it with PRAGMA journal_mode=WAL. Ideal for single-server deployments."},
	{"doc-7", "Docker Multi-Stage Builds", "Multi-stage Dockerfiles separate the build environment from the runtime image. Copy only the compiled binary into a minimal distroless image to reduce attack surface."},
	{"doc-8", "Rate Limiting with Token Bucket", "The token bucket algorithm allows bursts up to bucket capacity, then refills at a fixed rate. golang.org/x/time/rate provides a production-ready implementation."},
	{"doc-9", "Server-Sent Events", "SSE lets a server push events to a client over a persistent HTTP connection. The client uses an EventSource and receives data lines prefixed with 'data:'."},
	{"doc-10", "Structured Logging with slog", "Go 1.21 introduced the log/slog package for structured logging. Use JSON handler in production for log aggregation systems like Loki or CloudWatch."},
	{"doc-11", "Graceful Shutdown in Go", "Trap SIGINT/SIGTERM with os.Signal. Call http.Server.Shutdown with a timeout context to drain in-flight requests before exiting. Always close storage connections cleanly."},
	{"doc-12", "TF-IDF Relevance Scoring", "TF-IDF scores how relevant a term is to a document relative to the corpus. Term Frequency counts how often the term appears; Inverse Document Frequency penalises common terms."},
	{"doc-13", "OpenAPI 3.1 Specification", "OpenAPI 3.1 aligns with JSON Schema and supports webhooks. Define endpoints, request/response bodies, and security schemes. Use schema refs to avoid duplication."},
	{"doc-14", "Context Propagation in Go", "Pass context.Context as the first argument to functions that do I/O or call external services. Use context.WithTimeout and context.WithCancel for deadline and cancellation propagation."},
	{"doc-15", "GitHub Actions CI/CD", "GitHub Actions automates lint, test, and build pipelines triggered by push or pull_request events. Use reusable workflows and cache Go modules to speed up builds."},
}

// SearchTool implements the search_documents MCP tool.
type SearchTool struct{}

// New creates a SearchTool.
func New() *SearchTool { return &SearchTool{} }

// Name returns the tool name.
func (s *SearchTool) Name() string { return "search_documents" }

// Description returns a human-readable description.
func (s *SearchTool) Description() string {
	return "Search a document corpus using TF-IDF keyword scoring. Returns the most relevant documents for a query."
}

// InputSchema returns the JSON Schema for the tool's input.
func (s *SearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["query"],
		"properties": {
			"query": {"type": "string", "description": "Search query"},
			"top_k": {"type": "integer", "description": "Max results (default 5, max 20)", "default": 5, "maximum": 20}
		}
	}`)
}

type searchInput struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type searchResult struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// Execute runs TF-IDF scoring and returns the top_k results.
func (s *SearchTool) Execute(_ context.Context, params json.RawMessage) (mcp.ToolResult, error) {
	var input searchInput
	if err := json.Unmarshal(params, &input); err != nil {
		return mcp.ErrorResult("invalid parameters"), nil
	}

	q := strings.TrimSpace(input.Query)
	if q == "" {
		return mcp.ErrorResult("query must not be empty"), nil
	}

	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}

	results := score(q, topK)

	out, err := json.Marshal(results)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("marshal error: %v", err)), nil
	}
	return mcp.TextResult(string(out)), nil
}

func tokenise(text string) []string {
	text = strings.ToLower(text)
	text = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return ' '
	}, text)
	return strings.Fields(text)
}

func termFreq(tokens []string) map[string]float64 {
	tf := make(map[string]float64, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}
	n := float64(len(tokens))
	if n > 0 {
		for k := range tf {
			tf[k] /= n
		}
	}
	return tf
}

func score(query string, topK int) []searchResult {
	queryTokens := tokenise(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Compute IDF per query term over corpus.
	idf := make(map[string]float64, len(queryTokens))
	for _, qt := range queryTokens {
		df := 0
		for _, doc := range corpus {
			if strings.Contains(strings.ToLower(doc.Content+" "+doc.Title), qt) {
				df++
			}
		}
		if df > 0 {
			idf[qt] = math.Log(float64(len(corpus)+1) / float64(df))
		}
	}

	type scored struct {
		doc   document
		score float64
	}
	var results []scored

	for _, doc := range corpus {
		docTokens := tokenise(doc.Content + " " + doc.Title)
		tf := termFreq(docTokens)
		var tfidf float64
		for _, qt := range queryTokens {
			tfidf += tf[qt] * idf[qt]
		}
		if tfidf > 0 {
			results = append(results, scored{doc: doc, score: tfidf})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]searchResult, len(results))
	for i, r := range results {
		snippet := r.doc.Content
		if len(snippet) > 150 {
			snippet = snippet[:150] + "..."
		}
		out[i] = searchResult{
			ID:      r.doc.ID,
			Title:   r.doc.Title,
			Snippet: snippet,
			Score:   math.Round(r.score*1000) / 1000,
		}
	}
	return out
}
