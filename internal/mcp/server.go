package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "go-mcp-gateway"
	serverVersion   = "1.0.0"
)

// ToolRegistry is the interface the MCP server uses to look up and list tools.
// *tools.Registry satisfies this interface.
type ToolRegistry interface {
	Get(name string) (ToolExecutor, bool)
	List() []ToolDefinition
}

// ToolExecutor can run a tool and return a result.
type ToolExecutor interface {
	Execute(ctx context.Context, params json.RawMessage) (ToolResult, error)
}

// Server routes JSON-RPC 2.0 MCP requests to the appropriate handler.
type Server struct {
	registry ToolRegistry
	logger   *slog.Logger
	metrics  ServerMetrics
}

// ServerMetrics contains callbacks used to record request telemetry.
type ServerMetrics struct {
	IncRequest      func(method, status string)
	ObserveDuration func(method string, d time.Duration)
	IncToolCall     func(toolName, status string)
}

// NewServer creates a new MCP server.
func NewServer(registry ToolRegistry, logger *slog.Logger, m ServerMetrics) *Server {
	return &Server{registry: registry, logger: logger, metrics: m}
}

// Handle processes a raw JSON-RPC 2.0 request and returns a Response.
// Notifications (no id) are handled and return a zero-value Response with nil ID.
func (s *Server) Handle(ctx context.Context, raw []byte) (Response, bool) {
	start := time.Now()

	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		s.record("parse_error", "error", time.Since(start))
		return errorResponse(nil, CodeParseError, "parse error"), true
	}

	if req.JSONRPC != "2.0" || req.Method == "" {
		s.record("invalid_request", "error", time.Since(start))
		return errorResponse(req.ID, CodeInvalidRequest, "invalid request"), true
	}

	isNotification := req.ID == nil

	resp, err := s.dispatch(ctx, &req)
	status := "success"
	if err != nil {
		status = "error"
	}
	s.record(req.Method, status, time.Since(start))

	if isNotification {
		return Response{}, false
	}
	return resp, true
}

func (s *Server) dispatch(ctx context.Context, req *Request) (Response, error) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return Response{}, nil
	case "ping":
		return successResponse(req.ID, map[string]string{"result": "pong"}), nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return errorResponse(req.ID, CodeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method)), fmt.Errorf("method not found")
	}
}

func (s *Server) handleInitialize(req *Request) (Response, error) {
	result := InitializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    Capabilities{Tools: ToolsCapability{ListChanged: false}},
		ServerInfo:      ServerInfo{Name: serverName, Version: serverVersion},
	}
	return successResponse(req.ID, result), nil
}

func (s *Server) handleToolsList(req *Request) (Response, error) {
	return successResponse(req.ID, ToolsListResult{Tools: s.registry.List()}), nil
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) (Response, error) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, "invalid params"), fmt.Errorf("invalid params")
	}

	tool, ok := s.registry.Get(params.Name)
	if !ok {
		return errorResponse(req.ID, CodeMethodNotFound, fmt.Sprintf("tool not found: %s", params.Name)), fmt.Errorf("tool not found")
	}

	result, err := tool.Execute(ctx, params.Arguments)
	if err != nil {
		s.incToolCall(params.Name, "error")
		return errorResponse(req.ID, CodeInternalError, fmt.Sprintf("tool execution failed: %v", err)), err
	}
	s.incToolCall(params.Name, "success")
	return successResponse(req.ID, result), nil
}

func (s *Server) record(method, status string, d time.Duration) {
	if s.metrics.IncRequest != nil {
		s.metrics.IncRequest(method, status)
	}
	if s.metrics.ObserveDuration != nil {
		s.metrics.ObserveDuration(method, d)
	}
}

func (s *Server) incToolCall(name, status string) {
	if s.metrics.IncToolCall != nil {
		s.metrics.IncToolCall(name, status)
	}
}
