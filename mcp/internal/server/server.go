package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/tools"
)

// New creates and configures the MCP server with the given configuration and
// Pulse API client. It registers all tools according to the configured
// Access_Mode.
func New(cfg *config.Config, client pulseapi.PulseClient) (*mcp.Server, error) {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "pulse-mcp",
		Version: "1.0.0",
	}, nil)

	deps := tools.Deps{
		Client:     client,
		AccessMode: cfg.AccessMode,
	}

	Register(s, deps, cfg.AccessMode)

	return s, nil
}

// Run starts the MCP server transport and blocks until the context is canceled.
// It emits exactly one startup log line stating the transport and Access_Mode.
func Run(ctx context.Context, cfg *config.Config, s *mcp.Server) error {
	log.Printf("pulse-mcp started transport=%s access_mode=%s", cfg.Transport, cfg.AccessMode)

	switch cfg.Transport {
	case "stdio":
		return s.Run(ctx, &mcp.StdioTransport{})

	case "http":
		handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return s
		}, nil)

		srv := &http.Server{
			Addr:    cfg.HTTPAddr,
			Handler: handler,
		}

		// Start listening.
		ln, err := net.Listen("tcp", cfg.HTTPAddr)
		if err != nil {
			return fmt.Errorf("http listen: %w", err)
		}

		// Serve in a goroutine so we can wait for context cancellation.
		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Serve(ln)
		}()

		select {
		case <-ctx.Done():
			// Graceful shutdown.
			if err := srv.Close(); err != nil {
				return fmt.Errorf("http server close: %w", err)
			}
			return ctx.Err()
		case err := <-errCh:
			return err
		}

	default:
		return fmt.Errorf("unsupported transport: %s", cfg.Transport)
	}
}
