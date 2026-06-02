# Architecture

## System Overview

```
Client (curl / Claude / IDE)
         │
         │  HTTP POST /mcp  (Bearer token)
         ▼
┌──────────────────────────────────────────────────────┐
│                  HTTP Transport                       │
│  POST /mcp         GET /mcp/events    GET /healthz   │
│  GET  /readyz      GET /metrics                      │
└────────────────────┬─────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────┐
│               Auth Middleware                         │
│  Extract Bearer token → VerifyToken (RS256)           │
│  Store Claims in context                              │
│  401 on failure, log reason + client IP               │
└────────────────────┬─────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────┐
│           Rate Limit Middleware                        │
│  Per-subject token bucket (golang.org/x/time/rate)    │
│  429 + Retry-After on limit exceeded                  │
└────────────────────┬─────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────┐
│                 MCP Server                            │
│  JSON-RPC 2.0 method router                           │
│  initialize / tools/list / tools/call / ping          │
└────────────────────┬─────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────┐
│               Tool Registry                           │
│  search_documents  store_memory                       │
│  retrieve_memory   get_context                        │
└────────────────────┬─────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────┐
│                  Storage                              │
│  MemoryStore (default)   SQLiteStore (WAL mode)       │
└──────────────────────────────────────────────────────┘
```

## Auth Flow

```
1. Client sends: Authorization: Bearer <JWT>
2. Middleware extracts token string
3. JWT header parsed → kid extracted
4. JWKS client looks up RSA public key by kid
5. jwt.Parse verifies RS256 signature
6. Claims validated: exp, iat, iss, aud
7. Claims stored in context → available to handlers
```

## Transport Comparison

| Feature        | HTTP + SSE          | stdio                    |
|----------------|---------------------|--------------------------|
| Auth           | OIDC bearer token   | None (trusted local)     |
| Use case       | Remote API clients  | IDE extensions, local AI |
| Server push    | SSE /mcp/events     | Not supported            |
| Rate limiting  | Yes (per subject)   | No                       |
| Observability  | Prometheus + slog   | slog only                |

## Adding a New Tool

1. Create `internal/tools/mytool/tool.go` implementing `tools.Tool`:
   ```go
   type MyTool struct{}
   func (t *MyTool) Name() string              { return "my_tool" }
   func (t *MyTool) Description() string       { return "..." }
   func (t *MyTool) InputSchema() json.RawMessage { return json.RawMessage(`{...}`) }
   func (t *MyTool) Execute(ctx context.Context, params json.RawMessage) (mcp.ToolResult, error) {
       // implementation
   }
   ```

2. Register in `cmd/server/main.go`:
   ```go
   mustRegister(registry, mytool.New(), logger)
   ```

3. Tools are automatically surfaced via `tools/list` and callable via `tools/call`.
