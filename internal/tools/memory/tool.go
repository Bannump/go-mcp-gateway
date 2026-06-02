// Package memory implements the store_memory and retrieve_memory MCP tools.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Bannump/go-mcp-gateway/internal/mcp"
	"github.com/Bannump/go-mcp-gateway/internal/storage"
)

// StoreTool implements the store_memory MCP tool.
type StoreTool struct{ store storage.Store }

// NewStoreTool creates a StoreTool backed by the given Store.
func NewStoreTool(s storage.Store) *StoreTool { return &StoreTool{store: s} }

// Name returns the tool name.
func (t *StoreTool) Name() string { return "store_memory" }

// Description returns a human-readable description.
func (t *StoreTool) Description() string {
	return "Store a key-value pair in the memory store with an optional TTL."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *StoreTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["key", "value"],
		"properties": {
			"key":         {"type": "string", "description": "Storage key"},
			"value":       {"type": "string", "description": "Value to store"},
			"ttl_seconds": {"type": "integer", "description": "TTL in seconds (0 = no expiry)", "default": 0}
		}
	}`)
}

type storeInput struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// Execute stores the key-value pair and returns a confirmation.
func (t *StoreTool) Execute(ctx context.Context, params json.RawMessage) (mcp.ToolResult, error) {
	var input storeInput
	if err := json.Unmarshal(params, &input); err != nil {
		return mcp.ErrorResult("invalid parameters"), nil
	}
	if input.Key == "" {
		return mcp.ErrorResult("key must not be empty"), nil
	}

	ttl := time.Duration(input.TTLSeconds) * time.Second
	if err := t.store.Set(ctx, input.Key, input.Value, ttl); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("store error: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{"stored": true, "key": input.Key})
	return mcp.TextResult(string(out)), nil
}

// RetrieveTool implements the retrieve_memory MCP tool.
type RetrieveTool struct{ store storage.Store }

// NewRetrieveTool creates a RetrieveTool backed by the given Store.
func NewRetrieveTool(s storage.Store) *RetrieveTool { return &RetrieveTool{store: s} }

// Name returns the tool name.
func (t *RetrieveTool) Name() string { return "retrieve_memory" }

// Description returns a human-readable description.
func (t *RetrieveTool) Description() string {
	return "Retrieve a value from the memory store by key."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *RetrieveTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["key"],
		"properties": {
			"key": {"type": "string", "description": "Storage key to retrieve"}
		}
	}`)
}

type retrieveInput struct {
	Key string `json:"key"`
}

// Execute retrieves the value for the given key.
func (t *RetrieveTool) Execute(ctx context.Context, params json.RawMessage) (mcp.ToolResult, error) {
	var input retrieveInput
	if err := json.Unmarshal(params, &input); err != nil {
		return mcp.ErrorResult("invalid parameters"), nil
	}
	if input.Key == "" {
		return mcp.ErrorResult("key must not be empty"), nil
	}

	value, expiresAt, err := t.store.Get(ctx, input.Key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return mcp.ErrorResult(fmt.Sprintf("key not found: %s", input.Key)), nil
		}
		return mcp.ErrorResult(fmt.Sprintf("store error: %v", err)), nil
	}

	var expiresAtVal interface{}
	if !expiresAt.IsZero() {
		expiresAtVal = expiresAt.UTC().Format(time.RFC3339)
	}

	out, _ := json.Marshal(map[string]interface{}{
		"key":        input.Key,
		"value":      value,
		"expires_at": expiresAtVal,
	})
	return mcp.TextResult(string(out)), nil
}
