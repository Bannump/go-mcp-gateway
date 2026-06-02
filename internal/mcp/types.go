// Package mcp implements the Model Context Protocol over JSON-RPC 2.0.
package mcp

import "encoding/json"

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32000
)

// Request is a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServerInfo is returned by the initialize response.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the result of the initialize method.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

// Capabilities describes what the server supports.
type Capabilities struct {
	Tools ToolsCapability `json:"tools"`
}

// ToolsCapability signals that the server supports tool listing and calling.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ToolDefinition is returned by tools/list.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCallParams holds the parameters for a tools/call request.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ContentItem is a single piece of content in a ToolResult.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolResult is returned by tools/call.
type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError"`
}

// ToolsListResult wraps the tools/list response.
type ToolsListResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// TextResult builds a successful ToolResult with a single text item.
func TextResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
		IsError: false,
	}
}

// ErrorResult builds an error ToolResult with a single text item.
func ErrorResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
		IsError: true,
	}
}

// successResponse builds a JSON-RPC success response.
func successResponse(id *json.RawMessage, result interface{}) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

// errorResponse builds a JSON-RPC error response.
func errorResponse(id *json.RawMessage, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}
