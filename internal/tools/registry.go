// Package tools defines the Tool interface and the tool Registry.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Bannump/go-mcp-gateway/internal/mcp"
)

// Tool is the interface every tool must implement.
type Tool interface {
	// Name returns the unique tool identifier used in tools/call requests.
	Name() string
	// Description returns a human-readable description for tools/list.
	Description() string
	// InputSchema returns a JSON Schema object describing accepted parameters.
	InputSchema() json.RawMessage
	// Execute runs the tool with the given params and returns a ToolResult.
	Execute(ctx context.Context, params json.RawMessage) (mcp.ToolResult, error)
}

// Registry maps tool names to Tool implementations.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Returns an error if a tool with the same name is already registered.
func (r *Registry) Register(t Tool) error {
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("tools: %q already registered", t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

// Get returns the tool executor for the given name, satisfying mcp.ToolRegistry.
func (r *Registry) Get(name string) (mcp.ToolExecutor, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all tool definitions for the tools/list response, satisfying mcp.ToolRegistry.
func (r *Registry) List() []mcp.ToolDefinition {
	defs := make([]mcp.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, mcp.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return defs
}
