package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"

	"github.com/Bannump/go-mcp-gateway/internal/mcp"
)

// StdioTransport reads newline-delimited JSON-RPC from stdin and writes to stdout.
// No auth middleware is applied — stdio is a local/trusted transport.
type StdioTransport struct {
	server *mcp.Server
	logger *slog.Logger
	in     io.Reader
	out    io.Writer
}

// NewStdioTransport creates a StdioTransport using os.Stdin / os.Stdout.
func NewStdioTransport(server *mcp.Server, logger *slog.Logger) *StdioTransport {
	return &StdioTransport{
		server: server,
		logger: logger,
		in:     os.Stdin,
		out:    os.Stdout,
	}
}

// Run blocks, processing one request per line until ctx is cancelled or EOF.
func (s *StdioTransport) Run(ctx context.Context) error {
	enc := json.NewEncoder(s.out)
	scanner := bufio.NewScanner(s.in)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return io.EOF
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp, hasResp := s.server.Handle(ctx, line)
		if !hasResp {
			continue
		}

		if err := enc.Encode(resp); err != nil {
			s.logger.Error("stdio: encode response", "error", err)
		}
	}
}
