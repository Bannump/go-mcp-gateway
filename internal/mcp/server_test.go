package mcp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Bannump/go-mcp-gateway/internal/mcp"
	"github.com/Bannump/go-mcp-gateway/internal/tools"
)

func newTestServer(t *testing.T, toolList ...tools.Tool) *mcp.Server {
	t.Helper()
	reg := tools.NewRegistry()
	for _, tool := range toolList {
		require.NoError(t, reg.Register(tool))
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return mcp.NewServer(reg, logger, mcp.ServerMetrics{})
}

func TestServer_UnknownMethod(t *testing.T) {
	srv := newTestServer(t)
	raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"unknown/method"}`)
	resp, hasResp := srv.Handle(context.Background(), raw)
	assert.True(t, hasResp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.CodeMethodNotFound, resp.Error.Code)
}

func TestServer_MalformedJSON(t *testing.T) {
	srv := newTestServer(t)
	resp, hasResp := srv.Handle(context.Background(), []byte(`{invalid`))
	assert.True(t, hasResp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.CodeParseError, resp.Error.Code)
}

func TestServer_ToolsList(t *testing.T) {
	srv := newTestServer(t, &echoTool{})
	raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	resp, hasResp := srv.Handle(context.Background(), raw)
	assert.True(t, hasResp)
	assert.Nil(t, resp.Error)

	b, _ := json.Marshal(resp.Result)
	var result mcp.ToolsListResult
	require.NoError(t, json.Unmarshal(b, &result))
	assert.Len(t, result.Tools, 1)
	assert.Equal(t, "echo", result.Tools[0].Name)
}

func TestServer_ToolsCall(t *testing.T) {
	srv := newTestServer(t, &echoTool{})
	raw := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`)
	resp, hasResp := srv.Handle(context.Background(), raw)
	assert.True(t, hasResp)
	assert.Nil(t, resp.Error)
}

// echoTool is a stub tool for testing.
type echoTool struct{}

func (e *echoTool) Name() string        { return "echo" }
func (e *echoTool) Description() string { return "echoes input" }
func (e *echoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
}
func (e *echoTool) Execute(_ context.Context, params json.RawMessage) (mcp.ToolResult, error) {
	return mcp.TextResult(string(params)), nil
}
