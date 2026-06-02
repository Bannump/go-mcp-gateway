// Package contexttools implements the get_context MCP tool.
package contexttools

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Bannump/go-mcp-gateway/internal/mcp"
	"github.com/Bannump/go-mcp-gateway/internal/storage"
	"github.com/Bannump/go-mcp-gateway/internal/tools"
)

// ContextTool assembles multi-source context from search and memory.
type ContextTool struct {
	search tools.Tool
	store  storage.Store
}

// New creates a ContextTool. search must be the search_documents tool.
func New(search tools.Tool, store storage.Store) *ContextTool {
	return &ContextTool{search: search, store: store}
}

// Name returns the tool name.
func (t *ContextTool) Name() string { return "get_context" }

// Description returns a human-readable description.
func (t *ContextTool) Description() string {
	return "Assemble context from document search and memory store for a given query."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *ContextTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["query"],
		"properties": {
			"query":          {"type": "string", "description": "The query to build context for"},
			"include_memory": {"type": "boolean", "description": "Include memory store hits (default true)", "default": true},
			"top_k":          {"type": "integer", "description": "Max search results (default 3)", "default": 3}
		}
	}`)
}

type contextInput struct {
	Query         string `json:"query"`
	IncludeMemory *bool  `json:"include_memory"`
	TopK          int    `json:"top_k"`
}

type contextOutput struct {
	SearchResults  json.RawMessage `json:"search_results"`
	MemoryHits     []memoryHit     `json:"memory_hits"`
	ContextSummary string          `json:"context_summary"`
}

type memoryHit struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Execute assembles context from search results and memory store.
func (t *ContextTool) Execute(ctx stdctx.Context, params json.RawMessage) (mcp.ToolResult, error) {
	var input contextInput
	if err := json.Unmarshal(params, &input); err != nil {
		return mcp.ErrorResult("invalid parameters"), nil
	}

	q := strings.TrimSpace(input.Query)
	if q == "" {
		return mcp.ErrorResult("query must not be empty"), nil
	}

	topK := input.TopK
	if topK <= 0 {
		topK = 3
	}

	includeMemory := true
	if input.IncludeMemory != nil {
		includeMemory = *input.IncludeMemory
	}

	// Run document search.
	searchParams, _ := json.Marshal(map[string]interface{}{"query": q, "top_k": topK})
	searchResult, err := t.search.Execute(ctx, searchParams)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("search error: %v", err)), nil
	}

	var rawSearch json.RawMessage
	if len(searchResult.Content) > 0 {
		rawSearch = json.RawMessage(searchResult.Content[0].Text)
	} else {
		rawSearch = json.RawMessage("[]")
	}

	// Count search results.
	var searchItems []json.RawMessage
	json.Unmarshal(rawSearch, &searchItems) //nolint:errcheck
	nSearch := len(searchItems)

	// Scan memory store for keys that substring-match the query.
	var hits []memoryHit
	if includeMemory {
		keys, _ := t.store.List(ctx, "")
		queryLower := strings.ToLower(q)
		for _, key := range keys {
			if strings.Contains(strings.ToLower(key), queryLower) {
				val, err := t.store.Get(ctx, key)
				if err == nil {
					hits = append(hits, memoryHit{Key: key, Value: val})
				}
			}
		}
	}

	summary := fmt.Sprintf("Found %d document(s) and %d memory entrie(s) relevant to: %s", nSearch, len(hits), q)

	out := contextOutput{
		SearchResults:  rawSearch,
		MemoryHits:     hits,
		ContextSummary: summary,
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("marshal error: %v", err)), nil
	}
	return mcp.TextResult(string(encoded)), nil
}
