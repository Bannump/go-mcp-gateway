# go-mcp-gateway

> A production-grade MCP server in Go — no SDK, no shortcuts.

`go-mcp-gateway` implements the [Model Context Protocol](https://modelcontextprotocol.io) over JSON-RPC 2.0 entirely from scratch. It gives AI agents (Claude, GPT, custom agents) a secure, observable, and extensible gateway to tools and data — authenticated via OIDC, instrumented with Prometheus, and deployable as a single binary or Docker image.

Use it as a working reference implementation, a starter template for your own MCP server, or plug it directly into your AI stack.

---

## Table of Contents

- [How It Works](#how-it-works)
- [Architecture](#architecture)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Live Demo: All Four Tools](#live-demo-all-four-tools)
- [Auth Setup](#auth-setup)
- [Transport Modes](#transport-modes)
- [Storage Backends](#storage-backends)
- [Adding a New Tool](#adding-a-new-tool)
- [Observability](#observability)
- [Docker Deployment](#docker-deployment)
- [Project Structure](#project-structure)
- [Running Tests](#running-tests)
- [Contributing](#contributing)

---

## How It Works

An AI agent sends a JSON-RPC 2.0 request to the gateway. The gateway verifies the caller's identity using an OIDC bearer token (Google, Auth0, Okta — anything), enforces per-user rate limits, routes the request to the right tool, and returns a structured result the agent can act on. Every request is logged and counted.

The four built-in tools cover the most common AI context needs: **keyword search** over a document corpus, **persistent key-value memory** with TTL, and a **context assembler** that merges both into a single response.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                             CLIENT                                    │
│            curl  ·  Claude.ai  ·  IDE Plugin  ·  AI Agent            │
└────────────────────────────────┬─────────────────────────────────────┘
                                 │
              ┌──────────────────┴────────────────────┐
              │  HTTP POST /mcp  (Bearer JWT)          │
              │  GET  /mcp/events  (SSE stream)        │
              │  GET  /healthz  /readyz  /metrics      │
              └──────────────────┬────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────────────────┐
│                         HTTP TRANSPORT                                │
│                                                                       │
│   /mcp ──── authenticated      /healthz ── unauthenticated           │
│   /mcp/events ── SSE stream    /readyz  ── unauthenticated           │
│                                /metrics ── Prometheus scrape         │
└────────────────────────────────┬─────────────────────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────────────────┐
│                        AUTH MIDDLEWARE                                │
│                                                                       │
│   Authorization: Bearer <JWT>                                         │
│          │                                                            │
│          ▼                                                            │
│   ┌─────────────────┐     ┌─────────────────────────────────────┐    │
│   │  OIDC Verifier  │────►│  JWKS Client                        │    │
│   │  RS256 · iss    │     │  · Fetch keys from provider URL     │    │
│   │  aud · exp · iat│     │  · Cache with configurable TTL      │    │
│   └────────┬────────┘     │  · Background refresh goroutine     │    │
│            │              │  · Serve stale keys on refresh fail  │    │
│            │              └─────────────────────────────────────┘    │
│       Claims in ctx    401 + reason on failure                       │
└────────────────────────────────┬─────────────────────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────────────────┐
│                     RATE LIMIT MIDDLEWARE                             │
│                                                                       │
│   Token bucket per authenticated subject (Claims.Subject)            │
│   Default: 60 req/min  ·  configurable via RATE_LIMIT_RPM            │
│   429 Too Many Requests + Retry-After header on limit exceeded       │
└────────────────────────────────┬─────────────────────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────────────────┐
│                          MCP SERVER                                   │
│                                                                       │
│   JSON-RPC 2.0 method router                                         │
│                                                                       │
│   initialize ──► return server info + capabilities                   │
│   initialized ──► acknowledge (notification, no response)            │
│   ping ──► pong                                                       │
│   tools/list ──► all registered tools with JSON schemas              │
│   tools/call ──► dispatch to tool, return ToolResult                 │
└────────────────────────────────┬─────────────────────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────────────────┐
│                         TOOL REGISTRY                                 │
│                                                                       │
│  ┌────────────────────┐  ┌───────────────────┐  ┌─────────────────┐ │
│  │  search_documents  │  │   store_memory    │  │   get_context   │ │
│  │                    │  │   retrieve_memory │  │                 │ │
│  │  TF-IDF scoring    │  │                   │  │  search +       │ │
│  │  over 15-doc       │  │  key-value store  │  │  memory merged  │ │
│  │  corpus            │  │  with TTL expiry  │  │  in one call    │ │
│  └────────────────────┘  └────────┬──────────┘  └────────┬────────┘ │
└────────────────────────────────────────────────┬──────────────────────┘
                                                 │
                          ┌──────────────────────▼──────────────────────┐
                          │                 STORAGE                      │
                          │                                              │
                          │  ┌──────────────────┐ ┌──────────────────┐ │
                          │  │   MemoryStore    │ │   SQLiteStore    │ │
                          │  │                  │ │                  │ │
                          │  │  sync.RWMutex    │ │  WAL mode        │ │
                          │  │  TTL sweep       │ │  concurrent read │ │
                          │  │  every 5 min     │ │  UPSERT on write │ │
                          │  └──────────────────┘ └──────────────────┘ │
                          └──────────────────────────────────────────────┘
```

### Auth Flow (detail)

```
Client                    Gateway                       OIDC Provider
  │                          │                               │
  │── POST /mcp ────────────►│                               │
  │   Authorization:         │                               │
  │   Bearer eyJhbGci...     │                               │
  │                          │── parse JWT header ──────────►│
  │                          │   extract kid                 │
  │                          │◄─ RSA public key (cached) ───│
  │                          │                               │
  │                          │── verify RS256 signature      │
  │                          │── validate exp / iat / iss / aud
  │                          │                               │
  │                          │── store Claims in context     │
  │                          │── pass to rate limiter        │
  │                          │── route to MCP handler        │
  │◄── JSON-RPC response ───│                               │
```

---

## Features

| Category | What's included |
|---|---|
| **Protocol** | JSON-RPC 2.0 · `initialize` · `tools/list` · `tools/call` · `ping` |
| **Transport** | HTTP + SSE · stdio (for IDE plugins) |
| **Auth** | Generic OIDC via JWKS · RS256 · works with Google, Auth0, Okta, Keycloak |
| **Rate limiting** | Per-subject token bucket · configurable RPM · `Retry-After` header |
| **Tools** | `search_documents` · `store_memory` · `retrieve_memory` · `get_context` |
| **Storage** | In-memory (default) · SQLite WAL |
| **Observability** | `log/slog` structured JSON logging · 6 Prometheus metrics |
| **Reliability** | Graceful shutdown · JWKS cache serves stale keys on failure |
| **Deployment** | Multi-stage Dockerfile (distroless) · docker-compose · GitHub Actions CI/CD |

---

## Prerequisites

- Go 1.22+
- An OIDC provider (Google, Auth0, Okta, or any standard JWKS endpoint)
- `make` (optional but recommended)

---

## Quick Start

**1. Clone and enter the project**

```bash
git clone https://github.com/Bannump/go-mcp-gateway.git
cd go-mcp-gateway
```

**2. Set the three required environment variables**

```bash
# Example: Google OIDC
export OIDC_ISSUER=https://accounts.google.com
export OIDC_AUDIENCE=<your-google-client-id>.apps.googleusercontent.com
export OIDC_JWKS_URI=https://www.googleapis.com/oauth2/v3/certs
```

**3. Start the server**

```bash
make run
# or: go run ./cmd/server
```

You should see:

```json
{"time":"...","level":"INFO","msg":"jwks: cache refreshed","keys":5}
{"time":"...","level":"INFO","msg":"starting http transport","port":"8080"}
```

**4. Verify it's alive** (no auth required)

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}

curl http://localhost:8080/readyz
# {"status":"ready"}
```

**5. Get a token and call the server**

```bash
# With gcloud CLI (Google)
export TOKEN=$(gcloud auth print-identity-token)

# Initialize an MCP session
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}' \
  http://localhost:8080/mcp | jq .
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": { "tools": { "listChanged": false } },
    "serverInfo": { "name": "go-mcp-gateway", "version": "1.0.0" }
  }
}
```

**6. List available tools**

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  http://localhost:8080/mcp | jq '.result.tools[].name'
```

```
"search_documents"
"store_memory"
"retrieve_memory"
"get_context"
```

---

## Configuration

All configuration is via environment variables. The server reports **all** missing required vars at once on startup — no hunting one-by-one.

| Variable | Required | Default | Description |
|---|---|---|---|
| `OIDC_ISSUER` | yes | — | OIDC issuer URL (e.g. `https://accounts.google.com`) |
| `OIDC_AUDIENCE` | yes | — | Expected audience (your client ID) |
| `OIDC_JWKS_URI` | yes | — | JWKS endpoint URL |
| `PORT` | no | `8080` | HTTP listen port |
| `TRANSPORT` | no | `http` | `http` or `stdio` |
| `STORAGE_BACKEND` | no | `memory` | `memory` or `sqlite` |
| `SQLITE_PATH` | no | `./mcp.db` | SQLite file path |
| `LOG_LEVEL` | no | `info` | `debug` · `info` · `warn` · `error` |
| `LOG_FORMAT` | no | `json` | `json` (production) · `text` (development) |
| `JWKS_REFRESH_MINUTES` | no | `60` | How often to refresh JWKS keys |
| `RATE_LIMIT_RPM` | no | `60` | Max requests per minute per authenticated user |

**Sample `.env` file**

```bash
OIDC_ISSUER=https://accounts.google.com
OIDC_AUDIENCE=123456789-abc.apps.googleusercontent.com
OIDC_JWKS_URI=https://www.googleapis.com/oauth2/v3/certs
PORT=8080
STORAGE_BACKEND=memory
LOG_FORMAT=text
LOG_LEVEL=debug
RATE_LIMIT_RPM=120
```

---

## Live Demo: All Four Tools

> All examples use `$TOKEN` for the bearer token and pipe through `jq` for readability.
> The `result.content[0].text` field in every response is a JSON string — parse it for the actual data.

### `search_documents` — TF-IDF keyword search

Search the built-in 15-document corpus. Scores are computed with TF-IDF on every request — stateless, no index to warm up.

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 10,
    "method": "tools/call",
    "params": {
      "name": "search_documents",
      "arguments": { "query": "goroutines concurrency channels", "top_k": 3 }
    }
  }' \
  http://localhost:8080/mcp | jq '.result.content[0].text | fromjson'
```

```json
[
  {
    "id": "doc-2",
    "title": "Understanding Goroutines",
    "snippet": "Goroutines are lightweight threads managed by the Go runtime. They enable massive concurrency. Use channels to communicate between goroutines...",
    "score": 0.087
  },
  {
    "id": "doc-1",
    "title": "Introduction to Go",
    "snippet": "Go is a statically typed compiled language designed at Google. It features garbage collection, strong typing, and excellent concurrency primitives...",
    "score": 0.031
  },
  {
    "id": "doc-14",
    "title": "Context Propagation in Go",
    "snippet": "Pass context.Context as the first argument to functions that do I/O or call external services...",
    "score": 0.018
  }
]
```

---

### `store_memory` — persist a key-value pair

Store anything — user preferences, session state, agent notes. Set a TTL or store indefinitely.

```bash
# Store with a 1-hour TTL
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 20,
    "method": "tools/call",
    "params": {
      "name": "store_memory",
      "arguments": {
        "key": "user:alice:preferences",
        "value": "{\"theme\":\"dark\",\"language\":\"en\"}",
        "ttl_seconds": 3600
      }
    }
  }' \
  http://localhost:8080/mcp | jq '.result.content[0].text | fromjson'
```

```json
{ "stored": true, "key": "user:alice:preferences" }
```

---

### `retrieve_memory` — look up a stored value

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 30,
    "method": "tools/call",
    "params": {
      "name": "retrieve_memory",
      "arguments": { "key": "user:alice:preferences" }
    }
  }' \
  http://localhost:8080/mcp | jq '.result.content[0].text | fromjson'
```

```json
{
  "key": "user:alice:preferences",
  "value": "{\"theme\":\"dark\",\"language\":\"en\"}",
  "expires_at": null
}
```

If the key doesn't exist or has expired, `isError` is `true` and the content explains why — no silent nulls.

---

### `get_context` — the killer tool

Combines document search and memory store in a single round-trip. Perfect for RAG-style context assembly before generating a response.

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 40,
    "method": "tools/call",
    "params": {
      "name": "get_context",
      "arguments": {
        "query": "user alice preferences",
        "include_memory": true,
        "top_k": 2
      }
    }
  }' \
  http://localhost:8080/mcp | jq '.result.content[0].text | fromjson'
```

```json
{
  "search_results": [
    {
      "id": "doc-10",
      "title": "Structured Logging with slog",
      "snippet": "Go 1.21 introduced the log/slog package...",
      "score": 0.012
    }
  ],
  "memory_hits": [
    {
      "key": "user:alice:preferences",
      "value": "{\"theme\":\"dark\",\"language\":\"en\"}"
    }
  ],
  "context_summary": "Found 1 document(s) and 1 memory entrie(s) relevant to: user alice preferences"
}
```

---

## Auth Setup

### Google OIDC (recommended for testing)

```bash
# 1. Create a project and OAuth 2.0 client ID at console.cloud.google.com
#    Type: "Web application"

# 2. Set env vars
export OIDC_ISSUER=https://accounts.google.com
export OIDC_AUDIENCE=<your-client-id>.apps.googleusercontent.com
export OIDC_JWKS_URI=https://www.googleapis.com/oauth2/v3/certs

# 3. Get a test token (requires gcloud CLI)
export TOKEN=$(gcloud auth print-identity-token)
```

### Auth0

```bash
export OIDC_ISSUER=https://<your-tenant>.auth0.com/
export OIDC_AUDIENCE=<your-api-identifier>
export OIDC_JWKS_URI=https://<your-tenant>.auth0.com/.well-known/jwks.json
```

### Any OIDC Provider

Point `OIDC_JWKS_URI` at any provider's `/.well-known/jwks.json` and set the matching issuer and audience. The verifier is generic — it validates RS256 signatures against the JWKS and checks `iss`, `aud`, `exp`, and `iat` claims.

### What happens on auth failure

```json
{
  "code": "token_expired",
  "message": "auth: token expired"
}
```

Possible `code` values: `missing_token` · `token_expired` · `invalid_signature` · `unknown_key` · `invalid_claims`

---

## Transport Modes

### HTTP + SSE (default)

The primary production transport. All `/mcp` routes are authenticated; probes and metrics are not.

```
POST /mcp          JSON-RPC request → JSON-RPC response
GET  /mcp/events   SSE stream for server-initiated notifications
GET  /healthz      Liveness probe  → {"status":"ok"}
GET  /readyz       Readiness probe → {"status":"ready"} or 503
GET  /metrics      Prometheus metrics
```

### stdio

For local use — IDE extensions, CLI tools, local AI agents. No auth required; the caller is trusted by definition.

```bash
# Start in stdio mode
TRANSPORT=stdio go run ./cmd/server

# Send a request on stdin
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | TRANSPORT=stdio go run ./cmd/server
```

| | HTTP | stdio |
|---|---|---|
| Auth | OIDC bearer token | None (trusted) |
| Rate limiting | Per subject | None |
| Server push | SSE `/mcp/events` | Not supported |
| Use case | Remote clients, Claude.ai | VS Code, local agents |

---

## Storage Backends

Switch backends with `STORAGE_BACKEND=sqlite` — no code changes required.

### Memory (default)

- Zero dependencies, zero setup
- Thread-safe `sync.RWMutex` map
- Background goroutine sweeps expired keys every 5 minutes
- Resets on restart — use for ephemeral session data

### SQLite (persistent)

- WAL mode for concurrent reads during writes
- `UPSERT` on write, expired-row cleanup every 5 minutes
- Persists across restarts — use for durable memory

```bash
STORAGE_BACKEND=sqlite SQLITE_PATH=./data/mcp.db go run ./cmd/server
```

> **Docker note:** If using SQLite, switch the Dockerfile's final image from `distroless/static` to `distroless/base-debian12` (CGO required). See the comment in `deployments/Dockerfile`.

---

## Adding a New Tool

Three steps, no framework magic.

**Step 1 — implement the `Tool` interface**

```go
// internal/tools/mytool/tool.go
package mytool

import (
    "context"
    "encoding/json"

    "github.com/Bannump/go-mcp-gateway/internal/mcp"
)

type MyTool struct{}

func New() *MyTool { return &MyTool{} }

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "does something useful" }

func (t *MyTool) InputSchema() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "required": ["input"],
        "properties": {
            "input": { "type": "string", "description": "The input value" }
        }
    }`)
}

func (t *MyTool) Execute(ctx context.Context, params json.RawMessage) (mcp.ToolResult, error) {
    var args struct {
        Input string `json:"input"`
    }
    if err := json.Unmarshal(params, &args); err != nil {
        return mcp.ErrorResult("invalid params"), nil
    }
    return mcp.TextResult("you said: " + args.Input), nil
}
```

**Step 2 — register in `cmd/server/app.go`**

```go
import mytool "github.com/Bannump/go-mcp-gateway/internal/tools/mytool"

// inside buildRegistry():
toolList := []tools.Tool{
    searchTool,
    memtool.NewStoreTool(store),
    memtool.NewRetrieveTool(store),
    ctxtool.New(searchTool, store),
    mytool.New(),   // ← add here
}
```

**Step 3 — it's live**

Restart the server. Your tool appears in `tools/list` and is callable via `tools/call` immediately — no routing config, no registration files.

---

## Observability

### Logging

Structured JSON in production, human-readable text in development.

```bash
LOG_FORMAT=text LOG_LEVEL=debug go run ./cmd/server
```

Sample log line per request:

```json
{"time":"2025-06-01T12:00:00Z","level":"INFO","msg":"mcp request","method":"tools/call","client_subject":"user-123","latency_ms":0}
```

Token values and claim payloads are never logged.

### Prometheus Metrics

Scraped from `GET /metrics` (no auth required).

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `mcp_requests_total` | Counter | `method`, `status` | All JSON-RPC requests |
| `mcp_request_duration_seconds` | Histogram | `method` | Per-method latency |
| `mcp_tool_calls_total` | Counter | `tool_name`, `status` | Tool invocations |
| `auth_failures_total` | Counter | `reason` | Auth rejections by reason |
| `jwks_cache_refreshes_total` | Counter | — | Successful key refreshes |
| `jwks_cache_refresh_failures_total` | Counter | — | Failed key refreshes |

**Grafana:** import the [Go dashboard (ID 10826)](https://grafana.com/grafana/dashboards/10826) and add panels for the `mcp_*` metrics.

---

## Docker Deployment

### Single container

```bash
# Build
docker build -t go-mcp-gateway .

# Run (create .env from the sample in Configuration above)
docker run --env-file .env -p 8080:8080 go-mcp-gateway
```

### Docker Compose (with SQLite volume)

```bash
# Copy and fill in your values
cp .env.example .env   # edit OIDC_ISSUER, OIDC_AUDIENCE, OIDC_JWKS_URI

docker compose -f deployments/docker-compose.yml up
```

The Compose file mounts a named volume at `/data` so the SQLite database survives container restarts.

### Image size

The multi-stage build copies only the compiled binary into a `distroless/static` image — no shell, no package manager, minimal attack surface. The final image is typically **~12 MB**.

---

## Project Structure

```
go-mcp-gateway/
├── cmd/server/
│   ├── main.go              # startup, signal handling, graceful shutdown (≤60 lines)
│   └── app.go               # dependency wiring
├── internal/
│   ├── auth/
│   │   ├── claims.go        # Claims struct + context key
│   │   ├── oidc.go          # OIDC token verifier (RS256 + claim validation)
│   │   └── middleware.go    # HTTP middleware: extract + verify bearer token
│   ├── config/
│   │   └── config.go        # env-based config, reports all missing vars at once
│   ├── mcp/
│   │   ├── types.go         # all MCP / JSON-RPC 2.0 types
│   │   ├── server.go        # method router + ToolRegistry interface
│   │   └── transport/
│   │       ├── http.go      # HTTP + SSE transport, rate limiting
│   │       └── stdio.go     # stdio transport
│   ├── observability/
│   │   ├── logger.go        # slog setup (JSON / text)
│   │   └── metrics.go       # all Prometheus metrics
│   ├── storage/
│   │   ├── store.go         # Store interface + ErrNotFound
│   │   ├── memory.go        # in-memory implementation
│   │   └── sqlite.go        # SQLite WAL implementation
│   └── tools/
│       ├── registry.go      # Tool interface + Registry
│       ├── search/tool.go   # search_documents (TF-IDF)
│       ├── memory/tool.go   # store_memory + retrieve_memory
│       └── context/tool.go  # get_context (multi-source assembler)
├── pkg/
│   └── jwks/client.go       # reusable JWKS client (import in your own project)
├── api/openapi.yaml          # OpenAPI 3.1 spec
├── deployments/
│   ├── Dockerfile
│   └── docker-compose.yml
├── docs/architecture.md      # extended design notes
├── .github/workflows/
│   ├── ci.yml               # lint + test on push/PR
│   └── release.yml          # build + push Docker image on tag
└── Makefile
```

---

## Running Tests

```bash
make test        # go test ./... -race
make coverage    # HTML coverage report

# Run a specific package
go test ./internal/auth/... -v -run TestVerifyToken
```

**Test coverage:**

| Package | What's tested |
|---|---|
| `internal/auth` | Valid token · expired · wrong issuer · wrong audience · unknown kid · bad signature |
| `internal/mcp` | Unknown method → -32601 · malformed JSON → -32700 · tools/list · tools/call dispatch |
| `internal/storage` | Set/Get round-trip · TTL expiry → ErrNotFound · Delete · List with prefix |
| `internal/tools/search` | top_k results · capped at 20 · empty query error |

All tests run with `-race`. No mocks — storage tests use the real in-memory implementation.

---

## Makefile Targets

```bash
make build        # compile binary
make test         # run all tests with race detector
make lint         # golangci-lint (errcheck, govet, staticcheck, unused, gofmt)
make run          # go run ./cmd/server (HTTP mode)
make run-stdio    # go run ./cmd/server (stdio mode)
make docker-build # docker build -t go-mcp-gateway .
make docker-run   # docker run --env-file .env -p 8080:8080 go-mcp-gateway
make coverage     # generate + open HTML coverage report
```

---

## Contributing

1. Fork the repo and create a feature branch off `main`.
2. Run `make lint` and `make test` — both must pass with zero warnings before opening a PR.
3. Every exported symbol needs a godoc comment. No `panic()` in any HTTP-reachable path.
4. Keep `main.go` under 60 lines — dependency wiring goes in `cmd/server/app.go`.
5. New tools go in their own subdirectory under `internal/tools/` and self-register in `buildRegistry()`.

---

<p align="center">
  Built with Go · MIT License · <a href="https://github.com/Bannump/go-mcp-gateway">github.com/Bannump/go-mcp-gateway</a>
</p>
