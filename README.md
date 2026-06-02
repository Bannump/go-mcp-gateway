# go-mcp-gateway

A production-grade, open-source MCP (Model Context Protocol) server written in Go. It implements the JSON-RPC 2.0 MCP spec from scratch — no SDK — with OIDC authentication, two storage backends, Prometheus metrics, and a plugin-style tool registry. Use it as a reference implementation or starter template for building your own MCP-compatible AI context server.

## Architecture

```
Client (curl / Claude / IDE plugin)
    │  Bearer JWT
    ▼
HTTP Transport (/mcp, /mcp/events, /healthz, /readyz, /metrics)
    │
    ▼
Auth Middleware  ── OIDC verifier ── JWKS client (cached, auto-refresh)
    │
    ▼
Rate Limit Middleware  (token bucket per authenticated subject)
    │
    ▼
MCP Server  (JSON-RPC 2.0 router: initialize / tools/list / tools/call / ping)
    │
    ▼
Tool Registry ── search_documents / store_memory / retrieve_memory / get_context
    │
    ▼
Storage  (MemoryStore  |  SQLiteStore WAL)
```

## Features

- **MCP protocol**: `initialize`, `tools/list`, `tools/call`, `ping`, `initialized`
- **Transports**: HTTP + SSE (`--transport=http`) and stdio (`--transport=stdio`)
- **Auth**: Generic OIDC via JWKS (works with Google, Auth0, Okta, Keycloak, …)
- **Rate limiting**: per-subject token bucket, configurable RPM
- **Tools**: document search (TF-IDF), key-value memory store, multi-source context
- **Storage**: in-memory (default) or SQLite WAL
- **Observability**: structured JSON logging (`log/slog`), Prometheus metrics
- **Graceful shutdown**: drains in-flight requests on SIGINT/SIGTERM

## Quick Start

```bash
# 1. Set required env vars (example: Google OIDC)
export OIDC_ISSUER=https://accounts.google.com
export OIDC_AUDIENCE=<your-google-client-id>
export OIDC_JWKS_URI=https://www.googleapis.com/oauth2/v3/certs

# 2. Run
make run

# 3. Health check (no auth)
curl http://localhost:8080/healthz

# 4. List tools (requires a valid bearer token)
curl -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
     http://localhost:8080/mcp
```

## Auth Setup (Google OIDC)

1. Create a project at [console.cloud.google.com](https://console.cloud.google.com).
2. Enable the Google Identity API and create an OAuth 2.0 client ID (type: "Web application").
3. Set env vars:
   ```
   OIDC_ISSUER=https://accounts.google.com
   OIDC_AUDIENCE=<client-id>.apps.googleusercontent.com
   OIDC_JWKS_URI=https://www.googleapis.com/oauth2/v3/certs
   ```
4. To obtain a test token, use `gcloud auth print-identity-token` (requires gcloud CLI).

## Available Tools

### `search_documents`
Search a built-in document corpus using TF-IDF scoring.

```json
// Request
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{
  "name":"search_documents",
  "arguments":{"query":"goroutines channels","top_k":3}
}}

// Response (result.content[0].text is a JSON array)
[{"id":"doc-2","title":"Understanding Goroutines","snippet":"...","score":0.042}]
```

### `store_memory`
Store a key-value pair with optional TTL.

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{
  "name":"store_memory",
  "arguments":{"key":"user:prefs","value":"{\"theme\":\"dark\"}","ttl_seconds":3600}
}}
```

### `retrieve_memory`
Retrieve a value by key.

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{
  "name":"retrieve_memory",
  "arguments":{"key":"user:prefs"}
}}
```

### `get_context`
Assemble context from document search + memory store in one call.

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{
  "name":"get_context",
  "arguments":{"query":"Go concurrency","include_memory":true,"top_k":3}
}}
```

## Adding a New Tool

Implement the `tools.Tool` interface:

```go
// internal/tools/mytool/tool.go
type MyTool struct{}
func (t *MyTool) Name() string              { return "my_tool" }
func (t *MyTool) Description() string       { return "does something useful" }
func (t *MyTool) InputSchema() json.RawMessage {
    return json.RawMessage(`{"type":"object","required":["input"],"properties":{"input":{"type":"string"}}}`)
}
func (t *MyTool) Execute(ctx context.Context, params json.RawMessage) (mcp.ToolResult, error) {
    // parse params, do work, return mcp.TextResult(...) or mcp.ErrorResult(...)
}
```

Then register in `cmd/server/main.go`:
```go
mustRegister(registry, mytool.New(), logger)
```

## Transport Modes

| Mode    | Flag / Env           | Auth   | Use case                       |
|---------|----------------------|--------|--------------------------------|
| HTTP    | `TRANSPORT=http`     | OIDC   | Remote API, Claude.ai, curl    |
| stdio   | `TRANSPORT=stdio`    | None   | Local IDE plugins (VS Code, …) |

## Storage Backends

| Backend | `STORAGE_BACKEND` | Notes                              |
|---------|-------------------|------------------------------------|
| Memory  | `memory` (default)| Resets on restart, no persistence  |
| SQLite  | `sqlite`          | Set `SQLITE_PATH=./mcp.db`         |

## Observability

| Metric                          | Type      | Labels                  |
|---------------------------------|-----------|-------------------------|
| `mcp_requests_total`            | Counter   | `method`, `status`      |
| `mcp_request_duration_seconds`  | Histogram | `method`                |
| `mcp_tool_calls_total`          | Counter   | `tool_name`, `status`   |
| `auth_failures_total`           | Counter   | `reason`                |
| `jwks_cache_refreshes_total`    | Counter   | —                       |
| `jwks_cache_refresh_failures_total` | Counter | —                    |

Scrape `GET /metrics`. Import the [Go dashboard](https://grafana.com/grafana/dashboards/10826) in Grafana and add your MCP panels.

## Docker Deployment

```bash
# Build
docker build -f deployments/Dockerfile -t go-mcp-gateway .

# Run
docker run --env-file .env -p 8080:8080 go-mcp-gateway

# Or use Compose
docker compose -f deployments/docker-compose.yml up
```

> **SQLite note**: The default Dockerfile uses a CGO-free build and `distroless/static`.  
> For SQLite support, switch the final image to `distroless/base-debian12` and remove `CGO_ENABLED=0`.

## Running Tests

```bash
make test           # all tests with race detector
make coverage       # generate and open HTML coverage report
```

## Contributing

1. Fork the repo and create a feature branch.
2. Run `make lint` and `make test` — both must pass with zero warnings.
3. Open a pull request with a clear description of what changed and why.
4. All exported symbols must have godoc comments; no `panic()` in HTTP paths.
